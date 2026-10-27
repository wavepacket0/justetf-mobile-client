package justetf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

var (
	ErrAuthenticationRequired = errors.New("authentication required; log in again")
	ErrSessionChanged         = errors.New("authentication session changed")
	ErrClientClosed           = errors.New("client is closed")
	tokenPattern              = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)
)

type tokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type authSnapshot struct {
	epoch    uint64
	revision uint64
	access   string
	refresh  string
}

type refreshFlight struct {
	done   chan struct{}
	err    error
	cancel context.CancelFunc
}

type authState struct {
	sync.Mutex
	tokens           tokenPair
	epoch            uint64
	revision         uint64
	closed           bool
	flight           *refreshFlight
	lastRefreshError error
}

func (client *Client) isClosed() bool {
	client.auth.Lock()
	defer client.auth.Unlock()
	return client.auth.closed
}

func (client *Client) Close() {
	client.auth.Lock()
	if !client.auth.closed {
		client.auth.closed = true
		client.clearSessionLocked()
		client.cancel()
	}
	client.auth.Unlock()
	client.CloseIdleConnections()
}

func (client *Client) clearSessionLocked() {
	client.auth.epoch++
	client.auth.revision = 0
	client.auth.tokens = tokenPair{}
	client.auth.lastRefreshError = nil
	if client.auth.flight != nil {
		client.auth.flight.cancel()
		client.auth.flight = nil
	}
}

func (client *Client) Logout() {
	client.auth.Lock()
	defer client.auth.Unlock()
	client.clearSessionLocked()
}

func (client *Client) IsAuthenticated() bool {
	client.auth.Lock()
	defer client.auth.Unlock()
	return !client.auth.closed && client.auth.tokens.AccessToken != "" && client.auth.tokens.RefreshToken != ""
}

func (client *Client) authenticationTransport() error {
	address, _ := url.Parse(client.config.BaseURL)
	if address.Scheme == "https" {
		return nil
	}
	host := address.Hostname()
	if host == "localhost" || net.ParseIP(host).IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: authentication requires HTTPS except for loopback tests", ErrInvalidOptions)
}

func (client *Client) exchangeTokens(ctx context.Context, path, authorization string) (tokenPair, error) {
	if err := client.authenticationTransport(); err != nil {
		return tokenPair{}, err
	}
	body, err := client.doJSONAuthorization(ctx, http.MethodGet, path, nil, struct{}{}, requiredJSON, authorization)
	if err != nil {
		var apiError *APIError
		switch {
		case errors.As(err, &apiError), errors.Is(err, ErrClientClosed), errors.Is(err, ErrInvalidOptions):
			return tokenPair{}, err
		case errors.Is(err, context.Canceled):
			return tokenPair{}, context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			return tokenPair{}, context.DeadlineExceeded
		default:
			return tokenPair{}, errors.New("authentication request failed")
		}
	}
	var tokens tokenPair
	if json.Unmarshal(body, &tokens) != nil || len(tokens.AccessToken) > 8192 || len(tokens.RefreshToken) > 8192 || !tokenPattern.MatchString(tokens.AccessToken) || !tokenPattern.MatchString(tokens.RefreshToken) {
		return tokenPair{}, errors.New("invalid authentication response")
	}
	return tokens, nil
}

func (client *Client) Login(ctx context.Context, email, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if email == "" || password == "" || strings.ContainsAny(email, ":\r\n") {
		return fmt.Errorf("%w: nonempty email/password required; email cannot contain colon or line breaks", ErrInvalidOptions)
	}
	if err := client.authenticationTransport(); err != nil {
		return err
	}
	client.auth.Lock()
	if client.auth.closed {
		client.auth.Unlock()
		return ErrClientClosed
	}
	client.clearSessionLocked()
	epoch := client.auth.epoch
	client.auth.Unlock()
	authorization := "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+password))
	tokens, err := client.exchangeTokens(ctx, loginPath, authorization)
	client.auth.Lock()
	defer client.auth.Unlock()
	if client.auth.closed {
		return ErrClientClosed
	}
	if epoch != client.auth.epoch {
		return ErrSessionChanged
	}
	if err != nil {
		return err
	}
	client.auth.tokens = tokens
	return nil
}

func (client *Client) snapshot() (authSnapshot, error) {
	client.auth.Lock()
	defer client.auth.Unlock()
	if client.auth.closed {
		return authSnapshot{}, ErrClientClosed
	}
	if client.auth.tokens.AccessToken == "" || client.auth.tokens.RefreshToken == "" {
		return authSnapshot{}, ErrAuthenticationRequired
	}
	return authSnapshot{client.auth.epoch, client.auth.revision, client.auth.tokens.AccessToken, client.auth.tokens.RefreshToken}, nil
}

func (client *Client) Refresh(ctx context.Context) error {
	snapshot, err := client.snapshot()
	if err != nil {
		return err
	}
	return client.refreshFor(ctx, snapshot, true)
}

func (client *Client) refreshFor(ctx context.Context, snapshot authSnapshot, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client.auth.Lock()
	if client.auth.closed {
		client.auth.Unlock()
		return ErrClientClosed
	}
	if snapshot.epoch != client.auth.epoch {
		client.auth.Unlock()
		return ErrSessionChanged
	}
	if client.auth.tokens.RefreshToken == "" {
		client.auth.Unlock()
		return ErrAuthenticationRequired
	}
	if snapshot.revision != client.auth.revision {
		client.auth.Unlock()
		return nil
	}
	flight := client.auth.flight
	if flight == nil {
		if !force && client.auth.lastRefreshError != nil {
			err := client.auth.lastRefreshError
			client.auth.Unlock()
			return err
		}
		workCtx, cancel := context.WithTimeout(client.ctx, client.config.Timeout)
		flight = &refreshFlight{done: make(chan struct{}), cancel: cancel}
		client.auth.flight = flight
		go client.performRefresh(workCtx, snapshot, flight)
	}
	client.auth.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-flight.done:
		return flight.err
	}
}

func (client *Client) performRefresh(ctx context.Context, snapshot authSnapshot, flight *refreshFlight) {
	var tokens tokenPair
	var err error
	defer func() {
		if recover() != nil {
			err = errors.New("internal authentication refresh error")
		}
		flight.cancel()
		client.auth.Lock()
		defer client.auth.Unlock()
		if client.auth.closed {
			err = ErrClientClosed
		} else if snapshot.epoch != client.auth.epoch {
			err = ErrSessionChanged
		} else {
			if err == nil {
				client.auth.tokens = tokens
				client.auth.revision++
				client.auth.lastRefreshError = nil
			} else {
				client.auth.lastRefreshError = err
				var apiError *APIError
				if errors.As(err, &apiError) && (apiError.StatusCode == 401 || apiError.StatusCode == 403) {
					client.auth.tokens = tokenPair{}
				}
			}
		}
		if client.auth.flight == flight {
			client.auth.flight = nil
		}
		flight.err = err
		close(flight.done)
	}()
	tokens, err = client.exchangeTokens(ctx, refreshPath, "Bearer "+snapshot.refresh)
}

type replayPolicy uint8

const (
	neverReplay replayPolicy = iota
	replayReadOnce
)

func (client *Client) checkSession(epoch uint64) error {
	client.auth.Lock()
	defer client.auth.Unlock()
	if client.auth.closed {
		return ErrClientClosed
	}
	if client.auth.epoch != epoch {
		return ErrSessionChanged
	}
	return nil
}

func (client *Client) doAuthenticatedJSON(ctx context.Context, method, path string, query url.Values, body any, policy responsePolicy, replay replayPolicy) (json.RawMessage, error) {
	if replay > replayReadOnce || (replay == replayReadOnce && method != http.MethodGet && method != http.MethodHead) {
		return nil, fmt.Errorf("%w: replay is allowed only for designated GET/HEAD reads", ErrInvalidOptions)
	}
	ctx, cancel := context.WithTimeout(ctx, client.config.Timeout)
	defer cancel()
	snapshot, err := client.snapshot()
	if err != nil {
		return nil, err
	}
	result, err := client.doJSONAuthorization(ctx, method, path, query, body, policy, "Bearer "+snapshot.access)
	if sessionErr := client.checkSession(snapshot.epoch); sessionErr != nil {
		return nil, sessionErr
	}
	var apiError *APIError
	if replay == neverReplay || !errors.As(err, &apiError) || apiError.StatusCode != 401 {
		return result, err
	}
	if err := client.refreshFor(ctx, snapshot, false); err != nil {
		return nil, err
	}
	updated, err := client.snapshot()
	if err != nil {
		return nil, err
	}
	if updated.epoch != snapshot.epoch {
		return nil, ErrSessionChanged
	}
	result, err = client.doJSONAuthorization(ctx, method, path, query, body, policy, "Bearer "+updated.access)
	if sessionErr := client.checkSession(snapshot.epoch); sessionErr != nil {
		return nil, sessionErr
	}
	return result, err
}

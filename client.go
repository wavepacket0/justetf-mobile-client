package justetf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	BaseURL  string
	APIKey   string
	Language string
	Country  string
	Currency string
	Timeout  time.Duration
}

type Client struct {
	config Config
	http   *http.Client
	ctx    context.Context
	cancel context.CancelFunc
	auth   authState
}

type APIError struct {
	StatusCode int
}

func (err *APIError) Error() string {
	return fmt.Sprintf("justETF API returned HTTP %d", err.StatusCode)
}

func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("invalid API base URL")
	}
	if config.APIKey == "" {
		config.APIKey = mobileAPIKey
	}
	if config.Timeout < 0 {
		return nil, errors.New("timeout must be positive")
	}
	if config.Timeout == 0 {
		config.Timeout = 15 * time.Second
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	if config.Language == "" {
		config.Language = "it"
	}
	if config.Country == "" {
		config.Country = "IT"
	}
	if config.Currency == "" {
		config.Currency = "EUR"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{config: config, ctx: ctx, cancel: cancel, http: &http.Client{
		Transport: newTransport(),
		Timeout:   config.Timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

var isinPattern = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)

var ErrInvalidISIN = errors.New("ISIN must contain 12 uppercase letters/digits with a two-letter prefix and final digit")

func (client *Client) CloseIdleConnections() {
	client.http.CloseIdleConnections()
}

func (client *Client) GetProfile(ctx context.Context, isin string) (json.RawMessage, error) {
	return client.GetProfileWithOptions(ctx, isin, MarketOptions{})
}

func (client *Client) GetProfileWithOptions(ctx context.Context, isin string, options MarketOptions) (json.RawMessage, error) {
	if !isinPattern.MatchString(isin) {
		return nil, ErrInvalidISIN
	}
	query := client.marketQuery(options)
	return client.doJSON(ctx, http.MethodGet, profilePathPrefix+isin, query, struct{}{}, requiredJSON)
}

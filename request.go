package justetf

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type responsePolicy uint8

const (
	requiredJSON responsePolicy = iota
	optionalJSON
	emptySuccess
)

func encodeQuery(query url.Values) string {
	encoded := query.Encode()
	bare := make([]string, 0)
	for name, values := range query {
		if values == nil {
			bare = append(bare, url.QueryEscape(name))
		}
	}
	sort.Strings(bare)
	if encoded != "" {
		bare = append([]string{encoded}, bare...)
	}
	return strings.Join(bare, "&")
}

func (client *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, policy responsePolicy) (json.RawMessage, error) {
	return client.doJSONAuthorization(ctx, method, path, query, body, policy, "")
}

func authenticatedTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New("authenticated request failed")
}

func (client *Client) doJSONAuthorization(ctx context.Context, method, path string, query url.Values, body any, policy responsePolicy, authorization string) (json.RawMessage, error) {
	if client.isClosed() {
		return nil, ErrClientClosed
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(client.ctx, cancel)
	defer stop()
	defer cancel()
	if policy > emptySuccess {
		return nil, errors.New("invalid response policy")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	endpoint := client.config.BaseURL + path
	if len(query) != 0 {
		endpoint += "?" + encodeQuery(query)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	requestID := make([]byte, 16)
	if _, err := rand.Read(requestID); err != nil {
		return nil, err
	}
	requestID[6] = (requestID[6] & 0x0f) | 0x40
	requestID[8] = (requestID[8] & 0x3f) | 0x80
	request.Header.Set("x-request-id", fmt.Sprintf("%x-%x-%x-%x-%x", requestID[:4], requestID[4:6], requestID[6:8], requestID[8:10], requestID[10:]))
	request.Header.Set("x-api-key", client.config.APIKey)
	request.Header.Set("accept-language", client.config.Language)
	request.Header.Set("user-agent", mobileUserAgent)
	request.Header.Set("content-type", contentTypeJSON)
	if authorization != "" {
		base, _ := url.Parse(client.config.BaseURL)
		if request.URL.Scheme != base.Scheme || request.URL.Host != base.Host {
			return nil, errors.New("authorization origin mismatch")
		}
		request.Header.Set("authorization", authorization)
	}
	response, err := client.http.Do(request)
	if err != nil {
		if client.isClosed() {
			return nil, ErrClientClosed
		}
		if authorization != "" {
			return nil, authenticatedTransportError(err)
		}
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		io.CopyN(io.Discard, response.Body, 64<<10)
		return nil, &APIError{StatusCode: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		if client.isClosed() {
			return nil, ErrClientClosed
		}
		if authorization != "" {
			return nil, authenticatedTransportError(err)
		}
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, errors.New("API response exceeds 4 MiB")
	}
	if len(data) == 0 {
		if policy == optionalJSON || policy == emptySuccess {
			return nil, nil
		}
		return nil, errors.New("API returned an empty JSON response")
	}
	if policy == emptySuccess {
		return nil, errors.New("API returned a body for an empty-success operation")
	}
	if !json.Valid(data) {
		return nil, errors.New("API returned invalid JSON")
	}
	return json.RawMessage(data), nil
}

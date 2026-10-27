package justetf

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthenticationContracts(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.Method != "GET" || string(body) != "{}" || request.Header.Get("user-agent") != "Dart/3.12 (dart:io)" || request.Header.Get("x-api-key") != "test" {
			t.Error("authentication request differs from mobile contract")
		}
		switch request.URL.Path {
		case "/v1/auth/tokens":
			email, password, ok := request.BasicAuth()
			if !ok || email != "test@example.invalid" || password != "test:password" {
				t.Error("incorrect Basic login")
			}
			writer.Write([]byte(`{"accessToken":"access-one","refreshToken":"refresh-one"}`))
		case "/v1/auth/refreshed-tokens":
			count := refreshes.Add(1)
			expected := "Bearer refresh-one"
			access, refresh := "access-two", "refresh-two"
			if count == 2 {
				expected = "Bearer refresh-two"
				access, refresh = "access-three", "refresh-three"
			}
			if request.Header.Get("Authorization") != expected {
				t.Error("refresh did not rotate tokens")
			}
			json.NewEncoder(writer).Encode(tokenPair{access, refresh})
		case "/protected-test":
			if request.Header.Get("Authorization") == "Bearer access-one" {
				writer.WriteHeader(401)
				return
			}
			if request.Header.Get("Authorization") != "Bearer access-two" {
				t.Error("protected request did not use refreshed access token")
			}
			writer.Write([]byte(`{"ok":true}`))
		default:
			if request.Header.Get("Authorization") != "" {
				t.Error("public request leaked authorization")
			}
			writer.Write([]byte(`{"public":true}`))
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	if client.IsAuthenticated() {
		t.Fatal("new client should be anonymous")
	}
	if err := client.Login(ctx, "test@example.invalid", "test:password"); err != nil {
		t.Fatal(err)
	}
	if !client.IsAuthenticated() {
		t.Fatal("login did not establish a session")
	}
	result, err := client.doAuthenticatedJSON(ctx, "GET", "/protected-test", nil, struct{}{}, requiredJSON, replayReadOnce)
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("protected read failed: %v", err)
	}
	if _, err := client.GetQuote(ctx, "IE00B5BMR087", CurrencyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 2 {
		t.Fatal("unexpected refresh count")
	}
	client.Logout()
	if client.IsAuthenticated() {
		t.Fatal("logout retained authentication")
	}
	if err := client.Refresh(ctx); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("refresh without a session: %v", err)
	}
	client.Close()
	client.Close()
	if err := client.Login(ctx, "test@example.invalid", "password"); !errors.Is(err, ErrClientClosed) {
		t.Fatal("closed client accepted login")
	}
}

func TestLateOldTokenRejectionUsesRotatedSession(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
			return
		}
		refreshes.Add(1)
		writer.Write([]byte(`{"accessToken":"new-access","refreshToken":"new-refresh"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	old, err := client.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 4; attempt++ {
		if err := client.refreshFor(context.Background(), old, false); err != nil {
			t.Fatal(err)
		}
	}
	if refreshes.Load() != 1 {
		t.Fatal("late old-token rejections caused redundant refreshes")
	}
}

func TestAuthenticatedTransportErrorRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"private-access","refreshToken":"private-refresh"}`))
			return
		}
		connection, buffer, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer connection.Close()
		buffer.WriteString("HTTP/1.1 private-access invalid\r\n\r\n")
		buffer.Flush()
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	_, err = client.doAuthenticatedJSON(context.Background(), "GET", "/protected-test", nil, struct{}{}, requiredJSON, replayReadOnce)
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("malformed server response exposed authenticated transport details")
	}
}

func TestAmbiguousMutationIsNotReplayed(t *testing.T) {
	var mutations, refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		switch request.URL.Path {
		case "/v1/auth/tokens":
			writer.Write([]byte(`{"accessToken":"test-access","refreshToken":"test-refresh"}`))
		case "/v1/auth/refreshed-tokens":
			refreshes.Add(1)
			writer.WriteHeader(500)
		default:
			mutations.Add(1)
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			connection.Close()
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	_, err = client.doAuthenticatedJSON(context.Background(), "POST", "/mutation-test", nil, struct{}{}, emptySuccess, neverReplay)
	if err == nil || mutations.Load() != 1 || refreshes.Load() != 0 {
		t.Fatal("ambiguous mutation was replayed or unexpectedly succeeded")
	}
}
func TestConcurrentExpiredRequests(t *testing.T) {
	const workers = 16
	var expired atomic.Int32
	var refreshes atomic.Int32
	allExpired := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		switch request.URL.Path {
		case "/v1/auth/tokens":
			writer.Write([]byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
		case "/v1/auth/refreshed-tokens":
			refreshes.Add(1)
			<-allExpired
			writer.Write([]byte(`{"accessToken":"new-access","refreshToken":"new-refresh"}`))
		default:
			if request.Header.Get("Authorization") == "Bearer old-access" {
				if expired.Add(1) == workers {
					close(allExpired)
				}
				writer.WriteHeader(401)
				return
			}
			if request.Header.Get("Authorization") != "Bearer new-access" {
				t.Error("incorrect access token")
			}
			writer.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := client.doAuthenticatedJSON(context.Background(), "GET", "/protected-test", nil, struct{}{}, requiredJSON, replayReadOnce); err != nil {
				t.Errorf("concurrent protected read: %v", err)
			}
		}()
	}
	wait.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("expected one coalesced refresh, got %d", refreshes.Load())
	}
}

func TestAuthenticationReplayBoundaries(t *testing.T) {
	for _, scenario := range []struct {
		name               string
		method             string
		replay             replayPolicy
		protected, refresh int32
		status             int
	}{
		{"read-once", "GET", replayReadOnce, 2, 1, 401},
		{"mutation-never", "POST", neverReplay, 1, 0, 401},
		{"forbidden-read", "GET", replayReadOnce, 1, 0, 403},
		{"no-write-opt-in", "POST", replayReadOnce, 0, 0, 401},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var protected, refresh atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				io.Copy(io.Discard, request.Body)
				switch request.URL.Path {
				case "/v1/auth/tokens":
					writer.Write([]byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
				case "/v1/auth/refreshed-tokens":
					refresh.Add(1)
					writer.Write([]byte(`{"accessToken":"new-access","refreshToken":"new-refresh"}`))
				default:
					protected.Add(1)
					writer.WriteHeader(scenario.status)
				}
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
				t.Fatal(err)
			}
			_, err = client.doAuthenticatedJSON(context.Background(), scenario.method, "/protected-test", nil, struct{}{}, requiredJSON, scenario.replay)
			if err == nil {
				t.Fatal("unexpected protected success")
			}
			if protected.Load() != scenario.protected || refresh.Load() != scenario.refresh {
				t.Fatalf("unexpected replay counts: protected=%d refresh=%d", protected.Load(), refresh.Load())
			}
			if scenario.protected == 0 && !errors.Is(err, ErrInvalidOptions) {
				t.Fatal("mutation replay should be rejected before HTTP")
			}
		})
	}
}

func TestRefreshFailureCaching(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		switch request.URL.Path {
		case "/v1/auth/tokens":
			writer.Write([]byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
		case "/v1/auth/refreshed-tokens":
			if refreshes.Add(1) == 1 {
				writer.WriteHeader(503)
				return
			}
			writer.Write([]byte(`{"accessToken":"new-access","refreshToken":"new-refresh"}`))
		default:
			writer.WriteHeader(401)
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 4; attempt++ {
		_, err := client.doAuthenticatedJSON(context.Background(), "GET", "/protected-test", nil, struct{}{}, requiredJSON, replayReadOnce)
		var apiError *APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != 503 {
			t.Fatalf("unexpected refresh failure: %v", err)
		}
	}
	if refreshes.Load() != 1 {
		t.Fatal("failed automatic refresh was repeated for the same tokens")
	}
	if err := client.Refresh(context.Background()); err != nil || refreshes.Load() != 2 {
		t.Fatalf("explicit recovery refresh failed: %v", err)
	}
}

func TestCancelledRefreshWaiterAndClose(t *testing.T) {
	for _, closeClient := range []bool{false, true} {
		t.Run(fmtBool(closeClient), func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				io.Copy(io.Discard, request.Body)
				if request.URL.Path == "/v1/auth/tokens" {
					writer.Write([]byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
					return
				}
				close(started)
				select {
				case <-release:
					writer.Write([]byte(`{"accessToken":"new-access","refreshToken":"new-refresh"}`))
				case <-request.Context().Done():
				}
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			first := make(chan error, 1)
			go func() { first <- client.Refresh(ctx) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("refresh did not start")
			}
			if closeClient {
				client.Close()
				select {
				case err := <-first:
					if !errors.Is(err, ErrClientClosed) {
						t.Fatalf("close error=%v", err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("close did not cancel refresh")
				}
				if client.IsAuthenticated() {
					t.Fatal("closed client retained tokens")
				}
			} else {
				snapshot, err := client.snapshot()
				if err != nil {
					t.Fatal(err)
				}
				cancel()
				if err := <-first; !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled waiter error=%v", err)
				}
				second := make(chan error, 1)
				go func() { second <- client.refreshFor(context.Background(), snapshot, false) }()
				close(release)
				select {
				case err := <-second:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("other waiter was poisoned by cancellation")
				}
			}
		})
	}
}

func fmtBool(value bool) string {
	if value {
		return "close"
	}
	return "cancel-waiter"
}

func TestLogoutDuringLogin(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		close(started)
		<-release
		writer.Write([]byte(`{"accessToken":"private-access","refreshToken":"private-refresh"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result := make(chan error, 1)
	go func() { result <- client.Login(context.Background(), "test@example.invalid", "password") }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("login did not start")
	}
	client.Logout()
	close(release)
	if err := <-result; !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("stale login error=%v", err)
	}
	if client.IsAuthenticated() {
		t.Fatal("late login resurrected logged-out session")
	}
}

func TestAuthenticationValidationAndPrivacy(t *testing.T) {
	for _, response := range []string{
		`{"accessToken":"secret-access"}`,
		`{"accessToken":"secret-access","refreshToken":123}`,
		`{"accessToken":"secret\naccess","refreshToken":"secret-refresh"}`,
		`private response not JSON`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			io.Copy(io.Discard, request.Body)
			writer.Write([]byte(response))
		}))
		client, err := NewClient(Config{BaseURL: server.URL, APIKey: "secret-api-key"})
		if err != nil {
			t.Fatal(err)
		}
		err = client.Login(context.Background(), "secret-email@example.invalid", "secret-password")
		if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") || client.IsAuthenticated() {
			t.Fatalf("invalid or secret-bearing authentication result: %v", err)
		}
		client.Close()
		server.Close()
	}
	client, err := NewClient(Config{BaseURL: "http://example.invalid", APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); !errors.Is(err, ErrInvalidOptions) {
		t.Fatal("non-loopback plaintext login was allowed")
	}
	if err := client.Login(context.Background(), "bad:email", "secret-password"); !errors.Is(err, ErrInvalidOptions) || strings.Contains(err.Error(), "secret-password") {
		t.Fatal("invalid credentials were not safely rejected")
	}
}

func TestSessionIsolationAndAccountSwitch(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		switch request.URL.Path {
		case "/v1/auth/tokens":
			email, _, _ := request.BasicAuth()
			name := strings.Split(email, "@")[0]
			json.NewEncoder(writer).Encode(tokenPair{"access-" + name, "refresh-" + name})
		case "/v1/auth/refreshed-tokens":
			refreshes.Add(1)
			writer.WriteHeader(500)
		case "/slow-protected-test":
			close(started)
			select {
			case <-release:
				writer.WriteHeader(401)
			case <-request.Context().Done():
			}
		default:
			account := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer access-")
			json.NewEncoder(writer).Encode(map[string]string{"account": account})
		}
	}))
	defer server.Close()
	first, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx := context.Background()
	if err := first.Login(ctx, "one@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	if err := second.Login(ctx, "two@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		client *Client
		want   string
	}{{first, `{"account":"one"}`}, {second, `{"account":"two"}`}} {
		body, err := pair.client.doAuthenticatedJSON(ctx, "GET", "/protected-test", nil, struct{}{}, requiredJSON, replayReadOnce)
		if err != nil || !jsonEqual(body, []byte(pair.want)) {
			t.Fatal("authentication leaked between client instances")
		}
	}
	result := make(chan error, 1)
	go func() {
		_, err := first.doAuthenticatedJSON(ctx, "GET", "/slow-protected-test", nil, struct{}{}, requiredJSON, replayReadOnce)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("protected read did not start")
	}
	if err := first.Login(ctx, "two@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("old-account request should not replay as new account: %v", err)
	}
	if refreshes.Load() != 0 {
		t.Fatal("account switch caused an automatic refresh/replay")
	}
	first.Logout()
	if !second.IsAuthenticated() {
		t.Fatal("logout affected another client")
	}
}

func TestAuthenticationRedirectRefusal(t *testing.T) {
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { received.Add(1); writer.WriteHeader(500) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "private-key"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	err = client.Login(context.Background(), "private@example.invalid", "private-password")
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 307 || received.Load() != 0 || strings.Contains(err.Error(), "private") {
		t.Fatal("authentication redirect was followed or leaked credentials")
	}
}

func TestRejectedRefreshClearsTokens(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
			return
		}
		refreshes.Add(1)
		writer.WriteHeader(401)
		writer.Write([]byte(`{"message":"private refresh failure"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login(context.Background(), "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	var apiError *APIError
	if err := client.Refresh(context.Background()); !errors.As(err, &apiError) || apiError.StatusCode != 401 {
		t.Fatalf("refresh rejection error=%v", err)
	}
	if client.IsAuthenticated() {
		t.Fatal("rejected refresh retained invalid tokens")
	}
	if err := client.Refresh(context.Background()); !errors.Is(err, ErrAuthenticationRequired) || refreshes.Load() != 1 {
		t.Fatal("invalidated refresh token was reused")
	}
}

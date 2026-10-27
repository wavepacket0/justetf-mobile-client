package justetf

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestAPIKeyDefaultsAndOverrides(t *testing.T) {
	for _, scenario := range []struct {
		name string
		key  string
	}{
		{"app_default", ""},
		{"explicit_override", "test-key-override"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			expected := mobileAPIKey
			if scenario.key != "" {
				expected = scenario.key
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if expected == "" || request.Header.Get("x-api-key") != expected {
					t.Error("request must use the default app key or the explicit override")
				}
				writer.Write([]byte(`{}`))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: scenario.key})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err := client.GetProfile(context.Background(), "IE00B5BMR087"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v2/exchange-traded-funds/profiles/IE00B5BMR087" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("currency") != "EUR" || request.URL.Query().Get("universeCountry") != "IT" || request.URL.Query().Get("universeType") != "PRIVATE" {
			t.Errorf("unexpected query: %s", request.URL.RawQuery)
		}
		for header, expected := range map[string]string{
			"x-api-key":       "test-key",
			"accept-language": "it",
			"user-agent":      "Dart/3.12 (dart:io)",
			"content-type":    "application/json",
			"accept-encoding": "gzip",
			"content-length":  "2",
		} {
			if actual := request.Header.Get(header); actual != expected {
				t.Errorf("%s: got %q, want %q", header, actual, expected)
			}
		}
		if _, exists := request.Header["Accept"]; exists {
			t.Error("unexpected Accept header, absent from mobile capture")
		}
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(request.Header.Get("x-request-id")) {
			t.Error("expected UUIDv4 request ID")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || string(body) != "{}" {
			t.Errorf("captured profile GET body should be {}, got %q (error=%v)", body, err)
		}
		writer.Write([]byte(`{"isin":"IE00B5BMR087","name":"Example ETF"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := client.GetProfile(context.Background(), "IE00B5BMR087")
	if err != nil || string(profile) != `{"isin":"IE00B5BMR087","name":"Example ETF"}` {
		t.Fatalf("profile=%s, error=%v", profile, err)
	}
	if _, err := client.GetProfile(context.Background(), "../invalid"); err == nil {
		t.Fatal("expected invalid ISIN to be rejected")
	}
}

func TestProfileErrors(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"secret":"must not be exposed"}`},
		{"invalid-json", http.StatusOK, "not JSON"},
		{"redirect", http.StatusFound, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(scenario.status)
				writer.Write([]byte(scenario.body))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-key"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetProfile(context.Background(), "IE00B5BMR087")
			if err == nil {
				t.Fatal("expected an error")
			}
			if scenario.status != http.StatusOK {
				var apiError *APIError
				if !errors.As(err, &apiError) || apiError.StatusCode != scenario.status {
					t.Fatalf("unexpected API error: %v", err)
				}
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (transport roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestTransportIndependentOfDefault(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Error("client should not use the global default transport")
		return nil, errors.New("unexpected global transport call")
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if _, err := client.GetProfile(context.Background(), "IE00B5BMR087"); err != nil {
		t.Fatal(err)
	}
}

func TestJSONExecutor(t *testing.T) {
	requestIDs := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/executor-test" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("query") != "ETF & index" || len(request.URL.Query()) != 1 {
			t.Errorf("incorrect query encoding or unexpected global parameters: %s", request.URL.RawQuery)
		}
		if request.Header.Get("accept-language") != "fr" || request.Header.Get("user-agent") != "Dart/3.12 (dart:io)" {
			t.Error("executor did not apply common headers")
		}
		requestIDs <- request.Header.Get("x-request-id")
		body, err := io.ReadAll(request.Body)
		var decoded []map[string]string
		if err != nil || json.Unmarshal(body, &decoded) != nil || len(decoded) != 1 || decoded[0]["value"] != "ETF & index" {
			t.Errorf("incorrect JSON request body: %q, error=%v", body, err)
		}
		writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	for attempt := 0; attempt < 2; attempt++ {
		result, err := client.doJSON(context.Background(), http.MethodPost, "/executor-test",
			url.Values{"query": {"ETF & index"}}, []map[string]string{{"value": "ETF & index"}}, requiredJSON)
		if err != nil || string(result) != `{"ok":true}` {
			t.Fatalf("result=%s, error=%v", result, err)
		}
	}
	if first, second := <-requestIDs, <-requestIDs; first == second {
		t.Error("request ID was reused")
	}
	if _, err := client.doJSON(context.Background(), http.MethodPost, "/executor-test", nil, make(chan int), requiredJSON); err == nil {
		t.Fatal("expected unsupported JSON body to fail before making a request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.doJSON(ctx, http.MethodPost, "/executor-test", nil, nil, requiredJSON); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestJSONExecutorGzip(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"json", `{"name":"Example ETF"}`, false},
		{"decompressed-limit", `"` + strings.Repeat("x", maxResponseBytes) + `"`, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("accept-encoding") != "gzip" {
					t.Error("expected gzip negotiation")
				}
				writer.Header().Set("Content-Encoding", "gzip")
				compressed := gzip.NewWriter(writer)
				compressed.Write([]byte(scenario.body))
				compressed.Close()
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-key"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			result, err := client.GetProfile(context.Background(), "IE00B5BMR087")
			if scenario.wantErr {
				if err == nil || err.Error() != "API response exceeds 4 MiB" {
					t.Fatalf("expected decompressed response limit, got %v", err)
				}
			} else if err != nil || string(result) != scenario.body {
				t.Fatalf("result=%s, error=%v", result, err)
			}
		})
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftCanonical, _ := json.Marshal(leftValue)
	rightCanonical, _ := json.Marshal(rightValue)
	return string(leftCanonical) == string(rightCanonical)
}

func TestHTTPResponsePolicies(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		policy  responsePolicy
		status  int
		body    string
		wantErr bool
	}{
		{"required-object", requiredJSON, 200, `{"value":1.234567890123456789}`, false},
		{"required-array", requiredJSON, 200, `[]`, false},
		{"required-null", requiredJSON, 200, `null`, false},
		{"required-empty-200", requiredJSON, 200, "", true},
		{"required-empty-204", requiredJSON, 204, "", true},
		{"required-invalid", requiredJSON, 200, "not JSON", true},
		{"optional-empty-200", optionalJSON, 200, "", false},
		{"optional-empty-204", optionalJSON, 204, "", false},
		{"optional-object", optionalJSON, 201, `{"ok":true}`, false},
		{"optional-invalid", optionalJSON, 200, "not JSON", true},
		{"optional-whitespace", optionalJSON, 200, " \n", true},
		{"void-empty-200", emptySuccess, 200, "", false},
		{"void-empty-204", emptySuccess, 204, "", false},
		{"void-object", emptySuccess, 200, `{}`, true},
		{"void-null", emptySuccess, 200, `null`, true},
		{"void-whitespace", emptySuccess, 200, " ", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(scenario.status)
				writer.Write([]byte(scenario.body))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			result, err := client.doJSON(context.Background(), http.MethodGet, "/policy-test", nil, nil, scenario.policy)
			if (err != nil) != scenario.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, scenario.wantErr)
			}
			if err == nil && string(result) != scenario.body {
				t.Fatal("response bytes were changed")
			}
			if err == nil && scenario.body == "" && result != nil {
				t.Fatal("empty success should return nil")
			}
		})
	}
}

func TestResponsePolicyValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	for _, policy := range []responsePolicy{requiredJSON, optionalJSON, emptySuccess} {
		_, err := client.doJSON(context.Background(), http.MethodGet, "/policy-test", nil, nil, policy)
		var apiError *APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusForbidden {
			t.Fatalf("HTTP failure incorrectly accepted for policy %d: %v", policy, err)
		}
	}
	if _, err := client.doJSON(context.Background(), http.MethodGet, "/policy-test", nil, nil, responsePolicy(255)); err == nil || err.Error() != "invalid response policy" {
		t.Fatalf("invalid policy was not rejected before execution: %v", err)
	}
}

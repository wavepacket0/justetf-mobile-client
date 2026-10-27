package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

func TestAuthenticationDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			user, password, ok := request.BasicAuth()
			if !ok || user != "test@example.invalid" || password != "private-password" {
				t.Error("incorrect login request")
			}
		} else if request.URL.Path != "/v1/auth/refreshed-tokens" || request.Header.Get("Authorization") != "Bearer private-refresh" {
			t.Error("incorrect refresh request")
		}
		writer.Write([]byte(`{"accessToken":"private-access","refreshToken":"private-refresh"}`))
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	for _, scenario := range []struct{ operation, params, want string }{
		{"is_authenticated", `{}`, `false`},
		{"login", `{"email":"test@example.invalid","password":"private-password"}`, ""},
		{"is_authenticated", `{}`, `true`},
		{"refresh", `{}`, ""},
		{"logout", `{}`, ""},
		{"is_authenticated", `{}`, `false`},
	} {
		result, err := dispatch(ctx, client, scenario.operation, []byte(scenario.params))
		if err != nil || string(result) != scenario.want {
			t.Fatalf("%s returned unexpected result/error: %v", scenario.operation, err)
		}
		if strings.Contains(string(result), "private") {
			t.Fatal("native result exposed credentials or tokens")
		}
	}
	if _, err := dispatch(ctx, client, "refresh", []byte(`{}`)); !errors.Is(err, justetf.ErrAuthenticationRequired) {
		t.Fatalf("missing session error=%v", err)
	}
	for _, scenario := range []struct{ operation, params string }{
		{"login", `{"email":"test@example.invalid","password":123}`},
		{"login", `{"email":"test@example.invalid","password":"private-password","token":"private-token"}`},
		{"logout", `{"token":"private-token"}`},
	} {
		_, err := dispatch(ctx, client, scenario.operation, []byte(scenario.params))
		if !errors.Is(err, errInvalidArguments) || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid login arguments were not safely rejected")
		}
	}
	if err := client.Login(ctx, "test@example.invalid", "private-password"); err != nil {
		t.Fatal(err)
	}
	handle := storeClient(client)
	if !closeClient(handle) || client.IsAuthenticated() {
		t.Fatal("native close did not clear authentication")
	}
}

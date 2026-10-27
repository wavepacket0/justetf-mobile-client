package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

func TestSessionCloseCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	handle := storeClient(client)
	defer closeClient(handle)
	current := findClient(handle)
	result := make(chan error, 1)
	go func() {
		_, err := current.client.GetProfile(current.ctx, "IE00B5BMR087")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	if !closeClient(handle) || closeClient(handle) || findClient(handle) != nil {
		t.Fatal("closed handle remains accessible")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, justetf.ErrClientClosed) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close did not cancel request")
	}
}

func TestConcurrentSessionRegistry(t *testing.T) {
	client, err := justetf.NewClient(justetf.Config{APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := 0; iteration < 64; iteration++ {
				handle := storeClient(client)
				if findClient(handle) == nil {
					t.Error("missing live handle")
				}
				closeClient(handle)
				if findClient(handle) != nil {
					t.Error("stale handle resolved")
				}
			}
		}()
	}
	workers.Wait()
}

func BenchmarkSessionLookup(b *testing.B) {
	client, err := justetf.NewClient(justetf.Config{APIKey: "test-key"})
	if err != nil {
		b.Fatal(err)
	}
	handle := storeClient(client)
	defer closeClient(handle)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(worker *testing.PB) {
		for worker.Next() {
			findClient(handle)
		}
	})
}

func TestDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/exchange-traded-funds/profiles/IE00B5BMR087" {
			t.Error("dispatcher did not call the typed profile method")
		}
		io.Copy(io.Discard, request.Body)
		writer.Write([]byte(`{"isin":"IE00B5BMR087","value":1.234567890123456789}`))
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	result, err := dispatch(context.Background(), client, "get_profile", []byte(`{"isin":"IE00B5BMR087"}`))
	if err != nil || string(result) != `{"isin":"IE00B5BMR087","value":1.234567890123456789}` {
		t.Fatalf("result=%s, error=%v", result, err)
	}
	for _, scenario := range []struct {
		operation string
		params    string
	}{
		{"https://private.invalid/", `{}`},
		{"tracking", `{}`},
		{"get_profile", `[]`},
		{"get_profile", `null`},
		{"get_profile", `{"isin":123}`},
		{"get_profile", `{"isin":"IE00B5BMR087","private_field":"secret"}`},
		{"get_profile", `{"isin":"IE00B5BMR087"} {}`},
		{"get_profile", `{"isin":`},
	} {
		_, err := dispatch(context.Background(), client, scenario.operation, []byte(scenario.params))
		if !errors.Is(err, errInvalidArguments) || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected safe invalid-argument error, got %v", err)
		}
	}
	if _, err := dispatch(context.Background(), client, "get_profile", []byte(`{}`)); !errors.Is(err, justetf.ErrInvalidISIN) {
		t.Fatalf("missing ISIN should use the typed client validation: %v", err)
	}
}

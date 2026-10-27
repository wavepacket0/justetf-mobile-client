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
)

func TestAccountReadsRefreshAndIsolation(t *testing.T) {
	const workers = 8
	var expired, refreshes atomic.Int32
	allExpired := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		switch request.URL.Path {
		case "/v1/auth/tokens":
			email, _, _ := request.BasicAuth()
			owner := strings.Split(email, "@")[0]
			json.NewEncoder(writer).Encode(tokenPair{owner + "-old", owner + "-refresh"})
		case "/v1/auth/refreshed-tokens":
			refreshes.Add(1)
			if request.Header.Get("Authorization") != "Bearer one-refresh" {
				t.Error("refresh token crossed clients")
			}
			select {
			case <-allExpired:
			case <-request.Context().Done():
				return
			}
			writer.Write([]byte(`{"accessToken":"one-new","refreshToken":"one-refresh-new"}`))
		default:
			authorization := request.Header.Get("Authorization")
			if authorization == "Bearer one-old" {
				if expired.Add(1) == workers {
					close(allExpired)
				}
				writer.WriteHeader(401)
				return
			}
			owner := "one"
			if authorization == "Bearer two-old" {
				owner = "two"
			} else if authorization != "Bearer one-new" {
				t.Error("incorrect protected read token")
			}
			json.NewEncoder(writer).Encode(map[string]any{"owner": owner, "watchlists": []any{}, "nullable": nil})
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
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := first.GetWatchlists(ctx, WatchlistOptions{})
			if err != nil || !jsonEqual(result, []byte(`{"owner":"one","watchlists":[],"nullable":null}`)) {
				t.Errorf("parallel account read failed: %v", err)
			}
		}()
	}
	wait.Wait()
	if refreshes.Load() != 1 {
		t.Fatal("parallel account reads did not coalesce refresh")
	}
	first.Logout()
	result, err := second.GetWatchlists(ctx, WatchlistOptions{})
	if err != nil || !jsonEqual(result, []byte(`{"owner":"two","watchlists":[],"nullable":null}`)) {
		t.Fatal("another client's session or empty data was changed")
	}
}

func TestAccountOptionValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("invalid account options reached HTTP")
		writer.WriteHeader(500)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	for _, id := range []string{"", "../123", "can-create", "trade-quote", "0", "0001", "123?query=private", "123/456"} {
		if _, err := client.GetPortfolio(ctx, id, PortfolioOptions{}); !errors.Is(err, ErrInvalidOptions) {
			t.Fatal("unsafe or ambiguous portfolio ID accepted")
		}
	}
	for _, call := range []func() (json.RawMessage, error){
		func() (json.RawMessage, error) { return client.GetPriceAlert(ctx, "IE00B5BMR087", "../private") },
		func() (json.RawMessage, error) {
			return client.GetPortfolios(ctx, PortfolioListOptions{PageOptions: PageOptions{Limit: financialPtr(0)}})
		},
		func() (json.RawMessage, error) {
			return client.GetPortfolioTransactions(ctx, "123456", PageOptions{Page: -1})
		},
		func() (json.RawMessage, error) {
			return client.GetPortfolioAllocation(ctx, "123456", AllocationOptions{Offset: -1})
		},
		func() (json.RawMessage, error) { return client.GetTradeQuote(ctx, "IE00B5BMR087", TradeQuoteOptions{}) },
		func() (json.RawMessage, error) {
			return client.GetTradeQuote(ctx, "IE00B5BMR087", TradeQuoteOptions{Date: "2026-02-30"})
		},
		func() (json.RawMessage, error) {
			return client.GetPortfolioPerformanceChart(ctx, "123456", PortfolioChartOptions{DateFrom: financialPtr("bad")})
		},
	} {
		if _, err := call(); !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("invalid account options accepted: %v", err)
		}
	}
	if _, err := client.GetHoldingSupplementaryData(ctx, "../private"); !errors.Is(err, ErrInvalidISIN) {
		t.Fatal("invalid supplementary instrument accepted")
	}
}

func TestAccountPrecisionAndOverrides(t *testing.T) {
	const payload = `{"value":1.234567890123456789,"date":"2026-10-14","nullable":null,"futureField":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"test-access","refreshToken":"test-refresh"}`))
			return
		}
		query := request.URL.Query()
		switch {
		case strings.HasSuffix(request.URL.Path, "/portfolios"):
			for key, value := range map[string]string{"period": "Y1", "appVersion": "5.0", "limit": "7", "page": "2"} {
				if query.Get(key) != value {
					t.Errorf("incorrect portfolio option %s", key)
				}
			}
		case strings.HasSuffix(request.URL.Path, "/performance-chart"):
			if query.Has("includeDividends") || query.Has("appVersion") || query.Get("reduceData") != "false" || query.Get("currency") != "USD" || query.Get("dateFrom") != "2026-01-01" || query.Get("dateTo") != "2026-10-01" {
				t.Error("portfolio chart options were changed")
			}
			if !strings.Contains("&"+request.URL.RawQuery+"&", "&period&") {
				t.Error("bare portfolio period lost")
			}
		case strings.HasSuffix(request.URL.Path, "/trade-quote"):
			if query.Get("date") != "2026-10-14" || query.Get("currency") != "USD" {
				t.Error("trade quote overrides lost")
			}
		}
		writer.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Login(ctx, "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() (json.RawMessage, error){
		func() (json.RawMessage, error) {
			return client.GetPortfolios(ctx, PortfolioListOptions{PageOptions: PageOptions{Limit: financialPtr(7), Page: 2}, Period: "Y1", AppVersion: "5.0"})
		},
		func() (json.RawMessage, error) {
			return client.GetPortfolioPerformanceChart(ctx, "123456", PortfolioChartOptions{CurrencyOptions: CurrencyOptions{Currency: "USD"}, DateFrom: financialPtr("2026-01-01"), DateTo: financialPtr("2026-10-01"), Period: financialPtr(""), ReduceData: financialPtr(false)})
		},
		func() (json.RawMessage, error) {
			return client.GetTradeQuote(ctx, "IE00B5BMR087", TradeQuoteOptions{CurrencyOptions: CurrencyOptions{Currency: "USD"}, Date: "2026-10-14"})
		},
	} {
		result, err := call()
		if err != nil || string(result) != payload {
			t.Fatalf("account response precision or fields changed: %v", err)
		}
	}
}

package justetf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func mutationFixtureTransaction() ShareTransactionRequest {
	return ShareTransactionRequest{TradeDate: "2026-10-14", TransactionType: "DELIVERY", Amount: "123.450000000000000001", ISIN: "IE00B5BMR087", Quantity: "1.000000000000000001", Quote: "123.45", QuoteCurrency: "EUR", InstrumentCurrency: "EUR", PortfolioCurrency: "EUR", QuoteCurrencyType: "INSTRUMENT", TradeRate: "1", FundRate: "1", Fees: "0", Taxes: "0", Comment: "synthetic transaction"}
}

func exactMutationJSON(left, right []byte) bool {
	var leftValue, rightValue any
	leftDecoder := json.NewDecoder(bytes.NewReader(left))
	leftDecoder.UseNumber()
	rightDecoder := json.NewDecoder(bytes.NewReader(right))
	rightDecoder.UseNumber()
	return leftDecoder.Decode(&leftValue) == nil && rightDecoder.Decode(&rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func TestMutationNoReplay(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		drop   bool
		status int
	}{{"unauthorized", false, 401}, {"ambiguous-response", true, 0}} {
		t.Run(scenario.name, func(t *testing.T) {
			var writes, refreshes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				io.Copy(io.Discard, request.Body)
				if request.URL.Path == "/v1/auth/tokens" {
					writer.Write([]byte(`{"accessToken":"test-access","refreshToken":"test-refresh"}`))
					return
				}
				if request.URL.Path == "/v1/auth/refreshed-tokens" {
					refreshes.Add(1)
					writer.WriteHeader(500)
					return
				}
				writes.Add(1)
				if scenario.drop {
					connection, _, err := writer.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					connection.Close()
					return
				}
				writer.WriteHeader(scenario.status)
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
			_, err = client.CreatePortfolio(context.Background(), CreatePortfolioRequest{Name: "Synthetic", Investment: "ONE_TIME"})
			if err == nil || writes.Load() != 1 || refreshes.Load() != 0 {
				t.Fatal("mutation was replayed/refreshed or ambiguously succeeded")
			}
		})
	}
}

func TestMutationLifecycle(t *testing.T) {
	var state sync.Mutex
	var alert, filter, portfolio, transaction json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"test-access","refreshToken":"test-refresh"}`))
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-access" {
			t.Error("lifecycle mutation lost access token")
		}
		state.Lock()
		defer state.Unlock()
		switch request.Method + " " + request.URL.Path {
		case "POST /v1/price-alerts/etfs/IE00B5BMR087/alerts":
			alert = append(json.RawMessage(nil), body...)
			writer.Write(alert)
		case "GET /v1/price-alerts/etfs/IE00B5BMR087/alerts/test-alert":
			if alert == nil {
				writer.WriteHeader(404)
			} else {
				writer.Write(alert)
			}
		case "DELETE /v1/price-alerts/etfs/IE00B5BMR087/alerts/test-alert":
			alert = nil
		case "POST /v1/exchange-traded-funds/profiles/integrations/saved-filters":
			filter = append(json.RawMessage(nil), body...)
		case "GET /v1/exchange-traded-funds/profiles/integrations/saved-filters":
			items := []json.RawMessage{}
			if filter != nil {
				items = append(items, filter)
			}
			json.NewEncoder(writer).Encode(map[string]any{"savedFilters": items})
		case "DELETE /v1/exchange-traded-funds/profiles/integrations/saved-filters/test-filter":
			filter = nil
		case "POST /v1/exchange-traded-funds/portfolios":
			portfolio = append(json.RawMessage(nil), body...)
			writer.WriteHeader(201)
			writer.Write(portfolio)
		case "GET /v1/exchange-traded-funds/portfolios/123456":
			if portfolio == nil {
				writer.WriteHeader(404)
			} else {
				writer.Write(portfolio)
			}
		case "POST /v1/exchange-traded-funds/portfolios/123456/transactions/share":
			transaction = append(json.RawMessage(nil), body...)
			writer.Write(transaction)
		case "GET /v1/exchange-traded-funds/profiles/integrations/portfolios/123456/transactions":
			json.NewEncoder(writer).Encode(map[string]any{"transactions": []json.RawMessage{transaction}})
		case "DELETE /v1/exchange-traded-funds/portfolios/123456":
			portfolio = nil
			writer.WriteHeader(204)
		default:
			t.Error("unexpected mock lifecycle route")
			writer.WriteHeader(404)
		}
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
	created, err := client.CreatePriceAlert(ctx, "IE00B5BMR087", PriceAlertRequest{Price: "123.450000000000000001", Condition: "GTE"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := client.GetPriceAlert(ctx, "IE00B5BMR087", "test-alert")
	if err != nil || !exactMutationJSON(created, read) {
		t.Fatal("created alert not readable")
	}
	if err := client.DeletePriceAlert(ctx, "IE00B5BMR087", "test-alert"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetPriceAlert(ctx, "IE00B5BMR087", "test-alert"); err == nil {
		t.Fatal("deleted alert remained readable")
	}
	if err := client.CreateSavedFilter(ctx, SavedFilterRequest{Label: "Synthetic"}); err != nil {
		t.Fatal(err)
	}
	filters, err := client.GetSavedFilters(ctx)
	if err != nil || !strings.Contains(string(filters), `"filters":[]`) {
		t.Fatal("saved filters lost nil-to-array semantics")
	}
	if err := client.DeleteSavedFilter(ctx, "test-filter"); err != nil {
		t.Fatal(err)
	}
	filters, err = client.GetSavedFilters(ctx)
	if err != nil || !exactMutationJSON(filters, []byte(`{"savedFilters":[]}`)) {
		t.Fatal("deleted filter remained")
	}
	created, err = client.CreatePortfolio(ctx, CreatePortfolioRequest{Name: "Synthetic", Investment: "ONE_TIME"})
	if err != nil {
		t.Fatal(err)
	}
	read, err = client.GetPortfolio(ctx, "123456", PortfolioOptions{})
	if err != nil || !exactMutationJSON(created, read) {
		t.Fatal("created portfolio not readable")
	}
	if _, err := client.AddShareTransaction(ctx, "123456", mutationFixtureTransaction()); err != nil {
		t.Fatal(err)
	}
	transactions, err := client.GetPortfolioTransactions(ctx, "123456", PageOptions{})
	if err != nil || !strings.Contains(string(transactions), "123.450000000000000001") {
		t.Fatal("recorded transaction lost numeric precision")
	}
	if err := client.DeletePortfolio(ctx, "123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetPortfolio(ctx, "123456", PortfolioOptions{}); err == nil {
		t.Fatal("deleted portfolio remained readable")
	}
}

func TestMutationValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("invalid mutation reached HTTP")
		writer.WriteHeader(500)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	badNumber := mutationFixtureTransaction()
	badNumber.Amount = "NaN"
	badDate := mutationFixtureTransaction()
	badDate.TradeDate = "2026-02-30"
	for _, call := range []func() error{
		func() error {
			_, err := client.CreatePriceAlert(ctx, "IE00B5BMR087", PriceAlertRequest{Price: "0", Condition: "GTE"})
			return err
		},
		func() error {
			_, err := client.CreatePriceAlert(ctx, "IE00B5BMR087", PriceAlertRequest{Price: "1e3", Condition: "GTE"})
			return err
		},
		func() error { return client.DeletePriceAlert(ctx, "IE00B5BMR087", "../private") },
		func() error { return client.CreateSavedFilter(ctx, SavedFilterRequest{}) },
		func() error { return client.DeleteSavedFilter(ctx, "../private") },
		func() error {
			_, err := client.CreatePortfolio(ctx, CreatePortfolioRequest{Name: "Synthetic"})
			return err
		},
		func() error { return client.DeletePortfolio(ctx, "can-create") },
		func() error { _, err := client.AddShareTransaction(ctx, "123456", badNumber); return err },
		func() error { _, err := client.AddShareTransaction(ctx, "123456", badDate); return err },
	} {
		if err := call(); !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("invalid mutation not rejected locally: %v", err)
		}
	}
}

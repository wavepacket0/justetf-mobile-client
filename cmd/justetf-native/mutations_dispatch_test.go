package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

func TestMutationDispatchParity(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"test-access","refreshToken":"test-refresh"}`))
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-access" {
			t.Error("native mutation lost access token")
		}
		requests <- request.Method + " " + request.URL.Path + " " + string(body)
		if request.Method == "DELETE" {
			if strings.Contains(request.URL.Path, "/portfolios/") {
				writer.WriteHeader(204)
			}
			return
		}
		if strings.HasSuffix(request.URL.Path, "/saved-filters") {
			return
		}
		writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Login(ctx, "test@example.invalid", "password"); err != nil {
		t.Fatal(err)
	}
	transaction := justetf.ShareTransactionRequest{TradeDate: "2026-10-14", TransactionType: "DELIVERY", Amount: "100.000000000000000001", ISIN: "IE00B5BMR087", Quantity: "1", Quote: "100", QuoteCurrency: "EUR", InstrumentCurrency: "EUR", PortfolioCurrency: "EUR", QuoteCurrencyType: "INSTRUMENT", TradeRate: "1", FundRate: "1", Fees: "0", Taxes: "0"}
	encoded, err := json.Marshal(transaction)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&params); err != nil {
		t.Fatal(err)
	}
	params["portfolio_id"] = "123456"
	transactionParams, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		operation, parameters string
		direct                func() (json.RawMessage, error)
	}{
		{"create_price_alert", `{"isin":"IE00B5BMR087","price":"123.450000000000000001","condition":"GTE"}`, func() (json.RawMessage, error) {
			return client.CreatePriceAlert(ctx, "IE00B5BMR087", justetf.PriceAlertRequest{Price: "123.450000000000000001", Condition: "GTE"})
		}},
		{"delete_price_alert", `{"isin":"IE00B5BMR087","alert_id":"test-alert"}`, func() (json.RawMessage, error) {
			return nil, client.DeletePriceAlert(ctx, "IE00B5BMR087", "test-alert")
		}},
		{"create_saved_filter", `{"label":"Synthetic","saved_filter_data":{"filters":["one"],"keywords":"world"}}`, func() (json.RawMessage, error) {
			return nil, client.CreateSavedFilter(ctx, justetf.SavedFilterRequest{Label: "Synthetic", SavedFilterData: justetf.SavedFilterData{Filters: []string{"one"}, Keywords: "world"}})
		}},
		{"delete_saved_filter", `{"filter_id":"test-filter"}`, func() (json.RawMessage, error) { return nil, client.DeleteSavedFilter(ctx, "test-filter") }},
		{"create_portfolio", `{"name":"Synthetic","investment":"ONE_TIME"}`, func() (json.RawMessage, error) {
			return client.CreatePortfolio(ctx, justetf.CreatePortfolioRequest{Name: "Synthetic", Investment: "ONE_TIME"})
		}},
		{"delete_portfolio", `{"portfolio_id":"123456"}`, func() (json.RawMessage, error) { return nil, client.DeletePortfolio(ctx, "123456") }},
		{"add_share_transaction", string(transactionParams), func() (json.RawMessage, error) { return client.AddShareTransaction(ctx, "123456", transaction) }},
	} {
		t.Run(scenario.operation, func(t *testing.T) {
			expected, err := scenario.direct()
			if err != nil {
				t.Fatal(err)
			}
			actual, err := dispatch(ctx, client, scenario.operation, []byte(scenario.parameters))
			if err != nil || string(actual) != string(expected) {
				t.Fatalf("mutation dispatch result differs: %v", err)
			}
			first, second := <-requests, <-requests
			if first != second {
				t.Fatal("mutation dispatch request differs from direct Go")
			}
		})
	}
	for _, scenario := range []struct {
		operation, parameters string
		target                error
	}{
		{"create_price_alert", `{"isin":"IE00B5BMR087","price":true,"condition":"GTE"}`, errInvalidArguments},
		{"delete_saved_filter", `{"filter_id":"../private"}`, justetf.ErrInvalidOptions},
		{"create_saved_filter", `{"label":"Synthetic","saved_filter_data":{"filters":[123]}}`, errInvalidArguments},
		{"create_portfolio", `{"name":"Synthetic","investment":123}`, errInvalidArguments},
		{"delete_portfolio", `{"portfolio_id":123456}`, errInvalidArguments},
		{"add_share_transaction", `{"portfolio_id":"123456","unverified_field":"private"}`, errInvalidArguments},
	} {
		_, err := dispatch(ctx, client, scenario.operation, []byte(scenario.parameters))
		if !errors.Is(err, scenario.target) {
			t.Fatalf("unsafe mutation parameters were not rejected: %v", err)
		}
	}
}

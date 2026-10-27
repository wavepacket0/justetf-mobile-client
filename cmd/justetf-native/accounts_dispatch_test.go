package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

func TestAccountDispatchParity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.URL.Path == "/v1/auth/tokens" {
			writer.Write([]byte(`{"accessToken":"test-access","refreshToken":"test-refresh"}`))
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-access" {
			t.Error("native account dispatch lost authorization")
		}
		json.NewEncoder(writer).Encode(map[string]any{"method": request.Method, "path": request.URL.Path, "query": request.URL.Query(), "body": json.RawMessage(body)})
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
	limit := 7
	for _, scenario := range []struct {
		operation, params string
		direct            func() (json.RawMessage, error)
	}{
		{"get_personal_data", `{"country":"FR"}`, func() (json.RawMessage, error) { return client.GetPersonalData(ctx, "FR") }},
		{"get_watchlists", `{"currency":"USD","returns_period":"M6"}`, func() (json.RawMessage, error) {
			return client.GetWatchlists(ctx, justetf.WatchlistOptions{CurrencyOptions: justetf.CurrencyOptions{Currency: "USD"}, ReturnsPeriod: "M6"})
		}},
		{"get_saved_filters", `{}`, func() (json.RawMessage, error) { return client.GetSavedFilters(ctx) }},
		{"get_price_alerts", `{}`, func() (json.RawMessage, error) { return client.GetPriceAlerts(ctx) }},
		{"get_etf_price_alerts", `{"isin":"IE00B5BMR087"}`, func() (json.RawMessage, error) { return client.GetETFPriceAlerts(ctx, "IE00B5BMR087") }},
		{"get_price_alert", `{"isin":"IE00B5BMR087","alert_id":"test-alert"}`, func() (json.RawMessage, error) { return client.GetPriceAlert(ctx, "IE00B5BMR087", "test-alert") }},
		{"get_portfolios", `{"limit":7,"page":2,"period":"Y1"}`, func() (json.RawMessage, error) {
			return client.GetPortfolios(ctx, justetf.PortfolioListOptions{PageOptions: justetf.PageOptions{Limit: &limit, Page: 2}, Period: "Y1"})
		}},
		{"get_portfolio", `{"portfolio_id":"123456"}`, func() (json.RawMessage, error) { return client.GetPortfolio(ctx, "123456", justetf.PortfolioOptions{}) }},
		{"get_portfolio_composition", `{"portfolio_id":"123456","period":"Y1"}`, func() (json.RawMessage, error) {
			return client.GetPortfolioComposition(ctx, "123456", justetf.PortfolioCompositionOptions{Period: "Y1"})
		}},
		{"can_create_portfolio", `{}`, func() (json.RawMessage, error) { return client.CanCreatePortfolio(ctx) }},
		{"get_trade_quote", `{"isin":"IE00B5BMR087","date":"2026-10-14"}`, func() (json.RawMessage, error) {
			return client.GetTradeQuote(ctx, "IE00B5BMR087", justetf.TradeQuoteOptions{Date: "2026-10-14"})
		}},
		{"get_portfolio_transactions", `{"portfolio_id":"123456","limit":7,"page":2}`, func() (json.RawMessage, error) {
			return client.GetPortfolioTransactions(ctx, "123456", justetf.PageOptions{Limit: &limit, Page: 2})
		}},
		{"get_portfolio_allocation", `{"portfolio_id":"123456","grouping":"SECTORS","offset":2}`, func() (json.RawMessage, error) {
			return client.GetPortfolioAllocation(ctx, "123456", justetf.AllocationOptions{Grouping: "SECTORS", Offset: 2})
		}},
		{"get_portfolio_performance_chart", `{"portfolio_id":"123456"}`, func() (json.RawMessage, error) {
			return client.GetPortfolioPerformanceChart(ctx, "123456", justetf.PortfolioChartOptions{})
		}},
		{"get_holding_supplementary_data", `{"isin":"IE00B5BMR087"}`, func() (json.RawMessage, error) { return client.GetHoldingSupplementaryData(ctx, "IE00B5BMR087") }},
	} {
		t.Run(scenario.operation, func(t *testing.T) {
			expected, err := scenario.direct()
			if err != nil {
				t.Fatal(err)
			}
			actual, err := dispatch(ctx, client, scenario.operation, []byte(scenario.params))
			if err != nil || string(actual) != string(expected) {
				t.Fatalf("account dispatch differs from typed Go: %v", err)
			}
		})
	}
	for _, scenario := range []struct {
		operation, params string
		target            error
	}{
		{"get_portfolio", `{"portfolio_id":123456}`, errInvalidArguments},
		{"get_portfolio", `{"portfolio_id":"../private"}`, justetf.ErrInvalidOptions},
		{"get_portfolio_performance_chart", `{"portfolio_id":"123456","include_dividends":true}`, errInvalidArguments},
		{"get_trade_quote", `{"isin":"IE00B5BMR087"}`, justetf.ErrInvalidOptions},
		{"get_portfolio_transactions", `{"portfolio_id":"123456","page":-1}`, justetf.ErrInvalidOptions},
	} {
		_, err := dispatch(ctx, client, scenario.operation, []byte(scenario.params))
		if !errors.Is(err, scenario.target) {
			t.Fatalf("incorrect account validation error: %v", err)
		}
	}
}

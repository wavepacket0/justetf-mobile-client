package justetf

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBareFinancialQueryParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		parts := strings.Split(request.URL.RawQuery, "&")
		for _, expected := range []string{"dateFrom", "dateTo=", "period", "tag=one", "tag=two", "other+key"} {
			found := false
			for _, part := range parts {
				if part == expected {
					found = true
				}
			}
			if !found {
				t.Errorf("query form %q missing from %s", expected, request.URL.RawQuery)
			}
		}
		if len(parts) != 6 {
			t.Errorf("unexpected query parts: %v", parts)
		}
		writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	query := url.Values{"dateFrom": nil, "dateTo": {""}, "period": nil, "tag": {"one", "two"}, "other key": nil}
	if _, err := client.doJSON(context.Background(), http.MethodGet, "/chart-query-test", query, struct{}{}, requiredJSON); err != nil {
		t.Fatal(err)
	}
}

func financialPtr[Value any](value Value) *Value { return &value }

func TestFinancialOptionValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("invalid options reached HTTP")
		writer.WriteHeader(500)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx := context.Background()
	isin := "IE00B5BMR087"
	for _, call := range []func() (json.RawMessage, error){
		func() (json.RawMessage, error) {
			return client.GetPerformanceChart(ctx, isin, ChartOptions{DateFrom: financialPtr("2026-02-30")})
		},
		func() (json.RawMessage, error) {
			return client.GetPerformanceChart(ctx, isin, ChartOptions{DateFrom: financialPtr("2026-10-02"), DateTo: financialPtr("2026-10-01")})
		},
		func() (json.RawMessage, error) {
			return client.GetCountryExposure(ctx, isin, ExposureOptions{Top: financialPtr(0)})
		},
		func() (json.RawMessage, error) {
			return client.GetSectorExposure(ctx, isin, ExposureOptions{Scale: financialPtr(-1)})
		},
		func() (json.RawMessage, error) {
			return client.GetHoldingExposure(ctx, isin, ExposureOptions{Threshold: "NaN"})
		},
		func() (json.RawMessage, error) { return client.GetHoldingETFs(ctx, isin, HoldingETFsOptions{Page: -1}) },
		func() (json.RawMessage, error) {
			return client.GetIndexPerformance(ctx, IndexPerformanceOptions{Limit: financialPtr(0)})
		},
		func() (json.RawMessage, error) {
			return client.GetSavingsPlans(ctx, isin, SavingsPlansOptions{SavingsRate: "0.00"})
		},
		func() (json.RawMessage, error) {
			return client.GetOrderFees(ctx, isin, OrderFeesOptions{OrderVolume: "-100"})
		},
	} {
		if _, err := call(); !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("invalid financial options accepted: %v", err)
		}
	}
	if _, err := client.GetQuote(ctx, "../invalid", CurrencyOptions{}); !errors.Is(err, ErrInvalidISIN) {
		t.Fatal("invalid instrument accepted")
	}
}

func TestFinancialPrecisionAndChartOverrides(t *testing.T) {
	const payload = `{"value":1.234567890123456789,"nullable":null,"series":[],"futureField":{"value":"retained"}}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		query := request.URL.Query()
		switch {
		case strings.HasSuffix(request.URL.Path, "/performance-chart"):
			if query.Get("dateFrom") != "2026-01-01" || query.Get("dateTo") != "2026-10-01" || query.Get("reduceData") != "false" || query.Get("includeDividends") != "false" || query.Get("valuesType") != "MARKET_VALUE" || query.Get("currency") != "USD" {
				t.Error("chart overrides were changed")
			}
			if !strings.Contains("&"+request.URL.RawQuery+"&", "&period&") {
				t.Error("explicit bare period not preserved")
			}
		case strings.HasSuffix(request.URL.Path, "/savings-plans"):
			if query.Get("savingsRate") != "100.000000000000000001" {
				t.Error("decimal amount was rounded")
			}
		case strings.HasSuffix(request.URL.Path, "/exposures/countries"):
			if query.Get("threshold") != "0.000000000000000001" || query.Get("scale") != "0" {
				t.Error("decimal threshold or zero scale was changed")
			}
		}
		writer.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx := context.Background()
	isin := "IE00B5BMR087"
	for _, call := range []func() (json.RawMessage, error){
		func() (json.RawMessage, error) {
			return client.GetPerformanceChart(ctx, isin, ChartOptions{CurrencyOptions: CurrencyOptions{Currency: "USD"}, DateFrom: financialPtr("2026-01-01"), DateTo: financialPtr("2026-10-01"), Period: financialPtr(""), ReduceData: financialPtr(false), IncludeDividends: financialPtr(false), ValuesType: "MARKET_VALUE"})
		},
		func() (json.RawMessage, error) {
			return client.GetSavingsPlans(ctx, isin, SavingsPlansOptions{SavingsRate: "100.000000000000000001"})
		},
		func() (json.RawMessage, error) {
			return client.GetCountryExposure(ctx, isin, ExposureOptions{Threshold: "0.000000000000000001", Scale: financialPtr(0)})
		},
	} {
		result, err := call()
		if err != nil || string(result) != payload {
			t.Fatalf("financial response changed: %v", err)
		}
	}
}

func TestUnavailableFinancialChart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		io.Copy(io.Discard, request.Body)
		writer.WriteHeader(404)
		writer.Write([]byte(`{"message":"private upstream detail"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	_, err = client.GetPerformanceChart(context.Background(), "IE00B5BMR087", ChartOptions{})
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 404 || strings.Contains(err.Error(), "private") {
		t.Fatalf("unavailable chart error not preserved safely: %v", err)
	}
}

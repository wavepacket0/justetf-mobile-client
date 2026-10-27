package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

func TestFinancialDispatchValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("invalid options reached HTTP")
		writer.WriteHeader(500)
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	for _, scenario := range []struct {
		operation, params string
		target            error
	}{
		{"get_quote", `{"isin":"invalid"}`, justetf.ErrInvalidISIN},
		{"get_performance_chart", `{"isin":"IE00B5BMR087","date_from":"bad"}`, justetf.ErrInvalidOptions},
		{"get_performance_chart", `{"isin":"IE00B5BMR087","reduce_data":"false"}`, errInvalidArguments},
		{"get_country_exposure", `{"isin":"IE00B5BMR087","threshold":0.01}`, errInvalidArguments},
		{"get_holding_etfs", `{"isin":"IE00B5BMR087","page":-1}`, justetf.ErrInvalidOptions},
		{"get_savings_plans", `{"isin":"IE00B5BMR087","savings_rate":"NaN"}`, justetf.ErrInvalidOptions},
		{"get_order_fees", `{"isin":"IE00B5BMR087","order_volume":100}`, errInvalidArguments},
		{"get_index_performance", `{"limit":0}`, justetf.ErrInvalidOptions},
		{"get_brokers", `{"currency":"EUR"}`, errInvalidArguments},
		{"get_supplementary_data", `{"isin":"IE00B5BMR087"}`, errInvalidArguments},
	} {
		_, err := dispatch(context.Background(), client, scenario.operation, []byte(scenario.params))
		if !errors.Is(err, scenario.target) {
			t.Fatalf("%s: unexpected error %v", scenario.operation, err)
		}
	}
}

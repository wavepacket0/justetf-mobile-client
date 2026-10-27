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

func TestDiscoveryDispatchParity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		json.NewEncoder(writer).Encode(map[string]any{"method": request.Method, "path": request.URL.Path, "query": request.URL.Query(), "body": json.RawMessage(body)})
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test", Country: "DE", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx := context.Background()
	limit := 7
	for _, scenario := range []struct {
		operation, parameters string
		direct                func() (json.RawMessage, error)
	}{
		{"get_universe_countries", `{}`, func() (json.RawMessage, error) { return client.GetUniverseCountries(ctx) }},
		{"get_account_access", `{"country":"FR"}`, func() (json.RawMessage, error) { return client.GetAccountAccess(ctx, "FR") }},
		{"get_search_metadata", `{"app_version":"5.0"}`, func() (json.RawMessage, error) {
			return client.GetSearchMetadata(ctx, justetf.SearchMetadataOptions{AppVersion: "5.0"})
		}},
		{"search_etfs", `{"query":"ETF & index","filters":["one"],"limit":7,"page":2}`, func() (json.RawMessage, error) {
			return client.SearchETFs(ctx, justetf.SearchOptions{Query: "ETF & index", Filters: []string{"one"}, Limit: &limit, Page: 2})
		}},
		{"search_isins", `{"query":"ETF","sorting":"LARGEST"}`, func() (json.RawMessage, error) {
			return client.SearchISINs(ctx, justetf.SearchISINsOptions{Query: "ETF", Sorting: "LARGEST"})
		}},
		{"get_cards", `{"isins":["IE00B5BMR087"],"returns_period":"M6"}`, func() (json.RawMessage, error) {
			return client.GetCards(ctx, justetf.CardsOptions{ISINs: []string{"IE00B5BMR087"}, ReturnsPeriod: "M6"})
		}},
		{"compare_etfs", `{"isins":[]}`, func() (json.RawMessage, error) { return client.CompareETFs(ctx, justetf.ComparisonOptions{}) }},
		{"search_holdings", `{"query":"holding","limit":7,"offset":2}`, func() (json.RawMessage, error) {
			return client.SearchHoldings(ctx, justetf.HoldingSearchOptions{Query: "holding", Limit: &limit, Offset: 2})
		}},
	} {
		t.Run(scenario.operation, func(t *testing.T) {
			expected, err := scenario.direct()
			if err != nil {
				t.Fatal(err)
			}
			actual, err := dispatch(ctx, client, scenario.operation, []byte(scenario.parameters))
			if err != nil || string(actual) != string(expected) {
				t.Fatalf("dispatch differs from typed Go call: %v", err)
			}
		})
	}
}

func TestDiscoveryDispatchValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("invalid options caused a network request")
		writer.WriteHeader(500)
	}))
	defer server.Close()
	client, err := justetf.NewClient(justetf.Config{BaseURL: server.URL, APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	for _, scenario := range []struct {
		operation, parameters string
		target                error
	}{
		{"get_universe_countries", `{"query":"unsupported"}`, errInvalidArguments},
		{"get_account_access", `{"country":123}`, errInvalidArguments},
		{"get_search_metadata", `{"filters":[]}`, errInvalidArguments},
		{"search_etfs", `{"filters":[123]}`, errInvalidArguments},
		{"search_isins", `{"filters":{}}`, errInvalidArguments},
		{"get_cards", `{"isins":["invalid"]}`, justetf.ErrInvalidISIN},
		{"get_cards", `{"limit":0}`, justetf.ErrInvalidOptions},
		{"compare_etfs", `{"isins":[false]}`, errInvalidArguments},
		{"search_holdings", `{"filters":["unverified"]}`, errInvalidArguments},
		{"search_holdings", `{"currency":"USD"}`, errInvalidArguments},
		{"search_holdings", `{"offset":-1}`, justetf.ErrInvalidOptions},
	} {
		_, err := dispatch(context.Background(), client, scenario.operation, []byte(scenario.parameters))
		if !errors.Is(err, scenario.target) {
			t.Fatalf("%s: unexpected error %v", scenario.operation, err)
		}
	}
}

package justetf

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryOptions(t *testing.T) {
	limit := 7
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		for key, expected := range map[string]string{"query": "ETF & \u6307\u6570", "universeCountry": "DE", "currency": "USD", "returnPeriod": "M6", "returnPeriodEnd": "TODAY", "sortBy": "name", "sortDir": "ASC", "limit": "7", "page": "2"} {
			if query.Get(key) != expected {
				t.Errorf("%s=%q, expected %q", key, query.Get(key), expected)
			}
		}
		if request.Header.Get("accept-language") != "fr" {
			t.Error("language config not applied")
		}
		body, _ := io.ReadAll(request.Body)
		if !jsonEqual(body, []byte(`["one","two"]`)) {
			t.Errorf("body=%s", body)
		}
		writer.Write([]byte(`{"etfs":[],"totalEtfsCount":0,"futureField":null}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test", Country: "DE", Currency: "USD", Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	_, err = client.SearchETFs(context.Background(), SearchOptions{Filters: []string{"one", "two"}, Query: "ETF & \u6307\u6570", ReturnPeriod: "M6", ReturnPeriodEnd: "TODAY", SortBy: "name", SortDir: "ASC", Limit: &limit, Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []int{0, -1} {
		if _, err := client.SearchETFs(context.Background(), SearchOptions{Limit: &invalid}); !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("invalid limit accepted: %v", err)
		}
	}
	if _, err := client.GetCards(context.Background(), CardsOptions{Page: -1}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatal("negative page accepted")
	}
	if _, err := client.SearchHoldings(context.Background(), HoldingSearchOptions{Offset: -1}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatal("negative offset accepted")
	}
	if _, err := client.CompareETFs(context.Background(), ComparisonOptions{ISINs: []string{"../invalid"}}); !errors.Is(err, ErrInvalidISIN) {
		t.Fatal("invalid ISIN accepted")
	}
}

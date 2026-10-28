package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

type endpointCase struct {
	Name      string            `json:"name"`
	Mutation  bool              `json:"mutation"`
	Arguments []string          `json:"arguments"`
	Options   json.RawMessage   `json:"options"`
	Fields    map[string]string `json:"fields"`
}

func endpointCatalogue(t *testing.T) []endpointCase {
	t.Helper()
	data, err := os.ReadFile("contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []endpointCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func endpointClient(t *testing.T) *justetf.Client {
	t.Helper()
	target := os.Getenv("JUSTETF_TEST_BASE_URL")
	if target == "" {
		t.Skip("run python test/run.py to select the shared contract-test backend")
	}
	mode := os.Getenv("JUSTETF_TEST_MODE")
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal("invalid contract-test target")
	}
	if mode == "offline" {
		if parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
			t.Fatal("offline contracts require the loopback mock server")
		}
	} else if (mode != "online-readonly" && mode != "online") || target != "https://api.mobile.justetf.com" {
		t.Fatal("online contracts require the official HTTPS endpoint")
	}
	client, err := justetf.NewClient(justetf.Config{BaseURL: target, Language: "en", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal("cannot configure endpoint client")
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	email := requiredEnvironment(t, "JUSTETF_TEST_EMAIL")
	requireLiveSuccess(t, client.Login(ctx, email, requiredEnvironment(t, "JUSTETF_TEST_PASSWORD")))
	requireLiveSuccess(t, client.Refresh(ctx))
	raw, err := client.GetPersonalData(ctx, "")
	requireLiveSuccess(t, err)
	account, err := checkResponseFields(raw, map[string]string{"email": "string"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(account["email"].(string), email) {
		t.Fatal("account identity differs from the configured test account")
	}
	return client
}

func caseOptions[Options any](t *testing.T, scenario endpointCase) Options {
	t.Helper()
	var options Options
	if len(scenario.Options) != 0 {
		if err := json.Unmarshal(scenario.Options, &options); err != nil {
			t.Fatal("invalid catalogue options")
		}
	}
	return options
}

func callEndpoint(t *testing.T, client *justetf.Client, scenario endpointCase, arguments []string) (json.RawMessage, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch scenario.Name {
	case "get_profile":
		return client.GetProfile(ctx, arguments[0])
	case "get_universe_countries":
		return client.GetUniverseCountries(ctx)
	case "get_account_access":
		return client.GetAccountAccess(ctx, "")
	case "get_search_metadata":
		return client.GetSearchMetadata(ctx, caseOptions[justetf.SearchMetadataOptions](t, scenario))
	case "search_etfs":
		return client.SearchETFs(ctx, caseOptions[justetf.SearchOptions](t, scenario))
	case "search_isins":
		return client.SearchISINs(ctx, caseOptions[justetf.SearchISINsOptions](t, scenario))
	case "get_cards":
		return client.GetCards(ctx, caseOptions[justetf.CardsOptions](t, scenario))
	case "compare_etfs":
		return client.CompareETFs(ctx, caseOptions[justetf.ComparisonOptions](t, scenario))
	case "search_holdings":
		return client.SearchHoldings(ctx, caseOptions[justetf.HoldingSearchOptions](t, scenario))
	case "get_quote":
		return client.GetQuote(ctx, arguments[0], caseOptions[justetf.CurrencyOptions](t, scenario))
	case "get_performance_chart":
		return client.GetPerformanceChart(ctx, arguments[0], caseOptions[justetf.ChartOptions](t, scenario))
	case "get_returns":
		return client.GetReturns(ctx, arguments[0], caseOptions[justetf.PeriodEndOptions](t, scenario))
	case "get_risks":
		return client.GetRisks(ctx, arguments[0], caseOptions[justetf.PeriodEndOptions](t, scenario))
	case "get_country_exposure":
		return client.GetCountryExposure(ctx, arguments[0], caseOptions[justetf.ExposureOptions](t, scenario))
	case "get_sector_exposure":
		return client.GetSectorExposure(ctx, arguments[0], caseOptions[justetf.ExposureOptions](t, scenario))
	case "get_holding_exposure":
		return client.GetHoldingExposure(ctx, arguments[0], caseOptions[justetf.ExposureOptions](t, scenario))
	case "get_holding_details":
		return client.GetHoldingDetails(ctx, arguments[0], caseOptions[justetf.MarketOptions](t, scenario))
	case "get_holding_etfs":
		return client.GetHoldingETFs(ctx, arguments[0], caseOptions[justetf.HoldingETFsOptions](t, scenario))
	case "get_brokers":
		return client.GetBrokers(ctx, caseOptions[justetf.UniverseOptions](t, scenario))
	case "get_market_overview":
		return client.GetMarketOverview(ctx, caseOptions[justetf.MarketOverviewOptions](t, scenario))
	case "get_index_performance":
		return client.GetIndexPerformance(ctx, caseOptions[justetf.IndexPerformanceOptions](t, scenario))
	case "get_recommended_filters":
		return client.GetRecommendedFilters(ctx, caseOptions[justetf.RecommendedFiltersOptions](t, scenario))
	case "get_savings_plans":
		return client.GetSavingsPlans(ctx, arguments[0], caseOptions[justetf.SavingsPlansOptions](t, scenario))
	case "get_order_fees":
		return client.GetOrderFees(ctx, arguments[0], caseOptions[justetf.OrderFeesOptions](t, scenario))
	case "get_personal_data":
		return client.GetPersonalData(ctx, "")
	case "get_watchlists":
		return client.GetWatchlists(ctx, caseOptions[justetf.WatchlistOptions](t, scenario))
	case "get_saved_filters":
		return client.GetSavedFilters(ctx)
	case "get_price_alerts":
		return client.GetPriceAlerts(ctx)
	case "get_etf_price_alerts":
		return client.GetETFPriceAlerts(ctx, arguments[0])
	case "get_price_alert":
		return client.GetPriceAlert(ctx, arguments[0], arguments[1])
	case "get_portfolios":
		return client.GetPortfolios(ctx, caseOptions[justetf.PortfolioListOptions](t, scenario))
	case "get_portfolio":
		return client.GetPortfolio(ctx, arguments[0], caseOptions[justetf.PortfolioOptions](t, scenario))
	case "get_portfolio_composition":
		return client.GetPortfolioComposition(ctx, arguments[0], caseOptions[justetf.PortfolioCompositionOptions](t, scenario))
	case "can_create_portfolio":
		return client.CanCreatePortfolio(ctx)
	case "get_trade_quote":
		return client.GetTradeQuote(ctx, arguments[0], caseOptions[justetf.TradeQuoteOptions](t, scenario))
	case "get_portfolio_transactions":
		return client.GetPortfolioTransactions(ctx, arguments[0], caseOptions[justetf.PageOptions](t, scenario))
	case "get_portfolio_allocation":
		return client.GetPortfolioAllocation(ctx, arguments[0], caseOptions[justetf.AllocationOptions](t, scenario))
	case "get_portfolio_performance_chart":
		return client.GetPortfolioPerformanceChart(ctx, arguments[0], caseOptions[justetf.PortfolioChartOptions](t, scenario))
	case "get_holding_supplementary_data":
		return client.GetHoldingSupplementaryData(ctx, arguments[0])
	default:
		t.Fatal("catalogue endpoint lacks a Go adapter")
		return nil, nil
	}
}

func responseID(value any) string {
	switch identifier := value.(type) {
	case string:
		return identifier
	case json.Number:
		return identifier.String()
	default:
		return ""
	}
}

func TestEndpointContracts(t *testing.T) {
	client := endpointClient(t)
	identifiers := map[string]string{}
	for _, scenario := range endpointCatalogue(t) {
		if scenario.Mutation {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			arguments := append([]string(nil), scenario.Arguments...)
			for position, argument := range arguments {
				if strings.HasPrefix(argument, "$") {
					arguments[position] = identifiers[strings.TrimPrefix(argument, "$")]
					if arguments[position] == "" {
						if os.Getenv("JUSTETF_TEST_MODE") == "offline" {
							t.Fatal("mock seed resource missing")
						}
						t.Skip("readonly account has no resource for this endpoint; coverage unverified")
					}
				}
			}
			raw, err := callEndpoint(t, client, scenario, arguments)
			requireLiveSuccess(t, err)
			response, err := checkResponseFields(raw, scenario.Fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, present := scenario.Fields["isin"]; present && response["isin"] != arguments[0] {
				t.Fatal("response instrument identity differs from request")
			}
			if scenario.Name == "get_profile" && strings.TrimSpace(response["name"].(string)) == "" {
				t.Fatal("profile name must not be empty")
			}
			if scenario.Name == "get_portfolios" {
				for _, entry := range response["portfolios"].([]any) {
					portfolio, ok := entry.(map[string]any)
					if ok {
						identifier := responseID(portfolio["id"])
						if identifier != "" && identifier != "0" {
							identifiers["portfolio_id"] = identifier
							break
						}
					}
				}
			}
			if scenario.Name == "get_etf_price_alerts" {
				alerts := response["alerts"].([]any)
				if len(alerts) > 0 {
					alert, ok := alerts[0].(map[string]any)
					if ok {
						identifiers["alert_id"] = responseID(alert["id"])
					}
				}
			}
		})
		if t.Failed() {
			return
		}
	}
}

func TestMutationWorkflows(t *testing.T) {
	if os.Getenv("JUSTETF_TEST_MODE") == "online-readonly" {
		t.Skip("account mutations require online mode")
	}
	client := endpointClient(t)
	prefix := fmt.Sprintf("sdk-test-go-%d", time.Now().UnixNano())
	const isin = "IE00B5BMR087"
	ctx := context.Background()
	registerCleanup := func(identifier *string, label string, owns func(context.Context, string) bool, remove func(context.Context, string) error) {
		t.Cleanup(func() {
			if *identifier == "" {
				t.Errorf("write outcome unresolved; reconcile test resource %s", label)
				return
			}
			cleanupContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if !owns(cleanupContext, *identifier) {
				t.Errorf("cleanup refused: resource ownership could not be verified for %s, id=%s", label, *identifier)
				return
			}
			if err := remove(cleanupContext, *identifier); err != nil {
				t.Errorf("cleanup failed; leftover test resource %s, id=%s", label, *identifier)
			}
		})
	}
	portfolioID := ""
	registerCleanup(&portfolioID, prefix+"-portfolio", func(ctx context.Context, id string) bool {
		raw, err := client.GetPortfolio(ctx, id, justetf.PortfolioOptions{})
		if err != nil {
			return false
		}
		resource, err := checkResponseFields(raw, map[string]string{"name": "string"})
		return err == nil && resource["name"] == prefix+"-portfolio"
	}, client.DeletePortfolio)
	raw, err := client.CreatePortfolio(ctx, justetf.CreatePortfolioRequest{Name: prefix + "-portfolio", Investment: "ONE_TIME"})
	requireLiveSuccess(t, err)
	created, err := checkResponseFields(raw, map[string]string{"id": "number|string"})
	if err != nil {
		t.Fatal(err)
	}
	portfolioID = responseID(created["id"])
	if created["name"] != prefix+"-portfolio" {
		t.Fatal("created portfolio identity mismatch")
	}
	raw, err = client.GetPortfolio(ctx, portfolioID, justetf.PortfolioOptions{})
	requireLiveSuccess(t, err)
	portfolio, err := checkResponseFields(raw, map[string]string{"name": "string"})
	if err != nil {
		t.Fatal(err)
	}
	if portfolio["name"] != prefix+"-portfolio" {
		t.Fatal("portfolio read-after-create mismatch")
	}
	transaction := justetf.ShareTransactionRequest{TradeDate: "2026-10-14", TransactionType: "DELIVERY", Amount: "123.45", ISIN: isin, Quantity: "1", Quote: "123.45", QuoteCurrency: "EUR", InstrumentCurrency: "EUR", PortfolioCurrency: "EUR", QuoteCurrencyType: "INSTRUMENT", TradeRate: "1", FundRate: "1", Fees: "0", Taxes: "0", Comment: prefix}
	raw, err = client.AddShareTransaction(ctx, portfolioID, transaction)
	requireLiveSuccess(t, err)
	if _, err := checkResponseFields(raw, map[string]string{"transactionId": "number|string", "isin": "string"}); err != nil {
		t.Fatal(err)
	}
	raw, err = client.GetPortfolioTransactions(ctx, portfolioID, justetf.PageOptions{})
	requireLiveSuccess(t, err)
	entries, err := checkResponseFields(raw, map[string]string{"transactions": "array"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries["transactions"].([]any) {
		record, ok := entry.(map[string]any)
		if ok && record["isin"] == isin {
			found = true
		}
	}
	if !found {
		t.Fatal("transaction missing after successful write")
	}
	filterID := ""
	registerCleanup(&filterID, prefix+"-filter", func(ctx context.Context, id string) bool {
		raw, err := client.GetSavedFilters(ctx)
		if err != nil {
			return false
		}
		resource, err := checkResponseFields(raw, map[string]string{"savedFilters": "array"})
		if err != nil {
			return false
		}
		for _, entry := range resource["savedFilters"].([]any) {
			record, ok := entry.(map[string]any)
			if ok && responseID(record["id"]) == id && record["label"] == prefix+"-filter" {
				return true
			}
		}
		return false
	}, client.DeleteSavedFilter)
	requireLiveSuccess(t, client.CreateSavedFilter(ctx, justetf.SavedFilterRequest{Label: prefix + "-filter"}))
	raw, err = client.GetSavedFilters(ctx)
	requireLiveSuccess(t, err)
	filters, err := checkResponseFields(raw, map[string]string{"savedFilters": "array"})
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, entry := range filters["savedFilters"].([]any) {
		record, ok := entry.(map[string]any)
		if ok && record["label"] == prefix+"-filter" {
			matches++
			filterID = responseID(record["id"])
		}
	}
	if matches != 1 {
		filterID = ""
		t.Fatal("created saved filter cannot be identified uniquely")
	}
	alertID := ""
	registerCleanup(&alertID, prefix+"-alert", func(ctx context.Context, id string) bool {
		raw, err := client.GetPriceAlert(ctx, isin, id)
		if err != nil {
			return false
		}
		resource, err := checkResponseFields(raw, map[string]string{"comment": "string"})
		return err == nil && resource["comment"] == prefix+"-alert"
	}, func(ctx context.Context, id string) error { return client.DeletePriceAlert(ctx, isin, id) })
	raw, err = client.CreatePriceAlert(ctx, isin, justetf.PriceAlertRequest{Price: "9999999", Condition: "GTE", Comment: prefix + "-alert"})
	requireLiveSuccess(t, err)
	alert, err := checkResponseFields(raw, map[string]string{"id": "string"})
	if err != nil {
		t.Fatal(err)
	}
	alertID = responseID(alert["id"])
	raw, err = client.GetPriceAlert(ctx, isin, alertID)
	requireLiveSuccess(t, err)
	alert, err = checkResponseFields(raw, map[string]string{"comment": "string"})
	if err != nil {
		t.Fatal(err)
	}
	if alert["comment"] != prefix+"-alert" {
		t.Fatal("alert read-after-create mismatch")
	}
}

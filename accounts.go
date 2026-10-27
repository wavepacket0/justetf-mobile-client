package justetf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type WatchlistOptions struct {
	CurrencyOptions
	ReturnPeriodEnd string `json:"return_period_end,omitempty"`
	ReturnsPeriod   string `json:"returns_period,omitempty"`
}

type PageOptions struct {
	Limit *int `json:"limit,omitempty"`
	Page  int  `json:"page,omitempty"`
}

type PortfolioListOptions struct {
	PageOptions
	Period     string `json:"period,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
}

type PortfolioOptions struct {
	Period     string `json:"period,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
}

type PortfolioCompositionOptions struct {
	Period string `json:"period,omitempty"`
}

type TradeQuoteOptions struct {
	CurrencyOptions
	Date string `json:"date"`
}

type AllocationOptions struct {
	Grouping string `json:"grouping,omitempty"`
	Limit    *int   `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

type PortfolioChartOptions struct {
	CurrencyOptions
	DateFrom   *string `json:"date_from,omitempty"`
	DateTo     *string `json:"date_to,omitempty"`
	Period     *string `json:"period,omitempty"`
	ReduceData *bool   `json:"reduce_data,omitempty"`
	ValuesType string  `json:"values_type,omitempty"`
}

var (
	portfolioIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
	resourceIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
)

func (client *Client) getAccount(ctx context.Context, path string, query url.Values) (json.RawMessage, error) {
	return client.doAuthenticatedJSON(ctx, http.MethodGet, path, query, struct{}{}, requiredJSON, replayReadOnce)
}

func (client *Client) getPortfolioResource(ctx context.Context, id, path string, query url.Values) (json.RawMessage, error) {
	if !portfolioIDPattern.MatchString(id) {
		return nil, fmt.Errorf("%w: portfolio ID must be a positive decimal string", ErrInvalidOptions)
	}
	return client.getAccount(ctx, fmt.Sprintf(path, id), query)
}

func (client *Client) getAccountInstrument(ctx context.Context, isin, path string, query url.Values) (json.RawMessage, error) {
	if !isinPattern.MatchString(isin) {
		return nil, ErrInvalidISIN
	}
	return client.getAccount(ctx, fmt.Sprintf(path, isin), query)
}

func (client *Client) GetPersonalData(ctx context.Context, country string) (json.RawMessage, error) {
	return client.getAccount(ctx, personalDataPath, url.Values{"universeCountry": {defaultString(country, client.config.Country)}})
}

func (client *Client) GetWatchlists(ctx context.Context, options WatchlistOptions) (json.RawMessage, error) {
	query := client.currencyQuery(options.CurrencyOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	query.Set("returnsPeriod", defaultString(options.ReturnsPeriod, "Y1"))
	return client.getAccount(ctx, watchlistsPath, query)
}

func (client *Client) GetSavedFilters(ctx context.Context) (json.RawMessage, error) {
	return client.getAccount(ctx, savedFiltersPath, nil)
}
func (client *Client) GetPriceAlerts(ctx context.Context) (json.RawMessage, error) {
	return client.getAccount(ctx, priceAlertsPath, nil)
}
func (client *Client) GetETFPriceAlerts(ctx context.Context, isin string) (json.RawMessage, error) {
	return client.getAccountInstrument(ctx, isin, etfPriceAlertsPath, nil)
}

func (client *Client) GetPriceAlert(ctx context.Context, isin, id string) (json.RawMessage, error) {
	if !isinPattern.MatchString(isin) {
		return nil, ErrInvalidISIN
	}
	if !resourceIDPattern.MatchString(id) {
		return nil, fmt.Errorf("%w: invalid alert ID", ErrInvalidOptions)
	}
	return client.getAccount(ctx, fmt.Sprintf(priceAlertPath, isin, id), nil)
}

func (client *Client) GetPortfolios(ctx context.Context, options PortfolioListOptions) (json.RawMessage, error) {
	query := url.Values{"period": {defaultString(options.Period, "MAX")}, "appVersion": {defaultString(options.AppVersion, mobileAppVersion)}}
	if err := setPagination(query, options.Limit, options.Page, 10, "page"); err != nil {
		return nil, err
	}
	return client.getAccount(ctx, portfoliosPath, query)
}

func (client *Client) GetPortfolio(ctx context.Context, id string, options PortfolioOptions) (json.RawMessage, error) {
	query := url.Values{"period": {defaultString(options.Period, "MAX")}, "appVersion": {defaultString(options.AppVersion, mobileAppVersion)}}
	return client.getPortfolioResource(ctx, id, portfolioPath, query)
}

func (client *Client) GetPortfolioComposition(ctx context.Context, id string, options PortfolioCompositionOptions) (json.RawMessage, error) {
	return client.getPortfolioResource(ctx, id, portfolioCompositionPath, url.Values{"period": {defaultString(options.Period, "MAX")}})
}

func (client *Client) CanCreatePortfolio(ctx context.Context) (json.RawMessage, error) {
	return client.getAccount(ctx, canCreatePortfolioPath, nil)
}

func (client *Client) GetTradeQuote(ctx context.Context, isin string, options TradeQuoteOptions) (json.RawMessage, error) {
	if !isinPattern.MatchString(isin) {
		return nil, ErrInvalidISIN
	}
	if _, err := time.Parse("2006-01-02", options.Date); err != nil {
		return nil, fmt.Errorf("%w: trade date must use YYYY-MM-DD", ErrInvalidOptions)
	}
	query := client.currencyQuery(options.CurrencyOptions)
	query.Set("isin", isin)
	query.Set("date", options.Date)
	return client.getAccount(ctx, tradeQuotePath, query)
}

func (client *Client) GetPortfolioTransactions(ctx context.Context, id string, options PageOptions) (json.RawMessage, error) {
	query := url.Values{}
	if err := setPagination(query, options.Limit, options.Page, 10, "page"); err != nil {
		return nil, err
	}
	return client.getPortfolioResource(ctx, id, portfolioTransactionsPath, query)
}

func (client *Client) GetPortfolioAllocation(ctx context.Context, id string, options AllocationOptions) (json.RawMessage, error) {
	query := url.Values{"grouping": {defaultString(options.Grouping, "COUNTRIES")}}
	if err := setPagination(query, options.Limit, options.Offset, 5, "offset"); err != nil {
		return nil, err
	}
	return client.getPortfolioResource(ctx, id, portfolioAllocationPath, query)
}

func (client *Client) GetPortfolioPerformanceChart(ctx context.Context, id string, options PortfolioChartOptions) (json.RawMessage, error) {
	query, err := client.chartQuery(ChartOptions{CurrencyOptions: options.CurrencyOptions, DateFrom: options.DateFrom, DateTo: options.DateTo, Period: options.Period, ReduceData: options.ReduceData, ValuesType: options.ValuesType}, "MAX", false)
	if err != nil {
		return nil, err
	}
	return client.getPortfolioResource(ctx, id, portfolioChartPath, query)
}

func (client *Client) GetHoldingSupplementaryData(ctx context.Context, isin string) (json.RawMessage, error) {
	return client.getAccountInstrument(ctx, isin, holdingSupplementaryPath, nil)
}

package justetf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CurrencyOptions struct {
	Currency string `json:"currency,omitempty"`
}

type PeriodEndOptions struct {
	CurrencyOptions
	ReturnPeriodEnd string `json:"return_period_end,omitempty"`
}

type ChartOptions struct {
	CurrencyOptions
	DateFrom         *string `json:"date_from,omitempty"`
	DateTo           *string `json:"date_to,omitempty"`
	Period           *string `json:"period,omitempty"`
	ReduceData       *bool   `json:"reduce_data,omitempty"`
	ValuesType       string  `json:"values_type,omitempty"`
	IncludeDividends *bool   `json:"include_dividends,omitempty"`
}

type ExposureOptions struct {
	Top       *int   `json:"top,omitempty"`
	Scale     *int   `json:"scale,omitempty"`
	Threshold string `json:"threshold,omitempty"`
}

type HoldingETFsOptions struct {
	Country string `json:"country,omitempty"`
	Top     *int   `json:"top,omitempty"`
	Page    int    `json:"page,omitempty"`
	Scale   *int   `json:"scale,omitempty"`
}

type MarketOverviewOptions struct {
	MarketOptions
	ReturnPeriodEnd string `json:"return_period_end,omitempty"`
	ReturnsPeriod   string `json:"returns_period,omitempty"`
	HomeScreenOnly  *bool  `json:"home_screen_only,omitempty"`
}

type IndexPerformanceOptions struct {
	MarketOptions
	PerformanceType     string `json:"performance_type,omitempty"`
	PerformanceValue    string `json:"performance_value,omitempty"`
	PerformanceCategory string `json:"performance_category,omitempty"`
	Limit               *int   `json:"limit,omitempty"`
}

type RecommendedFiltersOptions struct {
	UniverseOptions
	AppVersion string `json:"app_version,omitempty"`
}

type SavingsPlansOptions struct {
	UniverseOptions
	SavingsRate string `json:"savings_rate,omitempty"`
	Sorting     string `json:"sorting,omitempty"`
}

type OrderFeesOptions struct {
	UniverseOptions
	OrderVolume string `json:"order_volume,omitempty"`
	Sorting     string `json:"sorting,omitempty"`
}

var decimalPattern = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)$`)

func decimalOption(value, fallback string, positive bool) (string, error) {
	value = defaultString(value, fallback)
	if !decimalPattern.MatchString(value) || (positive && strings.Trim(value, "0.") == "") {
		return "", fmt.Errorf("%w: expected a nonnegative decimal string (positive for monetary amounts)", ErrInvalidOptions)
	}
	return value, nil
}

func integerOption(value *int, fallback, minimum int) (int, error) {
	actual := fallback
	if value != nil {
		actual = *value
	}
	if actual < minimum {
		return 0, fmt.Errorf("%w: numeric option out of range", ErrInvalidOptions)
	}
	return actual, nil
}

func booleanOption(value *bool, fallback bool) string {
	if value != nil {
		return strconv.FormatBool(*value)
	}
	return strconv.FormatBool(fallback)
}

func (client *Client) currencyQuery(options CurrencyOptions) url.Values {
	return url.Values{"currency": {defaultString(options.Currency, client.config.Currency)}}
}

func (client *Client) getInstrument(ctx context.Context, isin, path string, query url.Values) (json.RawMessage, error) {
	if !isinPattern.MatchString(isin) {
		return nil, ErrInvalidISIN
	}
	return client.doJSON(ctx, http.MethodGet, fmt.Sprintf(path, isin), query, struct{}{}, requiredJSON)
}

func (client *Client) GetQuote(ctx context.Context, isin string, options CurrencyOptions) (json.RawMessage, error) {
	return client.getInstrument(ctx, isin, quotePath, client.currencyQuery(options))
}

func (client *Client) GetPerformanceChart(ctx context.Context, isin string, options ChartOptions) (json.RawMessage, error) {
	query, err := client.chartQuery(options, "Y1", true)
	if err != nil {
		return nil, err
	}
	return client.getInstrument(ctx, isin, chartPath, query)
}

func (client *Client) chartQuery(options ChartOptions, defaultPeriod string, dividends bool) (url.Values, error) {
	query := client.currencyQuery(options.CurrencyOptions)
	for name, value := range map[string]*string{"dateFrom": options.DateFrom, "dateTo": options.DateTo} {
		if value == nil {
			query[name] = nil
			continue
		}
		if *value != "" {
			if _, err := time.Parse("2006-01-02", *value); err != nil {
				return nil, fmt.Errorf("%w: dates must use YYYY-MM-DD", ErrInvalidOptions)
			}
		}
		query.Set(name, *value)
	}
	if options.DateFrom != nil && options.DateTo != nil && *options.DateFrom != "" && *options.DateTo != "" && *options.DateFrom > *options.DateTo {
		return nil, fmt.Errorf("%w: date_from must not exceed date_to", ErrInvalidOptions)
	}
	if options.Period == nil {
		query.Set("period", defaultPeriod)
	} else if *options.Period == "" {
		query["period"] = nil
	} else {
		query.Set("period", *options.Period)
	}
	query.Set("reduceData", booleanOption(options.ReduceData, true))
	if dividends {
		query.Set("includeDividends", booleanOption(options.IncludeDividends, true))
	}
	query.Set("valuesType", defaultString(options.ValuesType, "RELATIVE_CHANGE"))
	return query, nil
}

func (client *Client) periodEndQuery(options PeriodEndOptions) url.Values {
	query := client.currencyQuery(options.CurrencyOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	return query
}

func (client *Client) GetReturns(ctx context.Context, isin string, options PeriodEndOptions) (json.RawMessage, error) {
	return client.getInstrument(ctx, isin, returnsPath, client.periodEndQuery(options))
}

func (client *Client) GetRisks(ctx context.Context, isin string, options PeriodEndOptions) (json.RawMessage, error) {
	return client.getInstrument(ctx, isin, risksPath, client.periodEndQuery(options))
}

func exposureQuery(options ExposureOptions, defaultTop int) (url.Values, error) {
	top, err := integerOption(options.Top, defaultTop, 1)
	if err != nil {
		return nil, err
	}
	scale, err := integerOption(options.Scale, 4, 0)
	if err != nil {
		return nil, err
	}
	threshold, err := decimalOption(options.Threshold, "0.01", false)
	if err != nil {
		return nil, err
	}
	return url.Values{"top": {strconv.Itoa(top)}, "scale": {strconv.Itoa(scale)}, "threshold": {threshold}}, nil
}

func (client *Client) getExposure(ctx context.Context, isin, path string, options ExposureOptions, defaultTop int) (json.RawMessage, error) {
	query, err := exposureQuery(options, defaultTop)
	if err != nil {
		return nil, err
	}
	return client.getInstrument(ctx, isin, path, query)
}

func (client *Client) GetCountryExposure(ctx context.Context, isin string, options ExposureOptions) (json.RawMessage, error) {
	return client.getExposure(ctx, isin, countryExposurePath, options, 4)
}
func (client *Client) GetSectorExposure(ctx context.Context, isin string, options ExposureOptions) (json.RawMessage, error) {
	return client.getExposure(ctx, isin, sectorExposurePath, options, 4)
}
func (client *Client) GetHoldingExposure(ctx context.Context, isin string, options ExposureOptions) (json.RawMessage, error) {
	return client.getExposure(ctx, isin, holdingExposurePath, options, 10)
}

func (client *Client) GetHoldingDetails(ctx context.Context, isin string, options MarketOptions) (json.RawMessage, error) {
	return client.getInstrument(ctx, isin, holdingDetailsPath, client.marketQuery(options))
}

func (client *Client) GetHoldingETFs(ctx context.Context, isin string, options HoldingETFsOptions) (json.RawMessage, error) {
	top, err := integerOption(options.Top, 15, 1)
	if err != nil {
		return nil, err
	}
	scale, err := integerOption(options.Scale, 2, 0)
	if err != nil {
		return nil, err
	}
	if options.Page < 0 {
		return nil, fmt.Errorf("%w: page must be nonnegative", ErrInvalidOptions)
	}
	query := url.Values{"top": {strconv.Itoa(top)}, "page": {strconv.Itoa(options.Page)}, "scale": {strconv.Itoa(scale)}, "universeCountry": {defaultString(options.Country, client.config.Country)}}
	return client.getInstrument(ctx, isin, holdingETFsPath, query)
}

func (client *Client) GetBrokers(ctx context.Context, options UniverseOptions) (json.RawMessage, error) {
	return client.doJSON(ctx, http.MethodGet, brokersPath, client.universeQuery(options), struct{}{}, requiredJSON)
}

func (client *Client) GetMarketOverview(ctx context.Context, options MarketOverviewOptions) (json.RawMessage, error) {
	query := client.marketQuery(options.MarketOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	query.Set("returnsPeriod", defaultString(options.ReturnsPeriod, "D1"))
	query.Set("homeScreenOnly", booleanOption(options.HomeScreenOnly, false))
	return client.doJSON(ctx, http.MethodGet, marketOverviewPath, query, struct{}{}, requiredJSON)
}

func (client *Client) GetIndexPerformance(ctx context.Context, options IndexPerformanceOptions) (json.RawMessage, error) {
	limit, err := integerOption(options.Limit, 50, 1)
	if err != nil {
		return nil, err
	}
	query := client.marketQuery(options.MarketOptions)
	query.Set("performanceType", defaultString(options.PerformanceType, "TOP"))
	query.Set("performanceValue", defaultString(options.PerformanceValue, "RETURN_Y1"))
	query.Set("performanceCategory", defaultString(options.PerformanceCategory, "ALL"))
	query.Set("limit", strconv.Itoa(limit))
	return client.doJSON(ctx, http.MethodGet, indexPerformancePath, query, struct{}{}, requiredJSON)
}

func (client *Client) GetRecommendedFilters(ctx context.Context, options RecommendedFiltersOptions) (json.RawMessage, error) {
	query := client.universeQuery(options.UniverseOptions)
	query.Set("appVersion", defaultString(options.AppVersion, mobileAppVersion))
	return client.doJSON(ctx, http.MethodGet, recommendedFiltersPath, query, struct{}{}, requiredJSON)
}

func (client *Client) GetSavingsPlans(ctx context.Context, isin string, options SavingsPlansOptions) (json.RawMessage, error) {
	rate, err := decimalOption(options.SavingsRate, "100", true)
	if err != nil {
		return nil, err
	}
	query := client.universeQuery(options.UniverseOptions)
	query.Set("savingsRate", rate)
	query.Set("sorting", defaultString(options.Sorting, "RATING"))
	return client.getInstrument(ctx, isin, savingsPlansPath, query)
}

func (client *Client) GetOrderFees(ctx context.Context, isin string, options OrderFeesOptions) (json.RawMessage, error) {
	volume, err := decimalOption(options.OrderVolume, "100", true)
	if err != nil {
		return nil, err
	}
	query := client.universeQuery(options.UniverseOptions)
	query.Set("orderVolume", volume)
	query.Set("sorting", defaultString(options.Sorting, "BEST"))
	return client.getInstrument(ctx, isin, orderFeesPath, query)
}

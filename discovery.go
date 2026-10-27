package justetf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

var ErrInvalidOptions = errors.New("invalid request options")

type UniverseOptions struct {
	Country      string `json:"country,omitempty"`
	UniverseType string `json:"universe_type,omitempty"`
}

type MarketOptions struct {
	UniverseOptions
	Currency string `json:"currency,omitempty"`
}

type SearchMetadataOptions struct {
	MarketOptions
	ReturnPeriodEnd string `json:"return_period_end,omitempty"`
	AppVersion      string `json:"app_version,omitempty"`
}

type SearchOptions struct {
	MarketOptions
	Filters         []string `json:"filters,omitempty"`
	Query           string   `json:"query,omitempty"`
	ReturnPeriodEnd string   `json:"return_period_end,omitempty"`
	ReturnPeriod    string   `json:"return_period,omitempty"`
	SortBy          string   `json:"sort_by,omitempty"`
	SortDir         string   `json:"sort_dir,omitempty"`
	Limit           *int     `json:"limit,omitempty"`
	Page            int      `json:"page,omitempty"`
}

type SearchISINsOptions struct {
	MarketOptions
	Filters         []string `json:"filters,omitempty"`
	Query           string   `json:"query,omitempty"`
	ReturnPeriodEnd string   `json:"return_period_end,omitempty"`
	ReturnPeriod    string   `json:"return_period,omitempty"`
	Sorting         string   `json:"sorting,omitempty"`
}

type CardsOptions struct {
	MarketOptions
	ISINs           []string `json:"isins,omitempty"`
	ReturnPeriodEnd string   `json:"return_period_end,omitempty"`
	ReturnsPeriod   string   `json:"returns_period,omitempty"`
	Sorting         string   `json:"sorting,omitempty"`
	Limit           *int     `json:"limit,omitempty"`
	Page            int      `json:"page,omitempty"`
}

type ComparisonOptions struct {
	MarketOptions
	ISINs []string `json:"isins,omitempty"`
}

type HoldingSearchOptions struct {
	UniverseOptions
	Query     string `json:"query,omitempty"`
	SortField string `json:"sort_field,omitempty"`
	SortOrder string `json:"sort_order,omitempty"`
	Limit     *int   `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (client *Client) universeQuery(options UniverseOptions) url.Values {
	return url.Values{
		"universeCountry": {defaultString(options.Country, client.config.Country)},
		"universeType":    {defaultString(options.UniverseType, "PRIVATE")},
	}
}

func (client *Client) marketQuery(options MarketOptions) url.Values {
	query := client.universeQuery(options.UniverseOptions)
	query.Set("currency", defaultString(options.Currency, client.config.Currency))
	return query
}

func setPagination(query url.Values, limit *int, position, fallback int, positionName string) error {
	actualLimit := fallback
	if limit != nil {
		actualLimit = *limit
	}
	if actualLimit <= 0 || position < 0 {
		return fmt.Errorf("%w: limit must be positive and page/offset nonnegative", ErrInvalidOptions)
	}
	query.Set("limit", strconv.Itoa(actualLimit))
	query.Set(positionName, strconv.Itoa(position))
	return nil
}

func stringArray(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func validateISINs(values []string) error {
	for _, isin := range values {
		if !isinPattern.MatchString(isin) {
			return ErrInvalidISIN
		}
	}
	return nil
}

func (client *Client) GetUniverseCountries(ctx context.Context) (json.RawMessage, error) {
	return client.doJSON(ctx, http.MethodGet, universeCountriesPath, nil, struct{}{}, requiredJSON)
}

func (client *Client) GetAccountAccess(ctx context.Context, country string) (json.RawMessage, error) {
	query := url.Values{"universeCountry": {defaultString(country, client.config.Country)}}
	return client.doJSON(ctx, http.MethodGet, accountAccessPath, query, struct{}{}, requiredJSON)
}

func (client *Client) GetSearchMetadata(ctx context.Context, options SearchMetadataOptions) (json.RawMessage, error) {
	query := client.marketQuery(options.MarketOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	query.Set("appVersion", defaultString(options.AppVersion, mobileAppVersion))
	return client.doJSON(ctx, http.MethodGet, searchMetadataPath, query, struct{}{}, requiredJSON)
}

func (client *Client) SearchETFs(ctx context.Context, options SearchOptions) (json.RawMessage, error) {
	query := client.marketQuery(options.MarketOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	query.Set("returnPeriod", defaultString(options.ReturnPeriod, "Y1"))
	query.Set("sortBy", defaultString(options.SortBy, "fundSize"))
	query.Set("sortDir", defaultString(options.SortDir, "DESC"))
	if options.Query != "" {
		query.Set("query", options.Query)
	}
	if err := setPagination(query, options.Limit, options.Page, 20, "page"); err != nil {
		return nil, err
	}
	return client.doJSON(ctx, http.MethodPost, searchETFsPath, query, stringArray(options.Filters), requiredJSON)
}

func (client *Client) SearchISINs(ctx context.Context, options SearchISINsOptions) (json.RawMessage, error) {
	query := client.marketQuery(options.MarketOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	query.Set("returnPeriod", defaultString(options.ReturnPeriod, "Y1"))
	query.Set("sorting", defaultString(options.Sorting, "LARGEST"))
	if options.Query != "" {
		query.Set("query", options.Query)
	}
	return client.doJSON(ctx, http.MethodPost, searchISINsPath, query, stringArray(options.Filters), requiredJSON)
}

func (client *Client) GetCards(ctx context.Context, options CardsOptions) (json.RawMessage, error) {
	if err := validateISINs(options.ISINs); err != nil {
		return nil, err
	}
	query := client.marketQuery(options.MarketOptions)
	query.Set("returnPeriodEnd", defaultString(options.ReturnPeriodEnd, "YESTERDAY"))
	query.Set("returnsPeriod", defaultString(options.ReturnsPeriod, "Y1"))
	query.Set("sorting", defaultString(options.Sorting, "BEST"))
	if err := setPagination(query, options.Limit, options.Page, 30, "page"); err != nil {
		return nil, err
	}
	return client.doJSON(ctx, http.MethodPost, cardsPath, query, stringArray(options.ISINs), requiredJSON)
}

func (client *Client) CompareETFs(ctx context.Context, options ComparisonOptions) (json.RawMessage, error) {
	if err := validateISINs(options.ISINs); err != nil {
		return nil, err
	}
	return client.doJSON(ctx, http.MethodPost, comparisonPath, client.marketQuery(options.MarketOptions), stringArray(options.ISINs), requiredJSON)
}

func (client *Client) SearchHoldings(ctx context.Context, options HoldingSearchOptions) (json.RawMessage, error) {
	query := client.universeQuery(options.UniverseOptions)
	query.Set("query", options.Query)
	query.Set("sortField", defaultString(options.SortField, "name"))
	query.Set("sortOrder", defaultString(options.SortOrder, "ASC"))
	if err := setPagination(query, options.Limit, options.Offset, 15, "offset"); err != nil {
		return nil, err
	}
	return client.doJSON(ctx, http.MethodPost, holdingSearchPath, query, []string{}, requiredJSON)
}

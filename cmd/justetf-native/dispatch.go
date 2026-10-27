package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const (
	operationGetProfile           = "get_profile"
	operationGetUniverseCountries = "get_universe_countries"
	operationGetAccountAccess     = "get_account_access"
	operationGetSearchMetadata    = "get_search_metadata"
	operationSearchETFs           = "search_etfs"
	operationSearchISINs          = "search_isins"
	operationGetCards             = "get_cards"
	operationCompareETFs          = "compare_etfs"
	operationSearchHoldings       = "search_holdings"
)

var errInvalidArguments = errors.New("invalid operation arguments")

func dispatchTyped[Options any](ctx context.Context, parameters []byte, call func(context.Context, Options) (json.RawMessage, error)) (json.RawMessage, error) {
	var params Options
	trimmed := bytes.TrimSpace(parameters)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("%w: expected a JSON object", errInvalidArguments)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&params); err != nil {
		return nil, fmt.Errorf("%w: malformed or unknown option fields", errInvalidArguments)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: expected one JSON object", errInvalidArguments)
	}
	return call(ctx, params)
}

type profileParameters struct {
	ISIN string `json:"isin"`
	justetf.MarketOptions
}
type accountAccessParameters struct {
	Country string `json:"country,omitempty"`
}

func dispatch(ctx context.Context, client *justetf.Client, operation string, parameters []byte) (json.RawMessage, error) {
	switch operation {
	case operationLogin, operationRefresh, operationLogout, operationIsAuthenticated:
		return dispatchAuthentication(ctx, client, operation, parameters)
	case operationCreatePriceAlert, operationDeletePriceAlert, operationCreateSavedFilter, operationDeleteSavedFilter,
		operationCreatePortfolio, operationDeletePortfolio, operationAddShareTransaction:
		return dispatchMutations(ctx, client, operation, parameters)
	case operationGetPersonalData, operationGetWatchlists, operationGetSavedFilters, operationGetPriceAlerts,
		operationGetETFPriceAlerts, operationGetPriceAlert, operationGetPortfolios, operationGetPortfolio,
		operationGetPortfolioComposition, operationCanCreatePortfolio, operationGetTradeQuote,
		operationGetPortfolioTransactions, operationGetPortfolioAllocation, operationGetPortfolioPerformanceChart,
		operationGetHoldingSupplementaryData:
		return dispatchAccounts(ctx, client, operation, parameters)
	case operationGetProfile:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params profileParameters) (json.RawMessage, error) {
			return client.GetProfileWithOptions(ctx, params.ISIN, params.MarketOptions)
		})
	case operationGetUniverseCountries:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params struct{}) (json.RawMessage, error) {
			return client.GetUniverseCountries(ctx)
		})
	case operationGetAccountAccess:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params accountAccessParameters) (json.RawMessage, error) {
			return client.GetAccountAccess(ctx, params.Country)
		})
	case operationGetSearchMetadata:
		return dispatchTyped(ctx, parameters, client.GetSearchMetadata)
	case operationSearchETFs:
		return dispatchTyped(ctx, parameters, client.SearchETFs)
	case operationSearchISINs:
		return dispatchTyped(ctx, parameters, client.SearchISINs)
	case operationGetCards:
		return dispatchTyped(ctx, parameters, client.GetCards)
	case operationCompareETFs:
		return dispatchTyped(ctx, parameters, client.CompareETFs)
	case operationSearchHoldings:
		return dispatchTyped(ctx, parameters, client.SearchHoldings)
	default:
		return dispatchFinancial(ctx, client, operation, parameters)
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const (
	operationGetQuote              = "get_quote"
	operationGetPerformanceChart   = "get_performance_chart"
	operationGetReturns            = "get_returns"
	operationGetRisks              = "get_risks"
	operationGetCountryExposure    = "get_country_exposure"
	operationGetSectorExposure     = "get_sector_exposure"
	operationGetHoldingExposure    = "get_holding_exposure"
	operationGetHoldingDetails     = "get_holding_details"
	operationGetHoldingETFs        = "get_holding_etfs"
	operationGetBrokers            = "get_brokers"
	operationGetMarketOverview     = "get_market_overview"
	operationGetIndexPerformance   = "get_index_performance"
	operationGetRecommendedFilters = "get_recommended_filters"
	operationGetSavingsPlans       = "get_savings_plans"
	operationGetOrderFees          = "get_order_fees"
)

type quoteParameters struct {
	ISIN string `json:"isin"`
	justetf.CurrencyOptions
}
type chartParameters struct {
	ISIN string `json:"isin"`
	justetf.ChartOptions
}
type periodEndParameters struct {
	ISIN string `json:"isin"`
	justetf.PeriodEndOptions
}
type exposureParameters struct {
	ISIN string `json:"isin"`
	justetf.ExposureOptions
}
type holdingETFsParameters struct {
	ISIN string `json:"isin"`
	justetf.HoldingETFsOptions
}
type savingsParameters struct {
	ISIN string `json:"isin"`
	justetf.SavingsPlansOptions
}
type feesParameters struct {
	ISIN string `json:"isin"`
	justetf.OrderFeesOptions
}

func dispatchFinancial(ctx context.Context, client *justetf.Client, operation string, parameters []byte) (json.RawMessage, error) {
	switch operation {
	case operationGetQuote:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params quoteParameters) (json.RawMessage, error) {
			return client.GetQuote(ctx, params.ISIN, params.CurrencyOptions)
		})
	case operationGetPerformanceChart:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params chartParameters) (json.RawMessage, error) {
			return client.GetPerformanceChart(ctx, params.ISIN, params.ChartOptions)
		})
	case operationGetReturns:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params periodEndParameters) (json.RawMessage, error) {
			return client.GetReturns(ctx, params.ISIN, params.PeriodEndOptions)
		})
	case operationGetRisks:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params periodEndParameters) (json.RawMessage, error) {
			return client.GetRisks(ctx, params.ISIN, params.PeriodEndOptions)
		})
	case operationGetCountryExposure:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params exposureParameters) (json.RawMessage, error) {
			return client.GetCountryExposure(ctx, params.ISIN, params.ExposureOptions)
		})
	case operationGetSectorExposure:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params exposureParameters) (json.RawMessage, error) {
			return client.GetSectorExposure(ctx, params.ISIN, params.ExposureOptions)
		})
	case operationGetHoldingExposure:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params exposureParameters) (json.RawMessage, error) {
			return client.GetHoldingExposure(ctx, params.ISIN, params.ExposureOptions)
		})
	case operationGetHoldingDetails:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params profileParameters) (json.RawMessage, error) {
			return client.GetHoldingDetails(ctx, params.ISIN, params.MarketOptions)
		})
	case operationGetHoldingETFs:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params holdingETFsParameters) (json.RawMessage, error) {
			return client.GetHoldingETFs(ctx, params.ISIN, params.HoldingETFsOptions)
		})
	case operationGetBrokers:
		return dispatchTyped(ctx, parameters, client.GetBrokers)
	case operationGetMarketOverview:
		return dispatchTyped(ctx, parameters, client.GetMarketOverview)
	case operationGetIndexPerformance:
		return dispatchTyped(ctx, parameters, client.GetIndexPerformance)
	case operationGetRecommendedFilters:
		return dispatchTyped(ctx, parameters, client.GetRecommendedFilters)
	case operationGetSavingsPlans:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params savingsParameters) (json.RawMessage, error) {
			return client.GetSavingsPlans(ctx, params.ISIN, params.SavingsPlansOptions)
		})
	case operationGetOrderFees:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params feesParameters) (json.RawMessage, error) {
			return client.GetOrderFees(ctx, params.ISIN, params.OrderFeesOptions)
		})
	default:
		return nil, fmt.Errorf("%w: unsupported operation", errInvalidArguments)
	}
}

package main

import (
	"context"
	"encoding/json"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const (
	operationGetPersonalData              = "get_personal_data"
	operationGetWatchlists                = "get_watchlists"
	operationGetSavedFilters              = "get_saved_filters"
	operationGetPriceAlerts               = "get_price_alerts"
	operationGetETFPriceAlerts            = "get_etf_price_alerts"
	operationGetPriceAlert                = "get_price_alert"
	operationGetPortfolios                = "get_portfolios"
	operationGetPortfolio                 = "get_portfolio"
	operationGetPortfolioComposition      = "get_portfolio_composition"
	operationCanCreatePortfolio           = "can_create_portfolio"
	operationGetTradeQuote                = "get_trade_quote"
	operationGetPortfolioTransactions     = "get_portfolio_transactions"
	operationGetPortfolioAllocation       = "get_portfolio_allocation"
	operationGetPortfolioPerformanceChart = "get_portfolio_performance_chart"
	operationGetHoldingSupplementaryData  = "get_holding_supplementary_data"
)

type accountInstrumentParameters struct {
	ISIN string `json:"isin"`
}
type alertParameters struct {
	ISIN    string `json:"isin"`
	AlertID string `json:"alert_id"`
}
type portfolioParameters struct {
	PortfolioID string `json:"portfolio_id"`
	justetf.PortfolioOptions
}
type compositionParameters struct {
	PortfolioID string `json:"portfolio_id"`
	justetf.PortfolioCompositionOptions
}
type tradeQuoteParameters struct {
	ISIN string `json:"isin"`
	justetf.TradeQuoteOptions
}
type transactionsParameters struct {
	PortfolioID string `json:"portfolio_id"`
	justetf.PageOptions
}
type allocationParameters struct {
	PortfolioID string `json:"portfolio_id"`
	justetf.AllocationOptions
}
type portfolioChartParameters struct {
	PortfolioID string `json:"portfolio_id"`
	justetf.PortfolioChartOptions
}

func dispatchAccounts(ctx context.Context, client *justetf.Client, operation string, parameters []byte) (json.RawMessage, error) {
	switch operation {
	case operationGetPersonalData:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params accountAccessParameters) (json.RawMessage, error) {
			return client.GetPersonalData(ctx, params.Country)
		})
	case operationGetWatchlists:
		return dispatchTyped(ctx, parameters, client.GetWatchlists)
	case operationGetSavedFilters:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params struct{}) (json.RawMessage, error) {
			return client.GetSavedFilters(ctx)
		})
	case operationGetPriceAlerts:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params struct{}) (json.RawMessage, error) { return client.GetPriceAlerts(ctx) })
	case operationGetETFPriceAlerts:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params accountInstrumentParameters) (json.RawMessage, error) {
			return client.GetETFPriceAlerts(ctx, params.ISIN)
		})
	case operationGetPriceAlert:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params alertParameters) (json.RawMessage, error) {
			return client.GetPriceAlert(ctx, params.ISIN, params.AlertID)
		})
	case operationGetPortfolios:
		return dispatchTyped(ctx, parameters, client.GetPortfolios)
	case operationGetPortfolio:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params portfolioParameters) (json.RawMessage, error) {
			return client.GetPortfolio(ctx, params.PortfolioID, params.PortfolioOptions)
		})
	case operationGetPortfolioComposition:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params compositionParameters) (json.RawMessage, error) {
			return client.GetPortfolioComposition(ctx, params.PortfolioID, params.PortfolioCompositionOptions)
		})
	case operationCanCreatePortfolio:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params struct{}) (json.RawMessage, error) {
			return client.CanCreatePortfolio(ctx)
		})
	case operationGetTradeQuote:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params tradeQuoteParameters) (json.RawMessage, error) {
			return client.GetTradeQuote(ctx, params.ISIN, params.TradeQuoteOptions)
		})
	case operationGetPortfolioTransactions:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params transactionsParameters) (json.RawMessage, error) {
			return client.GetPortfolioTransactions(ctx, params.PortfolioID, params.PageOptions)
		})
	case operationGetPortfolioAllocation:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params allocationParameters) (json.RawMessage, error) {
			return client.GetPortfolioAllocation(ctx, params.PortfolioID, params.AllocationOptions)
		})
	case operationGetPortfolioPerformanceChart:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params portfolioChartParameters) (json.RawMessage, error) {
			return client.GetPortfolioPerformanceChart(ctx, params.PortfolioID, params.PortfolioChartOptions)
		})
	case operationGetHoldingSupplementaryData:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params accountInstrumentParameters) (json.RawMessage, error) {
			return client.GetHoldingSupplementaryData(ctx, params.ISIN)
		})
	default:
		return nil, errInvalidArguments
	}
}

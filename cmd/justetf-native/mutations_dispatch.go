package main

import (
	"context"
	"encoding/json"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const (
	operationCreatePriceAlert    = "create_price_alert"
	operationDeletePriceAlert    = "delete_price_alert"
	operationCreateSavedFilter   = "create_saved_filter"
	operationDeleteSavedFilter   = "delete_saved_filter"
	operationCreatePortfolio     = "create_portfolio"
	operationDeletePortfolio     = "delete_portfolio"
	operationAddShareTransaction = "add_share_transaction"
)

type createAlertParameters struct {
	ISIN string `json:"isin"`
	justetf.PriceAlertRequest
}
type savedFilterIDParameters struct {
	FilterID string `json:"filter_id"`
}
type portfolioIDParameters struct {
	PortfolioID string `json:"portfolio_id"`
}
type shareTransactionParameters struct {
	PortfolioID string `json:"portfolio_id"`
	justetf.ShareTransactionRequest
}

func dispatchMutations(ctx context.Context, client *justetf.Client, operation string, parameters []byte) (json.RawMessage, error) {
	switch operation {
	case operationCreatePriceAlert:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params createAlertParameters) (json.RawMessage, error) {
			return client.CreatePriceAlert(ctx, params.ISIN, params.PriceAlertRequest)
		})
	case operationDeletePriceAlert:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params alertParameters) (json.RawMessage, error) {
			return nil, client.DeletePriceAlert(ctx, params.ISIN, params.AlertID)
		})
	case operationCreateSavedFilter:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params justetf.SavedFilterRequest) (json.RawMessage, error) {
			return nil, client.CreateSavedFilter(ctx, params)
		})
	case operationDeleteSavedFilter:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params savedFilterIDParameters) (json.RawMessage, error) {
			return nil, client.DeleteSavedFilter(ctx, params.FilterID)
		})
	case operationCreatePortfolio:
		return dispatchTyped(ctx, parameters, client.CreatePortfolio)
	case operationDeletePortfolio:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params portfolioIDParameters) (json.RawMessage, error) {
			return nil, client.DeletePortfolio(ctx, params.PortfolioID)
		})
	case operationAddShareTransaction:
		return dispatchTyped(ctx, parameters, func(ctx context.Context, params shareTransactionParameters) (json.RawMessage, error) {
			return client.AddShareTransaction(ctx, params.PortfolioID, params.ShareTransactionRequest)
		})
	default:
		return nil, errInvalidArguments
	}
}

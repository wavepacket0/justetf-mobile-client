package justetf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type PriceAlertRequest struct {
	Price     json.Number `json:"price"`
	Condition string      `json:"condition"`
	Comment   string      `json:"comment"`
}

type SavedFilterData struct {
	Filters  []string `json:"filters"`
	Keywords string   `json:"keywords"`
}

type SavedFilterRequest struct {
	Label           string          `json:"label"`
	SavedFilterData SavedFilterData `json:"saved_filter_data"`
}

type CreatePortfolioRequest struct {
	Name       string `json:"name"`
	Currency   string `json:"currency"`
	Investment string `json:"investment"`
}

type ShareTransactionRequest struct {
	TradeDate          string      `json:"trade_date"`
	TransactionType    string      `json:"transaction_type"`
	Amount             json.Number `json:"amount"`
	Comment            string      `json:"comment"`
	ISIN               string      `json:"isin"`
	Quantity           json.Number `json:"quantity"`
	Quote              json.Number `json:"quote"`
	QuoteCurrency      string      `json:"quote_currency"`
	InstrumentCurrency string      `json:"instrument_currency"`
	PortfolioCurrency  string      `json:"portfolio_currency"`
	QuoteCurrencyType  string      `json:"quote_currency_type"`
	TradeRate          json.Number `json:"trade_rate"`
	FundRate           json.Number `json:"fund_rate"`
	Fees               json.Number `json:"fees"`
	Taxes              json.Number `json:"taxes"`
}

var mutationNumberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)

func validateMutationNumbers(values ...json.Number) error {
	for _, value := range values {
		if !mutationNumberPattern.MatchString(value.String()) {
			return fmt.Errorf("%w: mutation numbers must be plain finite JSON decimals", ErrInvalidOptions)
		}
	}
	return nil
}

func requiredMutationText(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: required mutation fields must not be empty", ErrInvalidOptions)
		}
	}
	return nil
}

func (client *Client) mutateAccount(ctx context.Context, method, path string, body any, policy responsePolicy) (json.RawMessage, error) {
	return client.doAuthenticatedJSON(ctx, method, path, nil, body, policy, neverReplay)
}

func (client *Client) CreatePriceAlert(ctx context.Context, isin string, request PriceAlertRequest) (json.RawMessage, error) {
	if !isinPattern.MatchString(isin) {
		return nil, ErrInvalidISIN
	}
	if err := validateMutationNumbers(request.Price); err != nil {
		return nil, err
	}
	if strings.HasPrefix(request.Price.String(), "-") || strings.Trim(request.Price.String(), "0.") == "" {
		return nil, fmt.Errorf("%w: alert price must be positive", ErrInvalidOptions)
	}
	if err := requiredMutationText(request.Condition); err != nil {
		return nil, err
	}
	return client.mutateAccount(ctx, http.MethodPost, fmt.Sprintf(etfPriceAlertsPath, isin), request, requiredJSON)
}

func (client *Client) DeletePriceAlert(ctx context.Context, isin, id string) error {
	if !isinPattern.MatchString(isin) {
		return ErrInvalidISIN
	}
	if !resourceIDPattern.MatchString(id) {
		return fmt.Errorf("%w: invalid alert ID", ErrInvalidOptions)
	}
	_, err := client.mutateAccount(ctx, http.MethodDelete, fmt.Sprintf(priceAlertPath, isin, id), struct{}{}, emptySuccess)
	return err
}

func (client *Client) CreateSavedFilter(ctx context.Context, request SavedFilterRequest) error {
	if err := requiredMutationText(request.Label); err != nil {
		return err
	}
	body := struct {
		Label string          `json:"label"`
		Data  SavedFilterData `json:"savedFilterData"`
	}{request.Label, SavedFilterData{Filters: stringArray(request.SavedFilterData.Filters), Keywords: request.SavedFilterData.Keywords}}
	_, err := client.mutateAccount(ctx, http.MethodPost, savedFiltersPath, body, emptySuccess)
	return err
}

func (client *Client) DeleteSavedFilter(ctx context.Context, id string) error {
	if !resourceIDPattern.MatchString(id) {
		return fmt.Errorf("%w: invalid saved-filter ID", ErrInvalidOptions)
	}
	_, err := client.mutateAccount(ctx, http.MethodDelete, fmt.Sprintf(savedFilterPath, id), struct{}{}, emptySuccess)
	return err
}

func (client *Client) CreatePortfolio(ctx context.Context, request CreatePortfolioRequest) (json.RawMessage, error) {
	if err := requiredMutationText(request.Name, request.Investment); err != nil {
		return nil, err
	}
	request.Currency = defaultString(request.Currency, client.config.Currency)
	return client.mutateAccount(ctx, http.MethodPost, portfoliosPath, request, requiredJSON)
}

func (client *Client) DeletePortfolio(ctx context.Context, id string) error {
	if !portfolioIDPattern.MatchString(id) {
		return fmt.Errorf("%w: portfolio ID must be a positive decimal string", ErrInvalidOptions)
	}
	_, err := client.mutateAccount(ctx, http.MethodDelete, fmt.Sprintf(portfolioPath, id), struct{}{}, emptySuccess)
	return err
}

func (client *Client) AddShareTransaction(ctx context.Context, id string, request ShareTransactionRequest) (json.RawMessage, error) {
	if !portfolioIDPattern.MatchString(id) {
		return nil, fmt.Errorf("%w: portfolio ID must be a positive decimal string", ErrInvalidOptions)
	}
	if !isinPattern.MatchString(request.ISIN) {
		return nil, ErrInvalidISIN
	}
	if _, err := time.Parse("2006-01-02", request.TradeDate); err != nil {
		return nil, fmt.Errorf("%w: trade date must use YYYY-MM-DD", ErrInvalidOptions)
	}
	if err := requiredMutationText(request.TransactionType, request.QuoteCurrency, request.InstrumentCurrency, request.PortfolioCurrency, request.QuoteCurrencyType); err != nil {
		return nil, err
	}
	if err := validateMutationNumbers(request.Amount, request.Quantity, request.Quote, request.TradeRate, request.FundRate, request.Fees, request.Taxes); err != nil {
		return nil, err
	}
	body := map[string]any{
		"tradeDate": request.TradeDate, "transactionType": request.TransactionType, "amount": request.Amount, "comment": request.Comment, "isin": request.ISIN,
		"quantity": request.Quantity, "quote": request.Quote, "quoteCurrency": request.QuoteCurrency, "instrumentCurrency": request.InstrumentCurrency,
		"portfolioCurrency": request.PortfolioCurrency, "quoteCurrencyType": request.QuoteCurrencyType, "tradeRate": request.TradeRate, "fundRate": request.FundRate, "fees": request.Fees, "taxes": request.Taxes,
	}
	return client.mutateAccount(ctx, http.MethodPost, fmt.Sprintf(shareTransactionsPath, id), body, requiredJSON)
}

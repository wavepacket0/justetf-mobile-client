// Usage: go run ./examples/go <profile|search|analyze|account> [args]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const defaultISIN = "IE00B5BMR087"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: <profile [ISIN] | search QUERY | analyze [ISIN] | account>")
	}

	// JUSTETF_BASE_URL lets the examples run against a mock server.
	client, err := justetf.NewClient(justetf.Config{BaseURL: os.Getenv("JUSTETF_BASE_URL")})
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	command, rest := args[0], args[1:]
	switch command {
	case "profile":
		return profile(ctx, client, isinArg(rest))
	case "search":
		return search(ctx, client, strings.Join(rest, " "))
	case "analyze":
		return analyze(ctx, client, isinArg(rest))
	case "account":
		return account(ctx, client)
	}
	return fmt.Errorf("unknown command %q", command)
}

func isinArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return defaultISIN
}

// profile shows a single call and typed error handling.
func profile(ctx context.Context, client *justetf.Client, isin string) error {
	body, err := client.GetProfile(ctx, isin)
	var apiErr *justetf.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("justETF rejected the request (HTTP %d)", apiErr.StatusCode)
	}
	if err != nil {
		return err
	}
	return printJSON("profile", body)
}

// search shows typed options; unset fields fall back to client defaults.
func search(ctx context.Context, client *justetf.Client, query string) error {
	if query == "" {
		return errors.New("usage: search QUERY")
	}
	limit := 5
	body, err := client.SearchETFs(ctx, justetf.SearchOptions{Query: query, Limit: &limit})
	if err != nil {
		return err
	}
	return printJSON("search", body)
}

func analyze(ctx context.Context, client *justetf.Client, isin string) error {
	top := 5
	sections := []struct {
		title string
		fetch func() (json.RawMessage, error)
	}{
		{"quote", func() (json.RawMessage, error) { return client.GetQuote(ctx, isin, justetf.CurrencyOptions{}) }},
		{"returns", func() (json.RawMessage, error) { return client.GetReturns(ctx, isin, justetf.PeriodEndOptions{}) }},
		{"risks", func() (json.RawMessage, error) { return client.GetRisks(ctx, isin, justetf.PeriodEndOptions{}) }},
		{"country exposure", func() (json.RawMessage, error) {
			return client.GetCountryExposure(ctx, isin, justetf.ExposureOptions{Top: &top})
		}},
	}
	for _, section := range sections {
		body, err := section.fetch()
		if err != nil {
			return fmt.Errorf("%s: %w", section.title, err)
		}
		if err := printJSON(section.title, body); err != nil {
			return err
		}
	}
	return nil
}

// account logs in with credentials from the environment and reads account data.
// The client keeps tokens internally and refreshes expired sessions for account reads.
func account(ctx context.Context, client *justetf.Client) error {
	email, password := os.Getenv("JUSTETF_EMAIL"), os.Getenv("JUSTETF_PASSWORD")
	if email == "" || password == "" {
		return errors.New("set JUSTETF_EMAIL and JUSTETF_PASSWORD")
	}
	if err := client.Login(ctx, email, password); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer client.Logout()
	fmt.Println("authenticated:", client.IsAuthenticated())

	watchlists, err := client.GetWatchlists(ctx, justetf.WatchlistOptions{})
	if err != nil {
		return fmt.Errorf("watchlists: %w", err)
	}
	if err := printJSON("watchlists", watchlists); err != nil {
		return err
	}
	portfolios, err := client.GetPortfolios(ctx, justetf.PortfolioListOptions{})
	if err != nil {
		return fmt.Errorf("portfolios: %w", err)
	}
	return printJSON("portfolios", portfolios)
}

func printJSON(title string, body json.RawMessage) error {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	fmt.Printf("== %s ==\n%s\n\n", title, out.String())
	return nil
}

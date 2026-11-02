<div align="center">

<h1>justETF Mobile Client</h1>

<p><strong>An unofficial client for justETF's mobile API</strong></p>

<p>
    <a href="#quick-start"><img src="https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.22 or newer"></a>
    <a href="#quick-start"><img src="https://img.shields.io/badge/Python-3.10%2B-3776AB?logo=python&logoColor=white" alt="Python 3.10 or newer"></a>
    <a href="#c-abi-v1"><img src="https://img.shields.io/badge/Native%20ABI-v1-555" alt="Native ABI version 1"></a>
    <a href="#project-status"><img src="https://img.shields.io/badge/API-unofficial-lightgrey" alt="Unofficial API"></a>
</p>

<p>Go core library with Python bindings via <code>ctypes</code>.</p>

<p>
    <a href="#quick-start">Quick start</a> |
    <a href="#api-surface">API surface</a> |
    <a href="#python-usage">Python</a> |
    <a href="#go-usage">Go</a> |
    <a href="#build">Build</a> |
    <a href="#test">Testing</a> |
    <a href="#c-abi-v1">C ABI</a>
</p>

</div>

## Quick Start

Clone the repository:

```sh
git clone https://github.com/wavepacket0/justetf-mobile-client.git
cd justetf-mobile-client
# Makes a live public profile request for the sample ISIN IE000VAHT5T0
go run ./examples/go profile
```

### Python

Install the published package from PyPI:

```sh
python -m pip install justetf-mobile-client
```

The wheel includes the native library, so no library path is needed:

```python
from justetf_mobile_client import Client

with Client() as client:
    print(client.get_profile("IE000VAHT5T0"))
```

To build from source, follow [Build](#build).

See [examples/](examples/README.md) for additional Go and Python examples and [Test](#test) for testing. 
For authenticated endpoints, you need to provide an account.

## API Surface

| Area | Included |
| --- | --- |
| Public REST | profiles, discovery, and financial data |
| Account API | authenticated endpoints and profile updates |
| Go | Native client with caller contexts and typed request options |
| Python | Standard-library `ctypes`, decoded JSON, and exact raw-response access |
| Native ABI | Versioned C interface with explicit buffer ownership |

## Shared HTTP execution

Endpoint methods define the route and request data, then delegate HTTP work to a shared Go executor. It builds and sends requests, applies common mobile API headers (client identity, language, API key, and request ID), and handles responses and errors.

Authentication is endpoint-specific: public requests remain unauthenticated even after login, while protected requests use the session managed by the Go client. Login and token refresh stay internal, session tokens are not exposed through the Python interface.

## Build

The Python binding loads the Go client as a shared library. Build it for the same OS and architecture as the Python interpreter. Local builds require Go 1.22+, Python 3.10+, and a C compiler supported by cgo (MinGW-w64 GCC on Windows, Xcode Command Line Tools on macOS, or GCC/Clang on Linux).

### Native library

```powershell
# Windows
New-Item -ItemType Directory -Force bin | Out-Null
go build -trimpath -buildmode=c-shared -o bin/justetf-native.dll ./cmd/justetf-native
```

```bash
# Linux
mkdir -p bin
go build -trimpath -buildmode=c-shared -o bin/libjustetf-native.so ./cmd/justetf-native
```

```bash
# macOS
mkdir -p bin
go build -trimpath -buildmode=c-shared -o bin/libjustetf-native.dylib ./cmd/justetf-native
```

### Python wheel

Build and smoke-test a wheel from the repository root. Wheel builds require `cibuildwheel`. Linux builds also require Docker or Podman.

```bash
python -m pip install --upgrade cibuildwheel
cp LICENSE python/LICENSE
cp README.md python/README.md
python -m cibuildwheel --config-file python/pyproject.toml --output-dir wheelhouse python
```

Each wheel targets the current OS and architecture, bundles the matching shared library, and runs an isolated mock-API smoke test.

## Test

Install test-only dependencies, then run the offline suite:

```bash
python -m pip install -r test/requirements.txt
python test/run.py --mode offline
```


Run the optional handle-lookup benchmark with:

```bash
go test ./cmd/justetf-native -run '^$' -bench BenchmarkSessionLookup -benchmem
```


Python tests build the Go shared library in bin, load it with ctypes, and send requests to local mock servers. By default, tests use mock servers and synthetic account credentials, never the real API. The handle benchmark measures registry lookup overhead, not network or whole-client speed. 

Go unit tests stay beside their packages. Root `test/` owns a shared Python mock server and synthetic catalogue for both clients. Choose a test tier with `JUSTETF_TEST_MODE`:

| Tier | Target and behavior |
| --- | --- |
| `offline` (default) | Local mock server and synthetic credentials; no real API calls. Exercises reads, writes, and failure cases. |
| `online-readonly` | Online API, performs login/refresh and reads without creating or deleting account records. |
| `online` | Online API, also creates scoped test records and cleans them up. Use a dedicated test account. |

Copy `.env.example` to `.env` to configure the runner. Leave credentials blank for `offline`, both online tiers require an account email and password:

```dotenv
JUSTETF_TEST_MODE=offline
JUSTETF_TEST_EMAIL=
JUSTETF_TEST_PASSWORD=
```

The `.env` file is Git-ignored and read only by the test runner. `--mode` overrides the value in `.env`. See the [test guide](test/README.md) for run commands and safety details.


## Python usage


Runnable Go and Python code for profile lookup, search, financial data, and account reads is in [examples/](examples/README.md). The tables below list every method.

### Search and discovery

| Go Method | Python Method / Native Operation | Python Options |
| --- | --- | --- |
| `GetUniverseCountries` | `get_universe_countries` | None |
| `GetAccountAccess` | `get_account_access` | Optional country string |
| `GetSearchMetadata` | `get_search_metadata` | `SearchMetadataOptions` |
| `SearchETFs` | `search_etfs` | `SearchOptions` |
| `SearchISINs` | `search_isins` | `SearchISINsOptions` |
| `GetCards` | `get_cards` | `CardsOptions` |
| `CompareETFs` | `compare_etfs` | `ComparisonOptions` |
| `SearchHoldings` | `search_holdings` | `HoldingSearchOptions` |

Every Python method has a `_json` variant returning owned bytes. Option dataclasses are frozen, keyword-only, and optional, omitted values are resolved in Go, not by a separate Python implementation. Go methods take their typed options as values (use the zero value for defaults). Pointer-valued Go `Limit` fields distinguish omission from an invalid explicit zero. Responses remain raw JSON in Go and dynamically decoded values in Python, no unverified response schema is enforced.

Defaults, overridable per request:
- Country/currency come from the client, universe type is `PRIVATE`, period end is `YESTERDAY`, period is `Y1`.
- Search: fund size descending, limit 20, page 0. Cards: `BEST`, limit 30. Search-ISINs: `LARGEST`. Holding search: name ascending, limit 15.
- Pages and offsets must be nonnegative, limits positive. No upper limit or closed enum set is imposed.
- Search filters are opaque identifiers from search metadata. Omitted lists serialize as `[]`, never `null`.
- The API's `returnPeriod` (search) versus `returnsPeriod` (cards) spelling is preserved.

These methods reproduce observed unauthenticated requests.

### Financial REST data

| Go Method | Python Method / Native Operation | Options |
| --- | --- | --- |
| `GetQuote` | `get_quote` | `CurrencyOptions` |
| `GetPerformanceChart` | `get_performance_chart` | `ChartOptions` |
| `GetReturns` | `get_returns` | `PeriodEndOptions` |
| `GetRisks` | `get_risks` | `PeriodEndOptions` |
| `GetCountryExposure` | `get_country_exposure` | `ExposureOptions` |
| `GetSectorExposure` | `get_sector_exposure` | `ExposureOptions` |
| `GetHoldingExposure` | `get_holding_exposure` | `ExposureOptions` |
| `GetHoldingDetails` | `get_holding_details` | `MarketOptions` |
| `GetHoldingETFs` | `get_holding_etfs` | `HoldingETFsOptions` |
| `GetBrokers` | `get_brokers` | `UniverseOptions` |
| `GetMarketOverview` | `get_market_overview` | `MarketOverviewOptions` |
| `GetIndexPerformance` | `get_index_performance` | `IndexPerformanceOptions` |
| `GetRecommendedFilters` | `get_recommended_filters` | `RecommendedFiltersOptions` |
| `GetSavingsPlans` | `get_savings_plans` | `SavingsPlansOptions` |
| `GetOrderFees` | `get_order_fees` | `OrderFeesOptions` |

Instrument methods take an ISIN before options. Brokers, overview, index performance, and recommended filters take options only. Every Python method has a `_json` variant. `get_profile` also accepts `MarketOptions`. Go retains `GetProfile` and adds `GetProfileWithOptions`, preserving the legacy C entry.

Charts:
- Dates use `YYYY-MM-DD`: reversed or malformed ranges are rejected.
- Omitted dates send bare `dateFrom`/`dateTo` keys: explicit empty strings send `dateFrom=`/`dateTo=`.
- Period defaults to `Y1`: `period=""` sends a bare period for custom ranges. There is no implicit switch between period and custom-range mode.
- Reduction and dividends default to true, values type to `RELATIVE_CHANGE`. Explicit false values are preserved.
- Unavailable charts raise `APIError` with status 404, never an empty success.

Other defaults (all overridable through options):
- Exposure: top 4 for countries/sectors, top 10 for holdings, scale 4, threshold `"0.01"`.
- Holding ETFs: top 15, scale 2, page 0.
- Overview: `D1`, yesterday, home-screen-only false.
- Index performance: TOP/RETURN_Y1/ALL, limit 50.
- Savings plans: amount `"100"`, RATING. Order fees: amount `"100"`, BEST.

Thresholds and monetary request amounts are decimal strings, never floats. Plain nonnegative decimals are accepted, monetary amounts must be positive. Exponent notation, negative amounts, NaN, and infinity are rejected. Counts must be positive, scales/pages nonnegative, explicit zero scale is preserved. Raw responses retain nulls, unknown fields, and exact numeric tokens. Holding supplementary data uses the authenticated account-read API below.

Defaults are Italian, IT, and EUR. Timeout is a positive number of seconds, at most 300, rounded up to
milliseconds. It covers the entire Go HTTP operation including reading the body.

Decoded results use Python's standard JSON numeric behavior. For exact decimal processing, use raw JSON with `json.loads(raw_json, parse_float=Decimal)` from `decimal`. The Go/native/raw-bytes path preserves the original numeric tokens.

Errors are `ValueError` for invalid arguments/configuration, `APIError` with `status_code` for HTTP failures, `TransportError` for I/O/timeouts/invalid upstream JSON, and `ClientClosedError` for closed clients. Other native failures raise `ClientError`. Missing/changed sessions raise `AuthenticationError`, failed upstream login/refresh HTTP responses still raise `APIError` with their status.

## Authentication

```python
from getpass import getpass
from justetf_mobile_client import Client

# For a local build. With an installed wheel, use Client() and omit the path.
with Client("bin/justetf-native.dll") as client:
    client.login(input("Email: "), getpass("Password: "))
    print(client.is_authenticated())
    client.refresh()
    client.logout()
```

Go provides `Login(ctx, email, password) error`, `Refresh(ctx) error`, `Logout()`, `IsAuthenticated() bool`, and `Close()`. Python login/refresh/logout return `None`. The status method reports local token presence, not remote validity or expiry.

Login reproduces `GET /v1/auth/tokens` with Basic authorization. Refresh uses `GET /v1/auth/refreshed-tokens` with the refresh token as Bearer authorization. Both send `{}` and the standard mobile headers. Tokens stay in the Go client and are not persisted or returned through the ABI. Passwords are not stored on the client. Login starts a new session and clears the old one before its request, a failed login leaves the client unauthenticated. Logout/close clear stored token references and invalidate pending authentication results. Managed-runtime memory is not guaranteed to be securely overwritten.

When an authenticated read gets a `401`, the client uses its refresh token to get a new access token and retries the read once. If several reads need this at once, only one refresh request is sent. Cancelling one Go request does not cancel that refresh for other reads: logout, close, or the client timeout does.

After an automatic refresh fails, the client does not keep retrying it on every request. Call `refresh()` to try again. If justETF rejects the refresh token with `401` or `403`, log in again.

## Authenticated Account Reads

```python
from getpass import getpass
from justetf_mobile_client import Client, PortfolioListOptions, TradeQuoteOptions

# For a local build. With an installed wheel, use Client() and omit the path.
with Client("bin/justetf-native.dll") as client:
    client.login(input("Email: "), getpass("Password: "))
    watchlists = client.get_watchlists()
    portfolios = client.get_portfolios(PortfolioListOptions(limit=10, page=0))
    quote = client.get_trade_quote("IE00B5BMR087", TradeQuoteOptions(date="2026-10-14"))
```

All listed methods require `login()` and read from that account; they do not change account data. Eligible reads can refresh the session and retry once after a `401`. IDs are passed as strings. Python returns decoded JSON by default, use a method's `_json` variant when you need the raw response bytes.

| Go Method | Python Method / Native Operation | Arguments After Context / Client |
| --- | --- | --- |
| `GetPersonalData` | `get_personal_data` | Optional country string (Go: empty string for default) |
| `GetWatchlists` | `get_watchlists` | `WatchlistOptions` |
| `GetSavedFilters` | `get_saved_filters` | None |
| `GetPriceAlerts` | `get_price_alerts` | None |
| `GetETFPriceAlerts` | `get_etf_price_alerts` | ISIN |
| `GetPriceAlert` | `get_price_alert` | ISIN, alert ID |
| `GetPortfolios` | `get_portfolios` | `PortfolioListOptions` |
| `GetPortfolio` | `get_portfolio` | Portfolio ID, `PortfolioOptions` |
| `GetPortfolioComposition` | `get_portfolio_composition` | Portfolio ID, `PortfolioCompositionOptions` |
| `CanCreatePortfolio` | `can_create_portfolio` | None; reads eligibility, does not create |
| `GetTradeQuote` | `get_trade_quote` | ISIN, `TradeQuoteOptions` with required date |
| `GetPortfolioTransactions` | `get_portfolio_transactions` | Portfolio ID, `PageOptions` |
| `GetPortfolioAllocation` | `get_portfolio_allocation` | Portfolio ID, `AllocationOptions` |
| `GetPortfolioPerformanceChart` | `get_portfolio_performance_chart` | Portfolio ID, `PortfolioChartOptions` |
| `GetHoldingSupplementaryData` | `get_holding_supplementary_data` | ISIN |



## Account Updates

These operations change justETF account data, a portfolio transaction is only a tracker entry, not a broker order. Live updates require explicit opt-in and should use a dedicated test account. Routine tests use local mocks.

| Go Method | Python Method / Native Operation | Parameters | Result |
| --- | --- | --- | --- |
| `CreatePriceAlert` | `create_price_alert` | ISIN, `PriceAlertRequest` | JSON |
| `DeletePriceAlert` | `delete_price_alert` | ISIN, alert ID | Void |
| `CreateSavedFilter` | `create_saved_filter` | `SavedFilterRequest` | Void |
| `DeleteSavedFilter` | `delete_saved_filter` | Saved-filter ID | Void |
| `CreatePortfolio` | `create_portfolio` | `CreatePortfolioRequest` | JSON |
| `DeletePortfolio` | `delete_portfolio` | Portfolio ID | Void |
| `AddShareTransaction` | `add_share_transaction` | Portfolio ID, `ShareTransactionRequest` | JSON |

Pass monetary values as decimal strings in Python and `json.Number` in Go to preserve precision, the client does not infer missing transaction values. Updates are never retried automatically. If a timeout or transport error leaves the result uncertain, check the account before trying again. Only the operations listed above are supported, other account changes are not implemented.

## Concurrency and lifetime

Clients support concurrent calls and reuse HTTP connections. Use `with` or call `close()` when finished. Closing cancels active requests and clears the session, but does not wait for every call to return.

## C ABI v1

The build produces a C header beside the shared library. That header defines ABI version 1, the C types, and the calling conventions. The main entry points create and close client handles, dispatch supported operations, report the ABI version, and free returned buffers. The legacy `justetf_profile` entry point remains available.

Clients are referenced by opaque handles. Calls pass UTF-8 input bytes and return a `JustETFBuffer` containing output bytes. Input memory remains caller-owned and must stay valid for the call. Free each returned buffer exactly once with `justetf_buffer_free`. Output is not guaranteed to be NUL-terminated, and Go pointers do not cross the ABI.

`justetf_call` accepts only registered operations and JSON parameters. It is not an arbitrary HTTP or URL interface. Check the returned status for success, invalid input, API errors, transport errors, closed handles, internal errors, or authentication and session errors. Use a native library matching the wrapper version and rebuild it when required exports are missing.

## Go usage

The root Go package is `justetf`. Import it from `github.com/wavepacket0/justetf-mobile-client`.

```go
client, err := justetf.NewClient(justetf.Config{
        Timeout: 15 * time.Second,
})
if err != nil {
        return err
}
defer client.Close()
profile, err := client.GetProfile(ctx, "IE000VAHT5T0")
```

Direct Go consumers can supply per-request contexts for cancellation. The native binding exposes a per-client timeout and client-wide cancellation through `Close()`, but not per-request cancellation handles. `CloseIdleConnections()` only manages the connection pool. Use `Close()` to cancel work, clear authentication, and end the client's lifecycle.

Protocol defaults and implemented endpoint paths live in `constants.go`. Add endpoint constants as features are implemented rather than copying the full research catalog into runtime code. Country, currency, and language remain per-client configuration. Tests retain independent expected protocol literals.

## Disclaimer

This is an independent, unofficial project and is not affiliated with, endorsed by, sponsored by, or supported by justETF. It uses justETF's mobile API, which may change or become unavailable without notice. The justETF name, logo, and other branding remain the property of justETF or their respective rights holders.
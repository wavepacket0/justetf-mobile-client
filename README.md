<div align="center">

<h1>justETF Mobile Client</h1>

<p><strong>An unofficial client for justETF's mobile API</strong></p>

<p>
    <a href="#quick-start"><img src="https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.22 or newer"></a>
    <a href="#quick-start"><img src="https://img.shields.io/badge/Python-3.10%2B-3776AB?logo=python&logoColor=white" alt="Python 3.10 or newer"></a>
    <a href="#c-abi-v1"><img src="https://img.shields.io/badge/Native%20ABI-v1-555" alt="Native ABI version 1"></a>
    <a href="#project-status"><img src="https://img.shields.io/badge/API-unofficial-lightgrey" alt="Unofficial API"></a>
</p>

<p>Go owns HTTP and authentication,<br>
Python calls the same implementation in-process through <code>ctypes</code> and a versioned C ABI.</p>

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

## API Surface

| Area | Included |
| --- | --- |
| Public REST | 24 operations for profiles, discovery, and financial data |
| Account API | 15 authenticated reads and seven captured mutations |
| Go | Native client with caller contexts and typed request options |
| Python | Standard-library `ctypes`, decoded JSON, and exact raw-response access |
| Native ABI | Versioned C interface with explicit buffer ownership |

Ads, banners, telemetry, tracking, push reporting, ratings, streaming, and
editorial content are outside the project scope. Responses remain JSON because
the upstream response schema is undocumented. General retry policies are not
implemented. Platform-specific Python wheels bundle the native library; release
publishing is not configured yet.

## Quick Start

From a repository checkout, use a native library built for your OS and CPU
architecture (see [Build](#build)), then install the Python package:

```bash
python -m pip install -e ./python
```

Then use the client, pointing it at the library you built:

```python
from justetf_mobile_client import Client

with Client("bin/libjustetf-native.so") as client:
    profile = client.get_profile("IE00B5BMR087")
    print(profile)
```

The library filename is platform-specific. CI is configured to build and test
wheels that include it; they have not been published yet. See [examples/](examples/README.md)
for runnable Go and Python code and [Test](#test) for offline tests and live-test safeguards.

## Project Status

The online read-only contracts passed against the live API on Windows. Six
account-specific checks were skipped because the test account has no portfolio
or price alert, so those cases remain unverified. GitHub Actions is configured
for Ubuntu, Windows, and macOS, with Python 3.10, 3.12, and 3.15 tests. The wheel
job builds platform-specific packages and tests the bundled library against the
mock API. Windows wheel build/install is verified locally; the hosted CI matrix
has not run yet. See the [test guide](test/README.md) for tier limits.

## Shared HTTP execution

Endpoint methods supply method, path, query, and JSON body to one Go request
executor. It owns common headers, request IDs, serialization, execution, bounded
response reads, status handling, and JSON validation. Its independently built
transport does not assume any concrete type for Go's global default transport.

REST defaults follow the mobile requests: `User-Agent: Dart/3.12
(dart:io)`, `Content-Type: application/json`, configured `Accept-Language`,
`x-api-key`, and a fresh UUIDv4 `x-request-id`. No `Accept` header is added.
The transport negotiates gzip and decompresses it automatically; the 4 MiB
response limit applies to decompressed bytes. Host and content length are
computed by Go. The profile GET sends the captured `{}` body (two bytes).
Country, currency, app-version parameters, and endpoint authorization policies
belong to the endpoints that require them, not unconditional global headers.
Public endpoints remain unauthenticated even after login. Protected execution
supports bounded authentication replay for explicitly designated reads only;
no general network retries or TLS fingerprint emulation has been added.

Every endpoint selects a response policy explicitly:
- Required JSON rejects empty or malformed bodies; profile lookup uses this.
- Optional JSON returns nil for a zero-byte success, otherwise requires JSON.
- Empty success requires a zero-byte body, including on `200` and `204`;
    unexpected response content is rejected instead of silently discarded.

Whitespace-only bodies are not considered empty. All policies retain timeout,
cancellation, HTTP-error handling, and decompressed response-size limits.
Go empty results are nil, native empty results have NULL data and zero length,
and decoded Python empty results are `None`. Raw Python results are `b""`.
Saved-filter creation and the three captured deletes use empty-success policies.
Login and refresh consume upstream token JSON in Go and return no tokens/results
to Python. No optional-JSON account endpoint is exposed yet.

## Build

From this directory, using Go 1.22+, Python 3.10+, and a cgo-compatible C
compiler on PATH. On Windows, MinGW-w64 GCC is verified; see [Quick Start](#quick-start)
for the clang qualification. Python and the shared library must have matching
OS/CPU architectures. cgo must be enabled (`go env CGO_ENABLED` should return `1`).

```powershell
# Windows (verified with MinGW-w64 GCC on PATH)
go build -trimpath -buildmode=c-shared -o bin/justetf-native.dll ./cmd/justetf-native
```

```bash
# Linux
go build -trimpath -buildmode=c-shared -o bin/libjustetf-native.so ./cmd/justetf-native

# macOS
go build -trimpath -buildmode=c-shared -o bin/libjustetf-native.dylib ./cmd/justetf-native
```

### Platform wheel

Build one wheel for the current 64-bit platform from the repository root:

```bash
python -m pip install --upgrade cibuildwheel
cp LICENSE python/LICENSE   # CI does this so the wheel bundles the license
python -m cibuildwheel --config-file python/pyproject.toml --output-dir wheelhouse python
```

Windows requires MinGW-w64 GCC on `PATH`; macOS uses the Xcode command-line
tools. Linux builds use Docker or Podman with a manylinux 2.28 image. The
manylinux setup downloads Go 1.27.2 and verifies its SHA-256 before compiling.
Each wheel is tagged for its OS/architecture, includes the matching shared
library beside the Python module, and runs an isolated mock-API smoke test.

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

The Python tests build a separate test library under the ignored `bin/`
directory; their build subprocess is tooling only, not runtime API transport.
By default, tests use mock servers and synthetic account credentials, never the real API. The handle
benchmark measures registry lookup overhead, not network or whole-client speed.

Go unit tests stay beside their packages. Root `test/` owns a shared Python mock
server and synthetic catalogue for both clients. `python test/run.py` reads
`JUSTETF_TEST_MODE` from `.env`: `offline` (default), `online-readonly`, or
`online-mutations`. Only the last tier permits scoped account writes with cleanup.
To configure it, copy `.env.example` to `.env` (Git-ignored). Only the test runner
reads `.env`; the client library and the examples never do.
See the [test guide](test/README.md) for commands, coverage, and cleanup limits.
Private captures, `testdata/`, and their generator remain Git-ignored and are not
required. An offline pass or skipped online check does not establish live compatibility.

The native integration tests and installed Windows x64 wheel have been executed
locally; hosted CI has not run yet. Set `PYTHONPATH` using the shell's syntax if
you do not install the Python package in editable mode. Installing with
`python -m pip install -e ./python` can replace setting `PYTHONPATH`; it does
not build or bundle the native library. Use the platform-wheel build for an
installed package that includes the Go library.

## Python usage

The shared mobile-app API key from APK version 4.4.2 is a protocol default in
`constants.go`, verified against the captured requests and archived app library.
It is not an account credential and does not replace login. No account passwords
or captured session tokens are included. Keep a client alive across requests to
reuse its HTTP connection pool.

Runnable Go and Python code for profile lookup, search, financial data, and
account reads is in [examples/](examples/README.md). The tables below list every
method.

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

Every Python method has a `_json` variant returning owned bytes. Option
dataclasses are frozen, keyword-only, and optional; omitted values are resolved
in Go, not by a separate Python implementation. Go methods take their typed
options as values (use the zero value for defaults). Pointer-valued Go `Limit`
fields distinguish omission from an invalid explicit zero. Responses remain
raw JSON in Go and dynamically decoded values in Python; no unverified response
schema is enforced.

Defaults, overridable per request:
- Country/currency come from the client; universe type is `PRIVATE`; period end is `YESTERDAY`; period is `Y1`.
- Search: fund size descending, limit 20, page 0. Cards: `BEST`, limit 30. Search-ISINs: `LARGEST`. Holding search: name ascending, limit 15.
- Pages and offsets must be nonnegative, limits positive. No upper limit or closed enum set is imposed.
- Search filters are opaque identifiers from search metadata. Omitted lists serialize as `[]`, never `null`.
- The API's `returnPeriod` (search) versus `returnsPeriod` (cards) spelling is preserved.

These methods reproduce observed unauthenticated requests; tests are offline
contracts, not live certification.

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

Instrument methods take an ISIN before options; brokers, overview, index
performance, and recommended filters take options only. Every Python method has
a `_json` variant. `get_profile` also accepts `MarketOptions`; Go retains
`GetProfile` and adds `GetProfileWithOptions`, preserving the legacy C entry.

Charts:
- Dates use `YYYY-MM-DD`; reversed or malformed ranges are rejected.
- Omitted dates send bare `dateFrom`/`dateTo` keys; explicit empty strings send `dateFrom=`/`dateTo=`.
- Period defaults to `Y1`; `period=""` sends a bare period for custom ranges. There is no implicit switch between period and custom-range mode.
- Reduction and dividends default to true, values type to `RELATIVE_CHANGE`. Explicit false values are preserved.
- Unavailable charts raise `APIError` with status 404, never an empty success.

Other defaults (all overridable through options):
- Exposure: top 4 for countries/sectors, top 10 for holdings, scale 4, threshold `"0.01"`.
- Holding ETFs: top 15, scale 2, page 0.
- Overview: `D1`, yesterday, home-screen-only false.
- Index performance: TOP/RETURN_Y1/ALL, limit 50.
- Savings plans: amount `"100"`, RATING. Order fees: amount `"100"`, BEST.

Thresholds and monetary request amounts are decimal strings, never floats.
Plain nonnegative decimals are accepted; monetary amounts must be positive.
Exponent notation, negative amounts, NaN, and infinity are rejected. Counts must
be positive, scales/pages nonnegative; explicit zero scale is preserved. Raw
responses retain nulls, unknown fields, and exact numeric tokens. Holding
supplementary data uses the authenticated account-read API below.

`get_profile` returns decoded Python JSON values; `get_profile_json` returns
owned Python `bytes` without decoding. The constructor also accepts `api_key`,
`base_url`, `language`, `country`, and `currency`. Defaults are Italian, IT,
and EUR. Timeout is a positive number of seconds, at most 300, rounded up to
milliseconds. It covers the entire Go HTTP operation including reading the body.

Decoded results use Python's standard JSON numeric behavior. For exact decimal
processing, use raw JSON with `json.loads(raw_json, parse_float=Decimal)` from
`decimal`; the Go/native/raw-bytes path preserves the original numeric tokens.

Python uses Go's shared app key when no override is supplied. An explicit `api_key`
takes precedence over the optional `JUSTETF_API_KEY` environment override; an empty
key uses Go's default. Go callers can override `Config.APIKey` directly. The shared
key may rotate with future app versions. `JUSTETF_BASE_URL` overrides the base URL.
Pass a library path
explicitly or set `JUSTETF_LIBRARY`; otherwise the wrapper looks beside itself
for the platform library. Only load a trusted library. Use HTTPS for real API
requests; HTTP is supported for local tests. Configuration passes directly
through memory, without changing environment variables or using command lines.

Errors are `ValueError` for invalid arguments/configuration, `APIError` with
`status_code` for HTTP failures, `TransportError` for I/O/timeouts/invalid upstream
JSON, and `ClientClosedError` for closed clients. Other native failures raise
`ClientError`. Missing/changed sessions raise `AuthenticationError`; failed
upstream login/refresh HTTP responses still raise `APIError` with their status.
HTTP error bodies are not exposed; redirects are not followed. Authenticated
transport errors are redacted while retaining cancellation/deadline signals.

## Authentication

```python
from getpass import getpass
from justetf_mobile_client import Client

with Client("bin/justetf-native.dll") as client:
    client.login(input("Email: "), getpass("Password: "))
    print(client.is_authenticated())
    client.refresh()
    client.logout()
```

Go provides `Login(ctx, email, password) error`, `Refresh(ctx) error`,
`Logout()`, `IsAuthenticated() bool`, and idempotent `Close()`.
Python login/refresh/logout return `None`; no raw token API is provided.
The status method reports local token presence, not remote validity or expiry.
Logout is local: no server logout/revocation endpoint has been captured.

Login reproduces `GET /v1/auth/tokens` with Basic authorization. Refresh uses
`GET /v1/auth/refreshed-tokens` with the refresh token as Bearer authorization.
Both send `{}` and the standard mobile headers. Authentication requires HTTPS;
plain HTTP is accepted only for localhost/loopback tests. Redirects are refused.
Credentials are passed in memory, never command-line arguments or URLs.

Tokens stay in the Go client and are not persisted or returned through the ABI.
Passwords are not stored on the client. Login starts a new session and clears
the old one before its request; a failed login leaves the client unauthenticated.
Logout/close clear stored token references and invalidate pending authentication
results. Managed-runtime memory is not guaranteed to be securely overwritten.

Concurrent expiry for the same token revision shares one refresh. Late old-token
rejections use the already rotated session rather than refreshing again.
Cancelling one Go waiter does not cancel a refresh needed by other callers;
shared work is bounded by the client timeout and cancelled by logout/close.
No authentication lock is held across network I/O.

Account reads use protected execution, not a public arbitrary-request API.
Only explicitly designated GET/HEAD reads may replay once
after a 401; a second 401 returns immediately. Mutations are never automatically
replayed, including after a dropped response. Non-401 failures do not trigger
automatic authentication refresh. The entire protected read/refresh/replay has
one client-timeout budget. Work cannot replay across an account/session change.

Refresh failures are shared/cached for that token revision to prevent storms.
An explicit `refresh()` can retry after a transient failure. Refresh 401/403
clears the tokens and requires login again. All account data methods are tested
with local mocks and synthetic contracts. Live read-only checks passed on
Windows x64; six portfolio/alert-dependent checks remain unverified because the
test account has no matching resources. See [Project Status](#project-status).

## Authenticated Account Reads

```python
from getpass import getpass
from justetf_mobile_client import Client, PortfolioListOptions, TradeQuoteOptions

with Client("bin/justetf-native.dll") as client:
    client.login(input("Email: "), getpass("Password: "))
    watchlists = client.get_watchlists()
    portfolios = client.get_portfolios(PortfolioListOptions(limit=10, page=0))
    quote = client.get_trade_quote("IE00B5BMR087", TradeQuoteOptions(date="2026-10-14"))
```

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

All methods are GET reads, require a local session, and select the protected
one-replay policy. Valid anonymous calls fail before HTTP with
`ErrAuthenticationRequired` in Go or `AuthenticationError` in Python. Public
methods still omit authorization. Every Python method has a `_json` raw-bytes
variant; decoded results retain the upstream object shape, including
`can_create_portfolio` returning an eligibility object rather than a bare bool.

Use string IDs in both languages. Portfolio IDs must be positive decimal strings
(no leading zeros); alert IDs accept alphanumeric, underscore, and hyphen path
segments. Invalid/missing IDs and path separators are rejected locally without
echoing the supplied value. IDs are not converted through floats or bounded to
a machine integer; the native parameter keys are `portfolio_id` and `alert_id`.

Portfolio lists/transactions default to limit 10/page 0; allocation defaults to
COUNTRIES, limit 5/offset 0. Limits must be positive and pages/offsets nonnegative.
Portfolio detail/list/composition periods default to MAX, app version to 4.4.2
where captured. Watchlists default to configured currency, yesterday, and Y1,
using the captured plural `returnsPeriod` query name. Trade dates must be supplied
as valid YYYY-MM-DD strings; no current-date assumption is made.

Portfolio charts default to MAX, reduction true, relative change, and bare date
keys. They share financial date validation and explicit false/empty-period
semantics, but never send the ETF-only `includeDividends` parameter. Raw account
responses preserve exact numeric tokens, dates, nulls, empty collections, and
unknown fields. Use raw bytes plus Decimal decoding when exact decimals matter.
Account data can contain personal information; the library does not log or
persist it. Callers are responsible for their own storage and logging practices.

## Account Mutations

These methods modify justETF account data. A recorded portfolio transaction is
an entry in justETF's tracker, not an order placed with a broker. All routine
tests use local mocks. Only call these methods on an account you intend to
change; live mutation testing requires explicit opt-in and a designated test
account. No server-side idempotency or exactly-once guarantee is assumed.

| Go Method | Python Method / Native Operation | Parameters | Result |
| --- | --- | --- | --- |
| `CreatePriceAlert` | `create_price_alert` | ISIN, `PriceAlertRequest` | JSON |
| `DeletePriceAlert` | `delete_price_alert` | ISIN, alert ID | Void |
| `CreateSavedFilter` | `create_saved_filter` | `SavedFilterRequest` | Void |
| `DeleteSavedFilter` | `delete_saved_filter` | Saved-filter ID | Void |
| `CreatePortfolio` | `create_portfolio` | `CreatePortfolioRequest` | JSON |
| `DeletePortfolio` | `delete_portfolio` | Portfolio ID | Void |
| `AddShareTransaction` | `add_share_transaction` | Portfolio ID, `ShareTransactionRequest` | JSON |

Go JSON results are `json.RawMessage`; Python JSON-result methods also have
`_json` variants returning bytes. Go void methods return only an error; Python
void methods return `None`. A successful empty `200` is not passed to a JSON
decoder, and portfolio deletion accepts the captured `204` response.

Python requests are frozen, keyword-only dataclasses exported from the package.
For example, `PriceAlertRequest(price="123.450000000000000001", condition="GTE")`
describes an alert without converting its price through a float. Numeric mutation
fields require plain decimal strings in Python and `json.Number` in Go. The Go
encoder emits JSON numeric tokens, not quoted numbers, preserving decimal
digits/trailing zeros. Exponents, NaN, infinity, and malformed decimals are
rejected. Alert prices must be positive; transaction numeric values preserve
the caller's sign rather than inventing undocumented transaction conventions.

`SavedFilterRequest` contains `label` and `saved_filter_data=SavedFilterData(...)`;
the Go wire body uses captured `savedFilterData`. Its filters are opaque string
IDs and an omitted/nil filter list becomes `[]`. `CreatePortfolioRequest`
contains name, currency (defaults to client currency), and a required string
investment category; the captured category is `ONE_TIME`, not a monetary amount.

`ShareTransactionRequest` requires trade date/type, ISIN, amount, quantity, quote,
quote/instrument/portfolio currencies, quote-currency type, trade/fund rates,
fees, and taxes; comment defaults to empty. All seven numeric fields must be
supplied explicitly, including `"0"` fees/taxes where applicable. Dates must use
valid YYYY-MM-DD. Category values are passed through, not restricted to a guessed
enum set. The client does not infer exchange rates or accounting arithmetic.

All seven operations use the non-replay authorization policy. A mutation 401 is
returned immediately without automatic refresh or replay. A transport failure,
timeout, cancellation, or invalid success response can happen after a write was
applied: reconcile account state through reads before deciding to retry. The
client does not silently duplicate a write or attempt an invented rollback.

Watchlist changes, transaction edits/deletes, and other uncaptured mutations
remain deferred until their request contracts are captured and reviewed.

## Concurrency and lifetime

- One client supports concurrent calls. `CDLL` releases the GIL during calls;
    Python locks protect handle access only, not network operations. JSON decoding
    still executes in Python and is not promised to run in parallel.
- Go uses a per-client pooled HTTP transport, allowing up to 10 idle connections
    per host. Registry locks are held only for lookup/registration/removal.
- `close()` is idempotent, rejects new work, cancels outstanding requests, and
    clears authentication and closes idle connections. Calls racing with close may succeed or report closure.
    Closing signals cancellation; it does not wait for every caller to return.
- Use `with` or explicit `close()`. A weak-reference finalizer is a fallback,
    not a substitute for deterministic cleanup.
- No Go pointers cross the C ABI. Successful responses are copied from Go to
    C-owned memory and then to Python-owned bytes. Every C buffer is freed in a
    Python `finally` block, including error paths. This is not zero-copy.
- Libraries remain loaded for the process lifetime. Do not call `FreeLibrary`,
    `dlclose`, or hot-reload the Go runtime. Load one library version per process.
    Do not use an initialized Go runtime after POSIX `fork`; use spawn instead.

## C ABI v1

The build emits a C header beside the library. It is the authority for C types
and calling conventions; Python declares every argument and return type.

| Entry Point | Contract |
| --- | --- |
| `justetf_abi_version` | Returns `1`; Python rejects other versions. |
| `justetf_client_new` | Takes UTF-8 JSON plus byte length; writes a nonzero `uint64_t` handle on success, zero on failure. |
| `justetf_profile` | Takes handle and 12 ISIN bytes; writes raw UTF-8 JSON on success or a JSON error object on failure. |
| `justetf_call` | Takes handle, operation name plus length, JSON parameters plus length, and output buffer; dispatches only registered operations. |
| `justetf_client_close` | Removes the handle and cancels its context. Stale handles are never recycled. |
| `justetf_buffer_free` | Frees one returned allocation; NULL is allowed. |

`JustETFBuffer` contains `void *data` and `size_t length`; bytes need not be
NUL-terminated. Provide a fresh writable output buffer for each call. Both
success and failure may allocate output: free `data` exactly once using
`justetf_buffer_free`, never Python's allocator or a different C runtime's `free`.
Do not reuse the pointer after freeing. Input memory stays caller-owned, must
remain valid for the call, and is never retained by Go. New-client configuration
is limited to 64 KiB; response bodies are limited to 4 MiB.

`justetf_call` is an additive ABI v1 export; the legacy `justetf_profile`
signature remains supported. Current Python wrappers use dispatch and require
an updated native library. Rebuild it if the wrapper reports a missing export.
Operation names are limited to 64 bytes and parameters to 64 KiB. Parameters
must be one JSON object matching the selected operation, not an arbitrary URL,
HTTP method, or path. Invalid operations/parameters return status `1` before
any upstream request. The allowlist contains `get_profile`, the eight
search/discovery operations, the fifteen financial REST operations, the fifteen
authenticated reads, the seven mutation operations above, and
`login`, `refresh`, `logout`, `is_authenticated`. Login parameters are the
`email` and `password` strings; the other session operations require `{}`.
Login/refresh/logout return NULL data and length zero; status returns JSON bool.
For example:

```json
{"operation": "get_profile", "parameters": {"isin": "IE00B5BMR087"}}
```

This illustrates the two logical arguments, not a wire envelope: the ABI takes
the operation name and parameter object separately. Do not expose internal
dispatch as an unrestricted HTTP proxy or add excluded tracking operations.

Status codes: `0` success, `1` invalid argument, `2` HTTP API error, `3` transport
or upstream response error, `4` closed/invalid handle, `5` internal failure,
`6` authentication required or session changed. Existing codes and ABI v1 entry
points are unchanged; rebuild the library for the new dispatcher operations.
Errors contain `message` and optionally an HTTP `status`; no shared last-error
state exists. Ordinary Go panics at fallible exports become status `5` without
exposing panic details. Invalid foreign pointers, double frees, fatal runtime
errors, and out-of-memory failures cannot be made safe by panic recovery.

## Go usage

The root package is `justetf`; import it from `github.com/wavepacket0/justetf-mobile-client`.

```go
client, err := justetf.NewClient(justetf.Config{
        Timeout: 15 * time.Second,
})
if err != nil {
        return err
}
defer client.Close()
profile, err := client.GetProfile(ctx, "IE00B5BMR087")
```

Direct Go consumers can supply per-request contexts for cancellation. The
native binding currently exposes a per-client timeout and client-wide close
cancellation, not per-request cancellation handles.
`CloseIdleConnections()` only manages the connection pool; use `Close()` to
cancel work, clear authentication, and end the client's lifecycle.

The local `justETF/` research directory is ignored and is not part of the final
repository. Runtime code, builds, installation, and routine tests do not depend
on it. Only optional capture-generation tooling may read private local inputs.
Do not publish raw traffic, personal data, APKs, or virtual environments.

Protocol defaults and implemented endpoint paths live in `constants.go`;
add endpoint constants as features are implemented rather than copying the full
research catalog into runtime code. Country, currency, and language remain
per-client configuration. Tests retain independent expected protocol literals.
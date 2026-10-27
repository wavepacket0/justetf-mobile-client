# Tests

One Python mock backend and one committed synthetic endpoint catalogue serve the
Go and Python integration suites. The clients execute the same endpoint calls and
field/type/identity assertions against either localhost or the real justETF API.
Changing tiers changes the target and credentials, not the expected contracts.
The catalogue is handwritten test data, not a dump of captured account responses.
Private `testdata/`, captures, and generation tooling are not test dependencies.

## Run

From the repository root, with Go 1.22+, Python 3.10+, cgo and a compatible C compiler:

```powershell
python -m pip install -r test/requirements.txt
python test/run.py
```

The runner loads the project `.env` using python-dotenv without variable
interpolation. Existing process environment values override `.env`; `--mode`
overrides both. The runtime client does not load `.env` itself. Test-only
dependencies do not become runtime Python package dependencies.

Configure the single selector in `.env`:

```dotenv
JUSTETF_TEST_MODE=offline
JUSTETF_TEST_EMAIL=
JUSTETF_TEST_PASSWORD=
```

See [../.env.example](../.env.example). Real account credentials stay in the ignored
`.env` or a CI secret store. The shared APK API key is supplied by Go's protocol
defaults, not required in `.env`. Never pass passwords on a command line.

## Three Tiers

- `offline` (default): starts a Python server on `127.0.0.1` with an ephemeral port,
	uses fake credentials even if `.env` contains real ones, and runs reads, scoped
	mutations, controlled failures, and unit tests. No real API calls are made.
- `online-readonly`: runs the same read contracts and login/refresh against the
	official HTTPS API. It never creates/deletes account records. Login and refresh
	create/rotate authentication tokens. Missing account credentials fail the run.
- `online-mutations`: additionally creates uniquely named portfolios, saved
	filters, alerts, and a share transaction within a newly created portfolio. Use
	only a dedicated, disposable account. Selecting this tier explicitly permits
	those live writes. No pre-existing account records should be modified.

The former `JUSTETF_LIVE` and `JUSTETF_LIVE_AUTH` flags are retired.
Online failures never fall back to mock responses. Offline failures never switch
to the real service. Online integration targets are pinned to the official host.

Override the file for a single invocation:

```powershell
python test/run.py --mode offline
python test/run.py --mode online-readonly
python test/run.py --mode online-mutations
```

Use `--language go` or `--language python` to focus on one client; both run by
default. The runner builds the native test library and bypasses Go's test cache
with `-count=1`. It stops on a failed suite and returns a nonzero exit status.

## Coverage

The shared catalogue covers all 24 public reads and 15 account reads currently
implemented, plus the seven write operations exercised by scoped workflows.
Both suites also test login and explicit token refresh. Local logout/session
state is covered by offline tests. Readonly tests discover portfolio and alert
IDs from the test account; missing resources are explicitly skipped as unverified,
not invented and not created as a side effect of readonly testing.

Expectations assert selected required fields, types, nullability, and stable
identity rather than exact changing financial values. Extra fields are accepted.
Synthetic dates and prices are examples, not evidence of current market accuracy.
The mock models the documented test scenarios, not the entire upstream business
engine, permissions, quotas, or all possible response variants.

Go unit tests remain beside their packages and use local stubs when they need
precise cancellation, refresh, connection, or error control. Python fault and
native-ownership tests use the shared server's fault handler and run offline only.
These tests are intentionally not redirected to production. Running `go test
./...` directly runs units; use the runner for the backend-selected contracts.

## Cleanup

Each write workflow registers cleanup immediately, retains created IDs before
further assertions, and deletes only resources it can re-identify by its unique
test name/comment. Saved-filter IDs are discovered by the unique label. The share
transaction is removed by deleting its test-created parent portfolio; there is
no standalone transaction-delete API in this client.

Cleanup runs after assertion failures with independent request timeouts. Ownership
mismatches, missing IDs, and cleanup errors fail the test and report the test
resource label/ID without printing response bodies, credentials, or tokens.
Offline runs additionally verify that the shared mock account state is restored.

Cleanup is best-effort, not a transaction rollback. A process crash, timeout after
a committed write, or unreachable server can leave resources behind. Reconcile
reported leftovers manually; never blindly retry writes or delete unrelated data.
Alerts can have irreversible external effects, such as notifications, even after
deletion. Live mutation runs therefore require a disposable account.

## Release Evidence

An offline pass is not a live compatibility claim. Record the date, revision,
tier, exercised endpoints, skips, and outcome of real runs without sensitive data.
Missing credentials, skipped resource checks, and unexecuted online tiers remain
UNVERIFIED. A passing live run establishes compatibility only for the exercised
cases at that time. Online tiers have not been executed merely by configuring them.

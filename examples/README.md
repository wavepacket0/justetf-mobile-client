# Examples

One runnable file per language, each with four subcommands that print the
decoded JSON response. Set `JUSTETF_BASE_URL` to target a mock server.

| Command | What it shows | Go | Python |
| --- | --- | --- | --- |
| `profile [ISIN]` | One ETF by ISIN, error handling | `go run ./examples/go profile` | `python examples/python/main.py profile` |
| `search QUERY` | Free-text search with typed options | `go run ./examples/go search world` | `python examples/python/main.py search world` |
| `analyze [ISIN]` | Quote, returns, risks, country exposure | `go run ./examples/go analyze` | `python examples/python/main.py analyze` |
| `account` | Login, watchlists, portfolios (read-only) | `go run ./examples/go account` | `python examples/python/main.py account` |

`account` reads `JUSTETF_EMAIL` and `JUSTETF_PASSWORD` from the process
environment. A `.env` file is not read, and these differ from the test-only
`JUSTETF_TEST_*` variables.

```bash
export JUSTETF_EMAIL=you@example.com JUSTETF_PASSWORD=...   # PowerShell: $env:JUSTETF_EMAIL = "..."
```

The default ISIN is `IE000VAHT5T0`.

## Python setup

Build the native library (see the top-level README), install the package, and
point `JUSTETF_LIBRARY` at the library unless you installed a platform wheel:

```bash
python -m pip install -e ./python
export JUSTETF_LIBRARY=bin/libjustetf-native.so   # PowerShell: $env:JUSTETF_LIBRARY = "bin/justetf-native.dll"
```

"""Usage: python examples/python/main.py <profile|search|analyze|account> [args]"""
import json
import os
import sys
from typing import Any

from justetf_mobile_client import APIError, Client, ExposureOptions, SearchOptions

DEFAULT_ISIN = "IE000VAHT5T0"


def print_json(title: str, body: Any) -> None:
    print(f"== {title} ==")
    print(json.dumps(body, indent=2, ensure_ascii=False), end="\n\n")


def profile(client: Client, isin: str) -> None:
    """A single call with typed error handling."""
    try:
        print_json("profile", client.get_profile(isin))
    except APIError as error:
        raise SystemExit(f"justETF rejected the request (HTTP {error.status_code})")


def search(client: Client, query: str) -> None:
    """Typed options; unset fields fall back to client defaults."""
    print_json("search", client.search_etfs(SearchOptions(query=query, limit=5)))


def analyze(client: Client, isin: str) -> None:
    print_json("quote", client.get_quote(isin))
    print_json("returns", client.get_returns(isin))
    print_json("risks", client.get_risks(isin))
    print_json("country exposure", client.get_country_exposure(isin, ExposureOptions(top=5)))


def account(client: Client) -> None:
    """Log in with credentials from the environment and read account data.

    The client keeps tokens internally and refreshes expired sessions for account reads.
    """
    email, password = os.environ.get("JUSTETF_EMAIL"), os.environ.get("JUSTETF_PASSWORD")
    if not email or not password:
        raise SystemExit("set JUSTETF_EMAIL and JUSTETF_PASSWORD")
    client.login(email, password)
    print("authenticated:", client.is_authenticated())
    try:
        print_json("watchlists", client.get_watchlists())
        print_json("portfolios", client.get_portfolios())
    finally:
        client.logout()


def main(argv: list[str]) -> None:
    if not argv:
        raise SystemExit("usage: <profile [ISIN] | search QUERY | analyze [ISIN] | account>")
    command, rest = argv[0], argv[1:]
    # Library path comes from JUSTETF_LIBRARY, or the bundled library in a wheel.
    with Client(timeout=30.0) as client:
        if command == "profile":
            profile(client, rest[0] if rest else DEFAULT_ISIN)
        elif command == "search" and rest:
            search(client, " ".join(rest))
        elif command == "analyze":
            analyze(client, rest[0] if rest else DEFAULT_ISIN)
        elif command == "account":
            account(client)
        else:
            raise SystemExit(f"unknown or incomplete command: {command}")


if __name__ == "__main__":
    main(sys.argv[1:])

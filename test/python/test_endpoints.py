import json
import os
import unittest
import uuid
from types import SimpleNamespace
from unittest.mock import patch
from pathlib import Path
from urllib.parse import urlsplit

import justetf_mobile_client as api


CATALOG = json.loads((Path(__file__).resolve().parents[1] / "contracts.json").read_text(encoding="utf-8"))
CASES = {case["name"]: case for case in CATALOG}


def validate_fields(value, fields):
    if not isinstance(value, dict):
        raise AssertionError("expected response object")
    for field, accepted in fields.items():
        if field not in value:
            raise AssertionError(f"required field {field} is missing")
        member = value[field]
        kind = "null" if member is None else "boolean" if isinstance(member, bool) else "number" if isinstance(member, (int, float)) else "string" if isinstance(member, str) else "array" if isinstance(member, list) else "object"
        if kind not in accepted.split("|"):
            raise AssertionError(f"field {field}: expected {accepted}, received {kind}")


@unittest.skipUnless(os.environ.get("JUSTETF_TEST_BASE_URL"), "run python test/run.py to select the contract-test backend")
class EndpointTests(unittest.TestCase):
    def setUp(self):
        self.mode = os.environ["JUSTETF_TEST_MODE"]
        target = os.environ["JUSTETF_TEST_BASE_URL"]
        parsed = urlsplit(target)
        if self.mode == "offline":
            self.assertTrue(parsed.scheme == "http" and parsed.hostname == "127.0.0.1", "offline contracts require the loopback mock server")
        else:
            self.assertTrue(self.mode in ("online-readonly", "online") and target == "https://api.mobile.justetf.com", "online contracts require the official HTTPS endpoint")
        self.client = api.Client(os.environ["JUSTETF_TEST_LIBRARY"], base_url=target, language="en", timeout=20)
        self.addCleanup(self.client.close)
        self.email = os.environ["JUSTETF_TEST_EMAIL"]
        self.call(self.client.login, self.email, os.environ["JUSTETF_TEST_PASSWORD"])
        self.call(self.client.refresh)
        account = self.call(self.client.get_personal_data)
        self.assertTrue(isinstance(account, dict) and isinstance(account.get("email"), str) and account["email"].casefold() == self.email.casefold(), "account identity does not match the configured test account")

    def call(self, method, *arguments):
        try:
            return method(*arguments)
        except api.APIError as error:
            self.fail(f"{method.__name__}: HTTP {error.status_code}")
        except api.ClientError:
            self.fail(f"{method.__name__}: client/transport failure (details withheld)")

    def test_endpoint_contracts(self):
        identifiers = {}
        for case in CATALOG:
            if case.get("mutation"):
                continue
            with self.subTest(endpoint=case["name"]):
                arguments = []
                missing = False
                for argument in case.get("arguments", []):
                    if argument.startswith("$"):
                        if argument[1:] not in identifiers:
                            missing = True
                            break
                        argument = identifiers[argument[1:]]
                    arguments.append(argument)
                if missing:
                    if self.mode == "offline":
                        self.fail("mock seed resource missing")
                    self.skipTest("readonly account has no resource for this endpoint; coverage unverified")
                if case.get("options_type"):
                    arguments.append(getattr(api, case["options_type"])(**case.get("options", {})))
                response = self.call(getattr(self.client, case["name"]), *arguments)
                validate_fields(response, case["fields"])
                if "isin" in case["fields"]:
                    self.assertTrue(response["isin"] == arguments[0], "response instrument identity differs from request")
                if case["name"] == "get_profile":
                    self.assertTrue(bool(response["name"].strip()), "profile name must not be empty")
                if case["name"] == "get_portfolios":
                    for portfolio in response["portfolios"]:
                        identifier = str(portfolio.get("id", ""))
                        if identifier.isdecimal() and int(identifier) > 0:
                            identifiers["portfolio_id"] = identifier
                            break
                if case["name"] == "get_etf_price_alerts" and response["alerts"]:
                    identifiers["alert_id"] = str(response["alerts"][0]["id"])

    def cleanup_owned(self, delete, identifier, label, *prefix):
        if identifier[0] is None:
            self.fail(f"write outcome unresolved; reconcile test resource {label}")
        try:
            if delete.__name__ == "delete_portfolio":
                owned = self.client.get_portfolio(identifier[0]).get("name") == label
            elif delete.__name__ == "delete_price_alert":
                owned = self.client.get_price_alert(*prefix, identifier[0]).get("comment") == label
            else:
                owned = any(str(entry.get("id")) == identifier[0] and entry.get("label") == label for entry in self.client.get_saved_filters()["savedFilters"])
            if not owned:
                self.fail(f"cleanup refused: resource ownership mismatch for {label}, id={identifier[0]}")
            delete(*prefix, identifier[0])
        except (api.ClientError, ValueError):
            self.fail(f"cleanup failed; leftover test resource {label}, id={identifier[0]}")

    def test_mutation_workflows(self):
        if self.mode == "online-readonly":
            self.skipTest("account mutations require online mode")
        prefix = "sdk-test-" + uuid.uuid4().hex
        isin = "IE00B5BMR087"
        portfolio_id = [None]
        self.addCleanup(self.cleanup_owned, self.client.delete_portfolio, portfolio_id, prefix + "-portfolio")
        portfolio = self.call(self.client.create_portfolio, api.CreatePortfolioRequest(name=prefix + "-portfolio", investment="ONE_TIME"))
        if isinstance(portfolio, dict) and portfolio.get("id") is not None:
            portfolio_id[0] = str(portfolio["id"])
        validate_fields(portfolio, CASES["create_portfolio"]["fields"])
        self.assertTrue(portfolio["name"] == prefix + "-portfolio", "created portfolio identity mismatch")
        self.assertTrue(self.call(self.client.get_portfolio, portfolio_id[0])["name"] == prefix + "-portfolio", "portfolio read-after-create mismatch")
        transaction = api.ShareTransactionRequest(
            trade_date="2026-10-14", transaction_type="DELIVERY", amount="123.45", isin=isin,
            quantity="1", quote="123.45", quote_currency="EUR", instrument_currency="EUR",
            portfolio_currency="EUR", quote_currency_type="INSTRUMENT", trade_rate="1", fund_rate="1", fees="0", taxes="0", comment=prefix,
        )
        created = self.call(self.client.add_share_transaction, portfolio_id[0], transaction)
        validate_fields(created, CASES["add_share_transaction"]["fields"])
        entries = self.call(self.client.get_portfolio_transactions, portfolio_id[0])
        self.assertTrue(any(entry.get("isin") == isin for entry in entries["transactions"]), "transaction missing after successful write")
        filter_id = [None]
        self.addCleanup(self.cleanup_owned, self.client.delete_saved_filter, filter_id, prefix + "-filter")
        self.call(self.client.create_saved_filter, api.SavedFilterRequest(label=prefix + "-filter"))
        filters = self.call(self.client.get_saved_filters)
        matches = [entry for entry in filters["savedFilters"] if entry.get("label") == prefix + "-filter"]
        if len(matches) == 1:
            filter_id[0] = str(matches[0]["id"])
        self.assertTrue(filter_id[0] is not None, "created saved filter cannot be identified uniquely")
        alert_id = [None]
        self.addCleanup(self.cleanup_owned, self.client.delete_price_alert, alert_id, prefix + "-alert", isin)
        alert = self.call(self.client.create_price_alert, isin, api.PriceAlertRequest(price="9999999", condition="GTE", comment=prefix + "-alert"))
        if isinstance(alert, dict) and alert.get("id") is not None:
            alert_id[0] = str(alert["id"])
        validate_fields(alert, CASES["create_price_alert"]["fields"])
        self.assertTrue(self.call(self.client.get_price_alert, isin, alert_id[0])["comment"] == prefix + "-alert", "alert read-after-create mismatch")


class CleanupSafetyTests(unittest.TestCase):
    def test_readonly_tier_skips_writes(self):
        endpoint = EndpointTests("test_mutation_workflows")
        endpoint.mode = "online-readonly"
        with self.assertRaises(unittest.SkipTest):
            endpoint.test_mutation_workflows()

    def test_wrong_backend_is_rejected_before_client_creation(self):
        for mode, target in (
            ("offline", "https://api.mobile.justetf.com"),
            ("online-readonly", "http://127.0.0.1:1"),
            ("online", "https://example.invalid"),
        ):
            endpoint = EndpointTests("test_endpoint_contracts")
            with self.subTest(mode=mode), patch.dict(os.environ, {"JUSTETF_TEST_MODE": mode, "JUSTETF_TEST_BASE_URL": target}):
                with self.assertRaisesRegex(AssertionError, "require"):
                    endpoint.setUp()

    def test_cleanup_refuses_unowned_resource(self):
        endpoint = EndpointTests("test_mutation_workflows")
        deleted = []

        def delete_portfolio(identifier):
            deleted.append(identifier)

        endpoint.client = SimpleNamespace(get_portfolio=lambda identifier: {"name": "preexisting-resource"})
        with self.assertRaisesRegex(AssertionError, "ownership mismatch"):
            endpoint.cleanup_owned(delete_portfolio, ["123456"], "sdk-test-owned")
        self.assertEqual(deleted, [])

    def test_cleanup_reports_ambiguous_create_without_deletion(self):
        endpoint = EndpointTests("test_mutation_workflows")
        deleted = []
        with self.assertRaisesRegex(AssertionError, "write outcome unresolved"):
            endpoint.cleanup_owned(lambda identifier: deleted.append(identifier), [None], "sdk-test-owned")
        self.assertEqual(deleted, [])

    def test_cleanup_failure_is_redacted(self):
        endpoint = EndpointTests("test_mutation_workflows")
        endpoint.client = SimpleNamespace(get_portfolio=lambda identifier: {"name": "sdk-test-owned"})

        def delete_portfolio(identifier):
            raise api.TransportError("private response detail")

        with self.assertRaisesRegex(AssertionError, "cleanup failed") as caught:
            endpoint.cleanup_owned(delete_portfolio, ["123456"], "sdk-test-owned")
        self.assertNotIn("private response detail", str(caught.exception))
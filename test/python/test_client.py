import base64
import ctypes
import gc
import json
import os
import subprocess
import sys
import threading
import unittest
import weakref
from unittest.mock import patch
from concurrent.futures import ThreadPoolExecutor
from decimal import Decimal
from dataclasses import replace
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from queue import Queue
from urllib.parse import parse_qs, urlsplit

from justetf_mobile_client import (
    APIError, AuthenticationError, Client, ClientClosedError, TransportError, _Buffer, _decode_result,
    CardsOptions, ComparisonOptions, HoldingSearchOptions, SearchISINsOptions,
    SearchMetadataOptions, SearchOptions,
    ChartOptions, CurrencyOptions, ExposureOptions, HoldingETFsOptions,
    IndexPerformanceOptions, MarketOptions, MarketOverviewOptions, OrderFeesOptions,
    PeriodEndOptions, SavingsPlansOptions,
    AllocationOptions, PageOptions, PortfolioChartOptions, PortfolioCompositionOptions,
    PortfolioListOptions, PortfolioOptions, TradeQuoteOptions, WatchlistOptions,
    CreatePortfolioRequest, PriceAlertRequest, SavedFilterData, SavedFilterRequest, ShareTransactionRequest,
)

from mock_server import (
    ACCOUNT_ROUTES, DISCOVERY_ROUTES, FINANCIAL_ROUTES, MUTATION_ROUTES,
    ProfileHandler, ProfileServer, account_path,
)


@unittest.skipIf(os.environ.get("JUSTETF_TEST_MODE", "offline") != "offline", "controlled failure tests run offline only")
class ClientTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        root = Path(__file__).resolve().parents[2]
        cls.library = Path(os.environ["JUSTETF_TEST_LIBRARY"]) if os.environ.get("JUSTETF_TEST_LIBRARY") else root / "bin" / (
            "justetf-native-test.dll" if os.name == "nt"
            else "libjustetf-native-test.dylib" if sys.platform == "darwin"
            else "libjustetf-native-test.so"
        )
        if not os.environ.get("JUSTETF_TEST_LIBRARY"):
            subprocess.run(
                ["go", "build", "-buildmode=c-shared", "-o", str(cls.library), "./cmd/justetf-native"],
                cwd=root,
                check=True,
            )
        cls.server = ProfileServer(("127.0.0.1", 0), ProfileHandler)
        cls.addClassCleanup(cls.server.server_close)
        cls.worker = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.worker.start()
        cls.addClassCleanup(cls.worker.join)
        cls.addClassCleanup(cls.server.shutdown)
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    def make_client(self) -> Client:
        return Client(self.library, api_key="fake-test-key", base_url=self.base_url)

    def mutation_cases(self):
        isin = "IE00B5BMR087"
        identifier = "11111111-2222-4333-8444-555555555555"
        transaction = ShareTransactionRequest(
            trade_date="2026-10-14", transaction_type="DELIVERY", amount="123.450000000000000001",
            isin=isin, quantity="1.000000000000000001", quote="123.45", quote_currency="EUR",
            instrument_currency="EUR", portfolio_currency="EUR", quote_currency_type="INSTRUMENT",
            trade_rate="1", fund_rate="1", fees="0", taxes="0", comment="Synthetic transaction",
        )
        return (
            ("create_price_alert", (isin, PriceAlertRequest(price="123.450000000000000001", condition="GTE", comment="Synthetic alert")), {"price":Decimal("123.450000000000000001"),"condition":"GTE","comment":"Synthetic alert"}),
            ("delete_price_alert", (isin, identifier), {}),
            ("create_saved_filter", (SavedFilterRequest(label="Synthetic filter", saved_filter_data=SavedFilterData(filters=["one","two"], keywords="world")),), {"label":"Synthetic filter","savedFilterData":{"filters":["one","two"],"keywords":"world"}}),
            ("delete_saved_filter", (identifier,), {}),
            ("create_portfolio", (CreatePortfolioRequest(name="Synthetic portfolio", investment="ONE_TIME"),), {"name":"Synthetic portfolio","currency":"EUR","investment":"ONE_TIME"}),
            ("delete_portfolio", ("123456",), {}),
            ("add_share_transaction", ("123456", transaction), {"tradeDate":"2026-10-14","transactionType":"DELIVERY","amount":Decimal("123.450000000000000001"),"comment":"Synthetic transaction","isin":isin,"quantity":Decimal("1.000000000000000001"),"quote":Decimal("123.45"),"quoteCurrency":"EUR","instrumentCurrency":"EUR","portfolioCurrency":"EUR","quoteCurrencyType":"INSTRUMENT","tradeRate":1,"fundRate":1,"fees":0,"taxes":0}),
        )

    def test_mutation_contracts(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            for name, arguments, _ in self.mutation_cases():
                with self.subTest(anonymous=name), self.assertRaises(AuthenticationError):
                    getattr(client, name)(*arguments)
            self.assertTrue(self.server.mutation_requests.empty())
            client.login("test@example.invalid", "test:password")
            for name, arguments, expected_body in self.mutation_cases():
                method, template = MUTATION_ROUTES[name]
                route = (method, account_path(template))
                fixture = self.server.mutation_contracts[route]
                for raw in ((False, True) if name in ("create_price_alert","create_portfolio","add_share_transaction") else (False,)):
                    with self.subTest(operation=name, raw=raw):
                        result = getattr(client, name + ("_json" if raw else ""))(*arguments)
                        self.assertEqual(json.loads(result) if raw else result, fixture.get("response_body"))
                        request = self.server.mutation_requests.get(timeout=2)
                        self.assertEqual((request["method"], request["path"]), route)
                        self.assertEqual(request["query"], "")
                        self.assertEqual(request["body"], expected_body)
                        self.assertTrue(request["authenticated"])
            self.assertEqual(self.server.auth_refreshes, 0)

    def test_mutation_currency_overrides(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            client.login("test@example.invalid", "test:password")
            client.create_portfolio(CreatePortfolioRequest(name="USD portfolio", currency="USD", investment="ONE_TIME"))
            self.assertEqual(self.server.mutation_requests.get(timeout=2)["body"]["currency"], "USD")
            transaction = self.mutation_cases()[-1][1][1]
            client.add_share_transaction("123456", replace(
                transaction, quote_currency="USD", instrument_currency="GBP", portfolio_currency="EUR",
                trade_rate="0.900000000000000001", fund_rate="1.100000000000000001",
            ))
            body = self.server.mutation_requests.get(timeout=2)["body"]
            self.assertEqual((body["quoteCurrency"], body["instrumentCurrency"], body["portfolioCurrency"]), ("USD", "GBP", "EUR"))
            self.assertEqual(body["tradeRate"], Decimal("0.900000000000000001"))
            self.assertEqual(body["fundRate"], Decimal("1.100000000000000001"))

    def test_mutation_never_replays(self) -> None:
        for mode in ("unauthorized", "dropped-response"):
            self.server.reset_authentication()
            with self.make_client() as client:
                client.login("test@example.invalid", "test:password")
                self.server.mutation_status = 401 if mode == "unauthorized" else None
                self.server.mutation_drop = mode == "dropped-response"
                try:
                    for name, arguments, _ in self.mutation_cases():
                        with self.subTest(operation=name, mode=mode):
                            expected_error = APIError if mode == "unauthorized" else TransportError
                            with self.assertRaises(expected_error) as caught:
                                getattr(client, name)(*arguments)
                            self.assertNotIn("private", str(caught.exception))
                            if mode == "unauthorized":
                                self.assertEqual(caught.exception.status_code, 401)
                            self.server.mutation_requests.get(timeout=2)
                            self.assertTrue(self.server.mutation_requests.empty(), "write was replayed")
                            self.assertEqual(self.server.auth_refreshes, 0)
                finally:
                    self.server.mutation_status = None
                    self.server.mutation_drop = False

    def test_mutation_validation_and_empty_filters(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            client.login("test@example.invalid", "test:password")
            for call in (
                lambda: client.create_price_alert("IE00B5BMR087", PriceAlertRequest(price="NaN", condition="GTE")),
                lambda: client.create_price_alert("IE00B5BMR087", PriceAlertRequest(price="0", condition="GTE")),
                lambda: client.delete_price_alert("IE00B5BMR087", "../private"),
                lambda: client.delete_saved_filter("../private"),
                lambda: client.create_saved_filter(SavedFilterRequest(label="")),
                lambda: client.create_portfolio(CreatePortfolioRequest(name="Synthetic", investment="")),
                lambda: client.delete_portfolio("can-create"),
            ):
                with self.assertRaises(ValueError):
                    call()
            with self.assertRaises(TypeError):
                client.create_price_alert("IE00B5BMR087", PriceAlertRequest(price=123.45, condition="GTE"))
            self.assertTrue(self.server.mutation_requests.empty())
            self.assertIsNone(client.create_saved_filter(SavedFilterRequest(label="Empty filters")))
            request = self.server.mutation_requests.get(timeout=2)
            self.assertEqual(request["body"], {"label":"Empty filters","savedFilterData":{"filters":[],"keywords":""}})

    def test_exported_abi_contract(self) -> None:
        header = self.library.with_suffix(".h").read_text(encoding="utf-8")
        self.assertIn("} JustETFBuffer;", header)
        with self.make_client() as client:
            self.assertEqual(client._native.lib.justetf_abi_version(), 1)
            for name in (
                "justetf_abi_version",
                "justetf_client_new",
                "justetf_profile",
                "justetf_call",
                "justetf_client_close",
                "justetf_buffer_free",
            ):
                with self.subTest(symbol=name):
                    self.assertTrue(callable(getattr(client._native.lib, name)))
                    self.assertIn(name + "(", header)

    def test_repeated_lookups_and_cleanup(self) -> None:
        with self.make_client() as client:
            ports = set()
            for _ in range(2):
                profile = client.get_profile("IE00B5BMR087")
                ports.add(profile["peer_port"])
                self.assertEqual(profile["isin"], "IE00B5BMR087")
                self.assertEqual(profile["name"], "Example ETF")
                self.assertEqual(profile["api_key"], "fake-test-key")
                self.assertEqual(profile["language"], "it")
                self.assertEqual(profile["user_agent"], "Dart/3.12 (dart:io)")
                self.assertEqual(profile["request_body"], "{}")
                self.assertEqual(
                    profile["query"],
                    {"currency": ["EUR"], "universeCountry": ["IT"], "universeType": ["PRIVATE"]},
                )
            self.assertEqual(len(ports), 1)
            self.assertIsInstance(client.get_profile_json("IE00B5BMR087"), bytes)
        client.close()
        with self.assertRaises(ClientClosedError):
            client.get_profile("IE00B5BMR087")

    def test_api_error_does_not_end_session(self) -> None:
        with self.make_client() as client:
            with self.assertRaises(APIError) as caught:
                client.get_profile("IE00B4L5Y983")
            self.assertEqual(caught.exception.status_code, 401)
            self.assertNotIn("private", str(caught.exception))
            self.assertEqual(client.get_profile("IE00B5BMR087")["isin"], "IE00B5BMR087")

    def test_validation_error(self) -> None:
        with self.make_client() as client:
            with self.assertRaises(ValueError):
                client.get_profile("../invalid")
            with self.assertRaises(TypeError):
                client.get_profile(None)

    def test_concurrent_calls(self) -> None:
        with self.make_client() as client, ThreadPoolExecutor(max_workers=8) as executor:
            results = list(executor.map(client.get_profile, ["IE00B5BMR087"] * 32))
        self.assertEqual(len(results), 32)
        self.assertTrue(all(profile["isin"] == "IE00B5BMR087" for profile in results))

    def test_api_key_defaults_and_overrides(self) -> None:
        for environment, override, expected in (
            ({}, None, None),
            ({}, "", None),
            ({"JUSTETF_API_KEY": "environment-test-key"}, None, "environment-test-key"),
            ({"JUSTETF_API_KEY": "environment-test-key"}, "explicit-test-key", "explicit-test-key"),
        ):
            with self.subTest(override=override, expected=expected):
                with patch.dict(os.environ, environment, clear=True):
                    with Client(self.library, api_key=override, base_url=self.base_url) as client:
                        profile = client.get_profile("IE00B5BMR087")
                        if expected is None:
                            self.assertTrue(isinstance(profile["api_key"], str) and bool(profile["api_key"]))
                        else:
                            self.assertEqual(profile["api_key"], expected)

    def test_shared_server_uses_public_catalogue(self) -> None:
        original_read = Path.read_text

        def read_public(path, *arguments, **keywords):
            self.assertNotIn("testdata", path.parts)
            return original_read(path, *arguments, **keywords)

        with patch.object(Path, "read_text", autospec=True, side_effect=read_public):
            with ProfileServer(("127.0.0.1", 0), ProfileHandler) as server:
                self.assertEqual(len(server.catalog), 46)
                self.assertEqual(server.contracts[DISCOVERY_ROUTES["search_etfs"]]["etfs"][0]["isin"], "IE00B5BMR087")
                self.assertEqual(server.account_contracts[("GET", ACCOUNT_ROUTES["get_personal_data"][0])]["email"], "test@example.invalid")
                self.assertEqual(server.mutation_contracts[MUTATION_ROUTES["create_portfolio"]]["status"], 201)
                self.assertNotIn("response_body", server.mutation_contracts[("DELETE", account_path(MUTATION_ROUTES["delete_portfolio"][1]))])

    def test_invalid_configuration(self) -> None:
        with self.assertRaises(ValueError):
            Client(self.library, api_key="fake-test-key", base_url="not-a-url")
        for timeout in (0, -1, float("inf"), float("nan"), 301):
            with self.subTest(timeout=timeout), self.assertRaises(ValueError):
                Client(self.library, api_key="fake-test-key", timeout=timeout)

    def test_parallel_requests_overlap(self) -> None:
        with self.make_client() as client, ThreadPoolExecutor(max_workers=2) as executor:
            futures = [executor.submit(client.get_profile, "IE00BKM4GZ66") for _ in range(2)]
            for future in futures:
                self.assertEqual(future.result(timeout=5)["isin"], "IE00BKM4GZ66")

    def test_close_cancels_active_request(self) -> None:
        self.server.started.clear()
        self.server.release.clear()
        try:
            with self.make_client() as client, ThreadPoolExecutor(max_workers=1) as executor:
                future = executor.submit(client.get_profile, "IE00BK5BQT80")
                self.assertTrue(self.server.started.wait(timeout=2))
                client.close()
                with self.assertRaises(ClientClosedError):
                    future.result(timeout=2)
        finally:
            self.server.release.set()

    def test_timeout(self) -> None:
        self.server.started.clear()
        self.server.release.clear()
        try:
            with Client(self.library, api_key="test", base_url=self.base_url, timeout=0.05) as client:
                with self.assertRaises(TransportError):
                    client.get_profile("IE00BK5BQT80")
        finally:
            self.server.release.set()

    def test_finalizer_closes_native_handle(self) -> None:
        client = self.make_client()
        native, handle = client._native, client._handle
        reference = weakref.ref(client)
        del client
        gc.collect()
        self.assertIsNone(reference())
        output = _Buffer()
        status = native.lib.justetf_profile(handle, b"IE00B5BMR087", 12, ctypes.byref(output))
        self.assertEqual(status, 4)
        with self.assertRaises(ClientClosedError):
            native.consume(status, output)

    def test_invalid_abi_arguments_and_stale_handles(self) -> None:
        with self.make_client() as client:
            native, handle = client._native, client._handle
            output = _Buffer()
            created = ctypes.c_uint64(999)
            status = native.lib.justetf_client_new(None, 0, ctypes.byref(created), ctypes.byref(output))
            self.assertEqual(status, 1)
            self.assertEqual(created.value, 0)
            with self.assertRaises(ValueError):
                native.consume(status, output)
            self.assertEqual(native.lib.justetf_profile(handle, None, 0, None), 1)
            native.lib.justetf_buffer_free(None)
        for _ in range(8):
            with self.make_client() as another:
                self.assertNotEqual(another._handle, handle)
        output = _Buffer()
        status = native.lib.justetf_profile(handle, b"IE00B5BMR087", 12, ctypes.byref(output))
        self.assertEqual(status, 4)
        with self.assertRaises(ClientClosedError):
            native.consume(status, output)

    def test_dispatch_validation(self) -> None:
        with self.make_client() as client:
            native, handle = client._native, client._handle
            for operation, name_length, parameters, params_length in (
                (None, 0, b"{}", 2),
                (b"get_profile", 65, b"{}", 2),
                (b"get_profile", 11, None, 0),
                (b"get_profile", 11, b"{}", 65537),
                (b"private_operation", 17, b"{}", 2),
                (b"get_profile", 11, b"null", 4),
                (b"get_profile", 11, b"[]", 2),
                (b"get_profile", 11, b'{"isin":123}', 12),
            ):
                with self.subTest(operation=operation, size=params_length):
                    output = _Buffer()
                    status = native.lib.justetf_call(
                        handle, operation, name_length, parameters, params_length, ctypes.byref(output)
                    )
                    self.assertEqual(status, 1)
                    with self.assertRaises(ValueError) as caught:
                        native.consume(status, output)
                    self.assertNotIn("private_operation", str(caught.exception))
            self.assertEqual(native.lib.justetf_call(handle, None, 0, None, 0, None), 1)
            with self.assertRaises(ValueError):
                client._call_json("get_profile", {"isin": "IE00B5BMR087", "private_field": "private_value"})
            with self.assertRaises(ValueError):
                client._call_json("https://private.invalid/", {})
        output = _Buffer()
        status = native.lib.justetf_call(handle, b"get_profile", 11, b"{}", 2, ctypes.byref(output))
        self.assertEqual(status, 4)
        with self.assertRaises(ClientClosedError):
            native.consume(status, output)

    def test_dispatch_matches_legacy_entry(self) -> None:
        with self.make_client() as client:
            def legacy(isin: str) -> bytes:
                encoded = isin.encode("utf-8")
                output = _Buffer()
                status = client._native.lib.justetf_profile(
                    client._handle, encoded, len(encoded), ctypes.byref(output)
                )
                return client._native.consume(status, output)

            expected = json.loads(legacy("IE00B5BMR087"))
            actual = client.get_profile("IE00B5BMR087")
            self.assertEqual(actual, expected)
            for isin, exception_type in (("../invalid", ValueError), ("IE00B4L5Y983", APIError)):
                with self.subTest(isin=isin):
                    with self.assertRaises(exception_type) as direct:
                        legacy(isin)
                    with self.assertRaises(exception_type) as dispatched:
                        client.get_profile(isin)
                    self.assertEqual(str(direct.exception), str(dispatched.exception))
                    self.assertEqual(
                        getattr(direct.exception, "status_code", None),
                        getattr(dispatched.exception, "status_code", None),
                    )

    def test_empty_result_mapping(self) -> None:
        with self.make_client() as client:
            raw = client._native.consume(0, _Buffer())
            self.assertEqual(raw, b"")
            self.assertIsNone(_decode_result(raw))
            self.assertIsNone(_decode_result(b"null"))
            self.assertEqual(_decode_result(b"[]"), [])
            with self.assertRaises(json.JSONDecodeError):
                _decode_result(b" ")

    def test_native_buffers_freed_on_success_and_failure(self) -> None:
        with self.make_client() as client:
            native = client._native
            with patch.object(native.lib, "justetf_buffer_free", wraps=native.lib.justetf_buffer_free) as free:
                for _ in range(8):
                    client.get_profile("IE00B5BMR087")
                    with self.assertRaises(APIError):
                        client.get_profile("IE00B4L5Y983")
                self.assertEqual(free.call_count, 16)
                with patch("justetf_mobile_client.ctypes.string_at", side_effect=MemoryError):
                    with self.assertRaises(MemoryError):
                        client.get_profile_json("IE00B5BMR087")
                self.assertEqual(free.call_count, 17)

    def test_discovery_round_trips(self) -> None:
        market = {"currency": ["EUR"], "universeCountry": ["IT"], "universeType": ["PRIVATE"]}
        with self.make_client() as client:
            for name, options, expected_body, query in (
                ("get_universe_countries", None, {}, {}),
                ("get_account_access", None, {}, {"universeCountry": ["IT"]}),
                ("get_search_metadata", SearchMetadataOptions(), {}, {**market, "returnPeriodEnd": ["YESTERDAY"], "appVersion": ["4.4.2"]}),
                ("search_etfs", SearchOptions(), [], {**market, "returnPeriodEnd": ["YESTERDAY"], "returnPeriod": ["Y1"], "sortBy": ["fundSize"], "sortDir": ["DESC"], "limit": ["20"], "page": ["0"]}),
                ("search_isins", SearchISINsOptions(filters=["one", "two"]), ["one", "two"], {**market, "returnPeriodEnd": ["YESTERDAY"], "returnPeriod": ["Y1"], "sorting": ["LARGEST"]}),
                ("get_cards", CardsOptions(isins=["IE00B5BMR087"]), ["IE00B5BMR087"], {**market, "returnPeriodEnd": ["YESTERDAY"], "returnsPeriod": ["Y1"], "sorting": ["BEST"], "limit": ["30"], "page": ["0"]}),
                ("compare_etfs", ComparisonOptions(), [], market),
                ("search_holdings", HoldingSearchOptions(), [], {"universeCountry": ["IT"], "universeType": ["PRIVATE"], "query": [""], "sortField": ["name"], "sortOrder": ["ASC"], "limit": ["15"], "offset": ["0"]}),
            ):
                route = DISCOVERY_ROUTES[name]
                for raw in (False, True):
                    with self.subTest(operation=name, raw=raw):
                        method = getattr(client, name + ("_json" if raw else ""))
                        result = method() if options is None else method(options)
                        self.assertEqual(json.loads(result) if raw else result, self.server.contracts[route])
                        request = self.server.requests.get(timeout=2)
                        self.assertEqual((request["method"], request["path"]), route)
                        self.assertEqual(request["query"], query)
                        self.assertEqual(request["body"], expected_body)
                        self.assertEqual(request["language"], "it")
                        self.assertIsNone(request["authorization"])

    def test_discovery_option_overrides(self) -> None:
        with Client(self.library, api_key="test", base_url=self.base_url, country="DE", currency="USD", language="fr") as client:
            client.search_etfs(SearchOptions(country="FR", currency="GBP", query="ETF & \u6307\u6570", filters=["one", "two"], return_period="M6", return_period_end="TODAY", sort_by="name", sort_dir="ASC", limit=7, page=2))
            request = self.server.requests.get(timeout=2)
            self.assertEqual(request["query"], {
                "universeCountry": ["FR"], "currency": ["GBP"], "universeType": ["PRIVATE"],
                "query": ["ETF & \u6307\u6570"], "returnPeriod": ["M6"], "returnPeriodEnd": ["TODAY"],
                "sortBy": ["name"], "sortDir": ["ASC"], "limit": ["7"], "page": ["2"],
            })
            self.assertEqual(request["body"], ["one", "two"])
            self.assertEqual(request["language"], "fr")
            client.get_account_access("AT")
            self.assertEqual(self.server.requests.get(timeout=2)["query"], {"universeCountry": ["AT"]})

    def test_discovery_validation(self) -> None:
        before = self.server.requests.qsize()
        with self.make_client() as client:
            for call in (
                lambda: client.search_etfs(SearchOptions(limit=0)),
                lambda: client.search_etfs(SearchOptions(limit=False)),
                lambda: client.search_etfs(SearchOptions(page=-1)),
                lambda: client.get_cards(CardsOptions(isins=["invalid"])),
                lambda: client.search_holdings(HoldingSearchOptions(offset=-1)),
                lambda: client.compare_etfs(ComparisonOptions(isins=["invalid"])),
                lambda: client._call_json("search_holdings", {"filters": ["unverified"]}),
            ):
                with self.assertRaises(ValueError):
                    call()
            with self.assertRaises(TypeError):
                client.search_etfs(CardsOptions())
            with self.assertRaises(TypeError):
                client.get_account_access(123)
        self.assertEqual(self.server.requests.qsize(), before)

    def test_discovery_concurrent_calls(self) -> None:
        with self.make_client() as client, ThreadPoolExecutor(max_workers=8) as executor:
            results = list(executor.map(lambda _: client.search_etfs(), range(24)))
            expected = self.server.contracts[DISCOVERY_ROUTES["search_etfs"]]
            self.assertTrue(all(result == expected for result in results))
        for _ in range(24):
            request = self.server.requests.get(timeout=2)
            self.assertEqual(request["body"], [])

    def test_financial_round_trips(self) -> None:
        with self.make_client() as client:
            for name, (path, query) in FINANCIAL_ROUTES.items():
                route = ("GET", path.replace("{isin}", "IE00B5BMR087"))
                for raw in (False, True):
                    with self.subTest(operation=name, raw=raw):
                        method = getattr(client, name + ("_json" if raw else ""))
                        result = method("IE00B5BMR087") if "{isin}" in path else method()
                        self.assertEqual(json.loads(result) if raw else result, self.server.contracts[route])
                        request = self.server.requests.get(timeout=2)
                        self.assertEqual((request["method"], request["path"]), route)
                        self.assertEqual(request["query"], parse_qs(query, keep_blank_values=True))
                        self.assertEqual(request["body"], {})
                        self.assertIsNone(request["authorization"])
                        if name == "get_performance_chart":
                            self.assertIn("dateFrom", request["raw_query"].split("&"))
                            self.assertIn("dateTo", request["raw_query"].split("&"))

    def test_financial_overrides_and_precision(self) -> None:
        isin = "IE00B5BMR087"
        with self.make_client() as client:
            client.get_performance_chart(isin, ChartOptions(currency="USD", date_from="2026-01-01", date_to="2026-10-01", period="", reduce_data=False, include_dividends=False, values_type="MARKET_VALUE"))
            request = self.server.requests.get(timeout=2)
            self.assertEqual(request["query"], {"currency":["USD"],"dateFrom":["2026-01-01"],"dateTo":["2026-10-01"],"period":[""],"reduceData":["false"],"includeDividends":["false"],"valuesType":["MARKET_VALUE"]})
            self.assertIn("period", request["raw_query"].split("&"))
            client.get_performance_chart(isin, ChartOptions(date_from=""))
            self.assertIn("dateFrom=", self.server.requests.get(timeout=2)["raw_query"].split("&"))
            client.get_country_exposure(isin, ExposureOptions(top=8, scale=0, threshold="0.000000000000000001"))
            self.assertEqual(self.server.requests.get(timeout=2)["query"], {"top":["8"],"scale":["0"],"threshold":["0.000000000000000001"]})
            client.get_savings_plans(isin, SavingsPlansOptions(savings_rate="100.000000000000000001"))
            self.assertEqual(self.server.requests.get(timeout=2)["query"]["savingsRate"], ["100.000000000000000001"])
            client.get_order_fees(isin, OrderFeesOptions(order_volume="200.500000000000000001"))
            self.assertEqual(self.server.requests.get(timeout=2)["query"]["orderVolume"], ["200.500000000000000001"])
            client.get_holding_etfs(isin, HoldingETFsOptions(country="DE", top=5, page=2, scale=0))
            self.assertEqual(self.server.requests.get(timeout=2)["query"], {"universeCountry":["DE"],"top":["5"],"page":["2"],"scale":["0"]})
            client.get_market_overview(MarketOverviewOptions(currency="USD", home_screen_only=True, returns_period="Y1"))
            self.assertEqual(self.server.requests.get(timeout=2)["query"]["homeScreenOnly"], ["true"])
            profile = client.get_profile(isin, MarketOptions(currency="USD", country="FR"))
            self.assertEqual(profile["query"]["currency"], ["USD"])
            self.assertEqual(profile["query"]["universeCountry"], ["FR"])
            quote_route = ("GET", FINANCIAL_ROUTES["get_quote"][0].replace("{isin}", isin))
            payload = b'{"price":1.234567890123456789,"nullable":null,"futureField":[]}'
            self.server.raw_responses[quote_route] = payload
            try:
                raw = client.get_quote_json(isin)
                self.assertEqual(raw, payload)
                self.assertEqual(json.loads(raw, parse_float=Decimal)["price"], Decimal("1.234567890123456789"))
                self.assertIsNone(json.loads(raw)["nullable"])
                self.server.requests.get(timeout=2)
            finally:
                del self.server.raw_responses[quote_route]

    def test_financial_validation_and_unavailable_chart(self) -> None:
        isin = "IE00B5BMR087"
        before = self.server.requests.qsize()
        with self.make_client() as client:
            for call in (
                lambda: client.get_quote("../invalid"),
                lambda: client.get_performance_chart(isin, ChartOptions(date_from="2026-02-30")),
                lambda: client.get_performance_chart(isin, ChartOptions(date_from="2026-10-02", date_to="2026-10-01")),
                lambda: client.get_country_exposure(isin, ExposureOptions(top=0)),
                lambda: client.get_sector_exposure(isin, ExposureOptions(scale=-1)),
                lambda: client.get_holding_exposure(isin, ExposureOptions(threshold="NaN")),
                lambda: client.get_holding_etfs(isin, HoldingETFsOptions(page=-1)),
                lambda: client.get_savings_plans(isin, SavingsPlansOptions(savings_rate="0")),
                lambda: client.get_order_fees(isin, OrderFeesOptions(order_volume="-1")),
                lambda: client.get_order_fees(isin, OrderFeesOptions(order_volume=100)),
                lambda: client.get_index_performance(IndexPerformanceOptions(limit=0)),
            ):
                with self.assertRaises(ValueError):
                    call()
            self.assertEqual(self.server.requests.qsize(), before)
            with self.assertRaises(APIError) as caught:
                client.get_performance_chart(isin, ChartOptions(period="MISSING"))
            self.assertEqual(caught.exception.status_code, 404)
            self.assertNotIn("private", str(caught.exception))
            self.server.requests.get(timeout=2)

    def test_financial_concurrent_calls(self) -> None:
        with self.make_client() as client, ThreadPoolExecutor(max_workers=8) as executor:
            results = list(executor.map(lambda _: client.get_quote("IE00B5BMR087"), range(24)))
            route = ("GET", FINANCIAL_ROUTES["get_quote"][0].replace("{isin}", "IE00B5BMR087"))
            self.assertTrue(all(result == self.server.contracts[route] for result in results))
        for _ in range(24):
            self.assertEqual(self.server.requests.get(timeout=2)["body"], {})

    def test_authentication_lifecycle(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            self.assertFalse(client.is_authenticated())
            self.assertIsNone(client.login("test@example.invalid", "test:password"))
            self.assertTrue(client.is_authenticated())
            request = self.server.auth_requests.get(timeout=2)
            self.assertEqual(request, {"path": "/v1/auth/tokens", "scheme": "Basic", "body": b"{}"})
            self.assertIsNone(client.refresh())
            self.assertIsNone(client.refresh())
            self.assertEqual(self.server.auth_refreshes, 2)
            for _ in range(2):
                self.assertEqual(self.server.auth_requests.get(timeout=2), {"path": "/v1/auth/refreshed-tokens", "scheme": "Bearer", "body": b"{}"})
            client.get_quote("IE00B5BMR087")
            self.assertIsNone(self.server.requests.get(timeout=2)["authorization"])
            self.assertNotIn("test:password", repr(client.__dict__))
            self.assertIsNone(client.logout())
            self.assertIsNone(client.logout())
            self.assertFalse(client.is_authenticated())
            with self.assertRaises(AuthenticationError):
                client.refresh()
        with self.assertRaises(ClientClosedError):
            client.is_authenticated()

    def test_authentication_errors_and_privacy(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            with self.assertRaises(AuthenticationError):
                client.refresh()
            with self.assertRaises(TypeError):
                client.login(123, "private-password")
            with self.assertRaises(ValueError) as invalid:
                client.login("", "private-password")
            self.assertNotIn("private", str(invalid.exception))
            with self.assertRaises(APIError) as rejected:
                client.login("test@example.invalid", "private-wrong-password")
            self.assertEqual(rejected.exception.status_code, 401)
            self.assertNotIn("private", str(rejected.exception))
            self.assertFalse(client.is_authenticated())
            self.server.auth_malformed = True
            try:
                with self.assertRaises(TransportError) as malformed:
                    client.login("test@example.invalid", "test:password")
                self.assertNotIn("private", str(malformed.exception))
            finally:
                self.server.auth_malformed = False
            client.login("test@example.invalid", "test:password")
            self.server.auth_fail_refresh = True
            try:
                with self.assertRaises(APIError) as expired:
                    client.refresh()
                self.assertEqual(expired.exception.status_code, 401)
                self.assertFalse(client.is_authenticated())
                with self.assertRaises(AuthenticationError):
                    client.refresh()
                self.assertEqual(self.server.auth_refreshes, 1)
            finally:
                self.server.auth_fail_refresh = False

    def test_authentication_close_during_refresh(self) -> None:
        self.server.reset_authentication()
        try:
            with self.make_client() as client, ThreadPoolExecutor(max_workers=1) as executor:
                client.login("test@example.invalid", "test:password")
                self.server.auth_release.clear()
                result = executor.submit(client.refresh)
                self.assertTrue(self.server.auth_started.wait(timeout=2))
                client.close()
                with self.assertRaises(ClientClosedError):
                    result.result(timeout=2)
        finally:
            self.server.auth_release.set()

    def test_authentication_client_isolation(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as first, self.make_client() as second:
            first.login("test@example.invalid", "test:password")
            self.assertTrue(first.is_authenticated())
            self.assertFalse(second.is_authenticated())
            second.login("test@example.invalid", "test:password")
            first.logout()
            self.assertTrue(second.is_authenticated())

    def test_account_round_trips(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            for name, (_, _, arguments) in ACCOUNT_ROUTES.items():
                with self.subTest(anonymous=name), self.assertRaises(AuthenticationError):
                    getattr(client, name)(*arguments)
            self.assertTrue(self.server.account_requests.empty())
            client.login("test@example.invalid", "test:password")
            for name, (template, query, arguments) in ACCOUNT_ROUTES.items():
                route = ("GET", account_path(template))
                for raw in (False, True):
                    with self.subTest(operation=name, raw=raw):
                        result = getattr(client, name + ("_json" if raw else ""))(*arguments)
                        self.assertEqual(json.loads(result) if raw else result, self.server.account_contracts[route])
                        request = self.server.account_requests.get(timeout=2)
                        self.assertEqual((request["method"], request["path"]), route)
                        self.assertEqual(request["query"], parse_qs(query, keep_blank_values=True))
                        self.assertEqual(request["body"], {})
                        self.assertTrue(request["authenticated"])
                        if name == "get_portfolio_performance_chart":
                            self.assertIn("dateFrom", request["raw_query"].split("&"))
                            self.assertIn("dateTo", request["raw_query"].split("&"))
                            self.assertNotIn("includeDividends", request["query"])

    def test_account_overrides_and_precision(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            client.login("test@example.invalid", "test:password")
            client.get_personal_data("FR")
            self.assertEqual(self.server.account_requests.get(timeout=2)["query"], {"universeCountry":["FR"]})
            client.get_watchlists(WatchlistOptions(currency="USD", return_period_end="TODAY", returns_period="M6"))
            self.assertEqual(self.server.account_requests.get(timeout=2)["query"], {"currency":["USD"],"returnPeriodEnd":["TODAY"],"returnsPeriod":["M6"]})
            client.get_portfolios(PortfolioListOptions(limit=7, page=2, period="Y1", app_version="5.0"))
            self.assertEqual(self.server.account_requests.get(timeout=2)["query"], {"period":["Y1"],"appVersion":["5.0"],"limit":["7"],"page":["2"]})
            client.get_portfolio_transactions("123456", PageOptions(limit=7, page=2))
            self.assertEqual(self.server.account_requests.get(timeout=2)["query"], {"limit":["7"],"page":["2"]})
            client.get_portfolio_allocation("123456", AllocationOptions(grouping="SECTORS", limit=3, offset=2))
            self.assertEqual(self.server.account_requests.get(timeout=2)["query"], {"grouping":["SECTORS"],"limit":["3"],"offset":["2"]})
            client.get_portfolio_performance_chart("123456", PortfolioChartOptions(currency="USD", date_from="2026-01-01", date_to="2026-10-01", period="", reduce_data=False))
            request = self.server.account_requests.get(timeout=2)
            self.assertEqual(request["query"], {"currency":["USD"],"dateFrom":["2026-01-01"],"dateTo":["2026-10-01"],"period":[""],"reduceData":["false"],"valuesType":["RELATIVE_CHANGE"]})
            self.assertIn("period", request["raw_query"].split("&"))
            client.get_trade_quote("IE00B5BMR087", TradeQuoteOptions(currency="USD", date="2026-10-14"))
            self.assertEqual(self.server.account_requests.get(timeout=2)["query"], {"isin":["IE00B5BMR087"],"currency":["USD"],"date":["2026-10-14"]})
            route = ("GET", account_path(ACCOUNT_ROUTES["get_watchlists"][0]))
            payload = b'{"watchlists":[],"value":1.234567890123456789,"date":"2026-10-14","nullable":null,"futureField":[]}'
            self.server.account_raw_responses[route] = payload
            try:
                raw = client.get_watchlists_json()
                self.assertEqual(raw, payload)
                decoded = json.loads(raw, parse_float=Decimal)
                self.assertEqual(decoded["value"], Decimal("1.234567890123456789"))
                self.assertEqual(decoded["watchlists"], [])
                self.assertIsNone(decoded["nullable"])
                self.server.account_requests.get(timeout=2)
            finally:
                del self.server.account_raw_responses[route]

    def test_account_parallel_expiry_refresh(self) -> None:
        self.server.reset_authentication()
        try:
            with self.make_client() as client, ThreadPoolExecutor(max_workers=8) as executor:
                client.login("test@example.invalid", "test:password")
                self.server.account_expire_old = True
                self.server.account_expected_expired = 8
                self.server.auth_release.clear()
                results = list(executor.map(lambda _: client.get_watchlists(), range(8)))
                route = ("GET", account_path(ACCOUNT_ROUTES["get_watchlists"][0]))
                self.assertTrue(all(result == self.server.account_contracts[route] for result in results))
                self.assertEqual(self.server.auth_refreshes, 1)
                statuses = [self.server.account_requests.get(timeout=2)["status"] for _ in range(16)]
                self.assertEqual(statuses.count(401), 8)
                self.assertEqual(statuses.count(200), 8)
        finally:
            self.server.auth_release.set()

    def test_account_validation_and_forbidden(self) -> None:
        self.server.reset_authentication()
        with self.make_client() as client:
            client.login("test@example.invalid", "test:password")
            for call in (
                lambda: client.get_portfolio("../private"),
                lambda: client.get_portfolio("can-create"),
                lambda: client.get_portfolios(PortfolioListOptions(limit=0)),
                lambda: client.get_portfolio_transactions("123456", PageOptions(page=-1)),
                lambda: client.get_portfolio_allocation("123456", AllocationOptions(offset=-1)),
                lambda: client.get_trade_quote("IE00B5BMR087"),
                lambda: client.get_trade_quote("IE00B5BMR087", TradeQuoteOptions(date="2026-02-30")),
                lambda: client.get_price_alert("IE00B5BMR087", "../private"),
                lambda: client.get_holding_supplementary_data("invalid"),
                lambda: client._call_json("get_portfolio_performance_chart", {"portfolio_id":"123456","include_dividends":True}),
            ):
                with self.assertRaises(ValueError):
                    call()
            with self.assertRaises(TypeError):
                client.get_portfolio(123456)
            self.assertTrue(self.server.account_requests.empty())
            self.server.account_status = 403
            try:
                with self.assertRaises(APIError) as caught:
                    client.get_watchlists()
                self.assertEqual(caught.exception.status_code, 403)
                self.assertNotIn("private", str(caught.exception))
                self.assertEqual(self.server.auth_refreshes, 0)
                self.server.account_requests.get(timeout=2)
            finally:
                self.server.account_status = 200


if __name__ == "__main__":
    unittest.main()
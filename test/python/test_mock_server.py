import json
import os
import threading
import unittest
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from mock_server import ACCOUNT_ROUTES, DISCOVERY_ROUTES, FINANCIAL_ROUTES, MUTATION_ROUTES, ContractHandler, ProfileServer


@unittest.skipIf(os.environ.get("JUSTETF_TEST_MODE", "offline") != "offline", "controlled server tests run offline only")
class MockServerTests(unittest.TestCase):
    def setUp(self):
        self.server = ProfileServer(("127.0.0.1", 0), ContractHandler)
        self.addCleanup(self.server.server_close)
        worker = threading.Thread(target=self.server.serve_forever, daemon=True)
        worker.start()
        self.addCleanup(worker.join)
        self.addCleanup(self.server.shutdown)
        self.base = f"http://127.0.0.1:{self.server.server_port}"

    def request(self, method, path, body=None, authenticated=True):
        headers = {"Content-Type": "application/json", "x-api-key": "synthetic-test-key"}
        if authenticated:
            headers["Authorization"] = "Bearer test-access-0"
        request = Request(self.base + path, json.dumps({} if body is None else body).encode(), headers, method=method)
        try:
            response = urlopen(request, timeout=3)
        except HTTPError as error:
            status = error.code
            error.close()
            return status, None
        with response:
            raw = response.read()
            return response.status, json.loads(raw) if raw else None

    def test_catalogue_covers_supported_routes(self):
        cases = self.server.catalog
        self.assertEqual(len({case["name"] for case in cases}), 46)
        routes = {(case["method"], case["path"]) for case in cases}
        expected = set(DISCOVERY_ROUTES.values()) | set(MUTATION_ROUTES.values())
        expected.update(("GET", path) for path, _ in FINANCIAL_ROUTES.values())
        expected.update(("GET", path) for path, _, _ in ACCOUNT_ROUTES.values())
        self.assertTrue(expected <= routes)
        self.assertTrue(all("response" in case or case.get("empty") for case in cases))

    def test_unknown_routes_and_unauthenticated_account_fail_closed(self):
        self.assertEqual(self.request("GET", "/private-unknown")[0], 404)
        self.assertEqual(self.request("GET", "/v1/exchange-traded-funds/portfolios", authenticated=False)[0], 401)
        self.assertEqual(self.request("POST", "/v4/exchange-traded-funds/profiles/search", {})[0], 400)

    def test_owned_portfolio_delete_preserves_seed(self):
        path = "/v1/exchange-traded-funds/portfolios"
        status, created = self.request("POST", path, {"name": "sdk-test-owned", "currency": "EUR", "investment": "ONE_TIME"})
        self.assertEqual(status, 201)
        identifier = str(created["id"])
        self.assertEqual(self.request("DELETE", path + "/" + identifier)[0], 204)
        self.assertEqual(self.request("GET", path + "/" + identifier)[0], 404)
        self.assertEqual(set(self.server.portfolios), {"123456"})
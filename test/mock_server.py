import base64
import json
import threading
import copy
import re
import uuid
from decimal import Decimal
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from queue import Queue
from urllib.parse import parse_qs, urlsplit

from justetf_mobile_client import TradeQuoteOptions

DISCOVERY_ROUTES = {
    "get_universe_countries": ("GET", "/v3/exchange-traded-funds/users/metadata/universe-countries"),
    "get_account_access": ("GET", "/v1/exchange-traded-funds/users/metadata/account-access"),
    "get_search_metadata": ("GET", "/v4/exchange-traded-funds/profiles/search/metadata"),
    "search_etfs": ("POST", "/v4/exchange-traded-funds/profiles/search"),
    "search_isins": ("POST", "/v2/exchange-traded-funds/profiles/search-isins"),
    "get_cards": ("POST", "/v2/exchange-traded-funds/profiles/cards"),
    "compare_etfs": ("POST", "/v3/exchange-traded-funds/profiles/comparison"),
    "search_holdings": ("POST", "/v3/exchange-traded-funds/profiles/exposures/holdings/search"),
}

FINANCIAL_ROUTES = {
    "get_quote": ("/v1/exchange-traded-funds/profiles/{isin}/quote", "currency=EUR"),
    "get_performance_chart": ("/v1/exchange-traded-funds/profiles/{isin}/performance-chart", "currency=EUR&dateFrom&dateTo&period=Y1&reduceData=true&includeDividends=true&valuesType=RELATIVE_CHANGE"),
    "get_returns": ("/v2/exchange-traded-funds/profiles/{isin}/returns", "currency=EUR&returnPeriodEnd=YESTERDAY"),
    "get_risks": ("/v2/exchange-traded-funds/profiles/{isin}/risks", "currency=EUR&returnPeriodEnd=YESTERDAY"),
    "get_country_exposure": ("/v1/exchange-traded-funds/profiles/{isin}/exposures/countries", "top=4&scale=4&threshold=0.01"),
    "get_sector_exposure": ("/v1/exchange-traded-funds/profiles/{isin}/exposures/sectors", "top=4&scale=4&threshold=0.01"),
    "get_holding_exposure": ("/v2/exchange-traded-funds/profiles/{isin}/exposures/holdings", "top=10&scale=4&threshold=0.01"),
    "get_holding_details": ("/v5/exchange-traded-funds/profiles/exposures/holdings/{isin}", "currency=EUR&universeCountry=IT&universeType=PRIVATE"),
    "get_holding_etfs": ("/v2/exchange-traded-funds/profiles/exposures/holdings/{isin}/etfs", "top=15&page=0&scale=2&universeCountry=IT"),
    "get_brokers": ("/v2/exchange-traded-funds/profiles/integrations/brokers", "universeCountry=IT&universeType=PRIVATE"),
    "get_market_overview": ("/v2/exchange-traded-funds/profiles/integrations/indexes/market-overview-indexes", "currency=EUR&universeCountry=IT&universeType=PRIVATE&returnPeriodEnd=YESTERDAY&returnsPeriod=D1&homeScreenOnly=false"),
    "get_index_performance": ("/v2/exchange-traded-funds/profiles/integrations/indexes/performance", "currency=EUR&universeCountry=IT&universeType=PRIVATE&performanceType=TOP&performanceValue=RETURN_Y1&performanceCategory=ALL&limit=50"),
    "get_recommended_filters": ("/v1/exchange-traded-funds/profiles/integrations/recommended-filters/search", "universeCountry=IT&universeType=PRIVATE&appVersion=4.4.2"),
    "get_savings_plans": ("/v4/exchange-traded-funds/profiles/{isin}/savings-plans", "universeCountry=IT&universeType=PRIVATE&savingsRate=100&sorting=RATING"),
    "get_order_fees": ("/v3/exchange-traded-funds/profiles/{isin}/order-fees", "universeCountry=IT&universeType=PRIVATE&orderVolume=100&sorting=BEST"),
}

ACCOUNT_ROUTES = {
    "get_personal_data": ("/v1/exchange-traded-funds/users/profiles/personal-data", "universeCountry=IT", ()),
    "get_watchlists": ("/v2/exchange-traded-funds/profiles/watchlists", "currency=EUR&returnPeriodEnd=YESTERDAY&returnsPeriod=Y1", ()),
    "get_saved_filters": ("/v1/exchange-traded-funds/profiles/integrations/saved-filters", "", ()),
    "get_price_alerts": ("/v1/price-alerts/etfs/alerts", "", ()),
    "get_etf_price_alerts": ("/v1/price-alerts/etfs/{isin}/alerts", "", ("IE00B5BMR087",)),
    "get_price_alert": ("/v1/price-alerts/etfs/{isin}/alerts/{id}", "", ("IE00B5BMR087", "11111111-2222-4333-8444-555555555555")),
    "get_portfolios": ("/v1/exchange-traded-funds/portfolios", "period=MAX&limit=10&page=0&appVersion=4.4.2", ()),
    "get_portfolio": ("/v1/exchange-traded-funds/portfolios/{id}", "period=MAX&appVersion=4.4.2", ("123456",)),
    "get_portfolio_composition": ("/v1/exchange-traded-funds/portfolios/{id}/composition", "period=MAX", ("123456",)),
    "can_create_portfolio": ("/v1/exchange-traded-funds/portfolios/can-create", "", ()),
    "get_trade_quote": ("/v1/exchange-traded-funds/portfolios/trade-quote", "isin=IE00B5BMR087&currency=EUR&date=2026-10-14", ("IE00B5BMR087", TradeQuoteOptions(date="2026-10-14"))),
    "get_portfolio_transactions": ("/v1/exchange-traded-funds/profiles/integrations/portfolios/{id}/transactions", "limit=10&page=0", ("123456",)),
    "get_portfolio_allocation": ("/v1/exchange-traded-funds/profiles/integrations/portfolios/{id}/allocation/x-ray", "grouping=COUNTRIES&limit=5&offset=0", ("123456",)),
    "get_portfolio_performance_chart": ("/v2/exchange-traded-funds/profiles/integrations/portfolios/{id}/performance-chart", "currency=EUR&dateFrom&dateTo&period=MAX&reduceData=true&valuesType=RELATIVE_CHANGE", ("123456",)),
    "get_holding_supplementary_data": ("/v1/exchange-traded-funds/profiles/exposures/holdings/{isin}/supplementary-data", "", ("IE00B5BMR087",)),
}

MUTATION_ROUTES = {
    "create_price_alert": ("POST", "/v1/price-alerts/etfs/{isin}/alerts"),
    "delete_price_alert": ("DELETE", "/v1/price-alerts/etfs/{isin}/alerts/{id}"),
    "create_saved_filter": ("POST", "/v1/exchange-traded-funds/profiles/integrations/saved-filters"),
    "delete_saved_filter": ("DELETE", "/v1/exchange-traded-funds/profiles/integrations/saved-filters/{id}"),
    "create_portfolio": ("POST", "/v1/exchange-traded-funds/portfolios"),
    "delete_portfolio": ("DELETE", "/v1/exchange-traded-funds/portfolios/{id}"),
    "add_share_transaction": ("POST", "/v1/exchange-traded-funds/portfolios/{id}/transactions/share"),
}


def account_path(template: str) -> str:
    identifier = "11111111-2222-4333-8444-555555555555" if template.startswith("/v1/price-alerts/") or "/saved-filters/" in template else "123456"
    return template.replace("{isin}", "IE00B5BMR087").replace("{id}", identifier)


class ProfileHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server: "ProfileServer"

    def handle(self) -> None:
        try:
            super().handle()
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass

    def do_GET(self) -> None:
        request_body = self.rfile.read(int(self.headers.get("content-length", "0")))
        if self.respond_authentication(request_body):
            return
        if self.respond_account(request_body):
            return
        if self.respond_discovery(request_body):
            return
        parsed = urlsplit(self.path)
        if parsed.path.endswith("/IE00BK5BQT80"):
            self.server.started.set()
            self.server.release.wait(timeout=3)
        if parsed.path.endswith("/IE00BKM4GZ66"):
            self.server.overlap.wait(timeout=3)
        if parsed.path.endswith("/IE00B4L5Y983"):
            body = b'{"private":"must not appear in errors"}'
            self.send_response(401)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        body = json.dumps(
            {
                "isin": parsed.path.rsplit("/", 1)[-1],
                "name": "Example ETF",
                "query": parse_qs(parsed.query),
                "api_key": self.headers.get("x-api-key"),
                "language": self.headers.get("accept-language"),
                "user_agent": self.headers.get("user-agent"),
                "request_body": request_body.decode("utf-8"),
                "peer_port": self.client_address[1],
            }
        ).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass

    def do_POST(self) -> None:
        request_body = self.rfile.read(int(self.headers.get("content-length", "0")))
        if self.respond_mutation(request_body):
            return
        if not self.respond_discovery(request_body):
            self.send_error(404)

    def do_DELETE(self) -> None:
        request_body = self.rfile.read(int(self.headers.get("content-length", "0")))
        if not self.respond_mutation(request_body):
            self.send_error(404)

    def respond_mutation(self, request_body: bytes) -> bool:
        parsed = urlsplit(self.path)
        route = (self.command, parsed.path)
        if route not in self.server.mutation_contracts:
            return False
        with self.server.auth_lock:
            authenticated = self.headers.get("authorization") == f"Bearer test-access-{self.server.auth_generation}"
            status = self.server.mutation_status if authenticated else 401
            drop = self.server.mutation_drop
        self.server.mutation_requests.put({
            "method": self.command, "path": parsed.path, "query": parsed.query,
            "body": json.loads(request_body, parse_float=Decimal), "authenticated": authenticated,
        })
        if drop:
            self.close_connection = True
            return True
        fixture = self.server.mutation_contracts[route]
        if status is None:
            status = fixture["status"]
        body = json.dumps(fixture["response_body"]).encode("utf-8") if "response_body" in fixture else b""
        if status >= 400:
            body = b'{"message":"private mutation error"}'
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        return True

    def respond_discovery(self, request_body: bytes) -> bool:
        parsed = urlsplit(self.path)
        route = (self.command, parsed.path)
        if route not in self.server.contracts:
            return False
        self.server.requests.put({
            "method": self.command, "path": parsed.path,
            "query": parse_qs(parsed.query, keep_blank_values=True),
            "raw_query": parsed.query,
            "body": json.loads(request_body),
            "language": self.headers.get("accept-language"),
            "authorization": self.headers.get("authorization"),
        })
        status = 200
        body = self.server.raw_responses.get(route)
        if body is None:
            body = json.dumps(self.server.contracts[route]).encode("utf-8")
        if parsed.path.endswith("/performance-chart") and parse_qs(parsed.query).get("period") == ["MISSING"]:
            status = 404
            body = b'{"message":"private upstream detail"}'
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        return True

    def respond_authentication(self, request_body: bytes) -> bool:
        path = urlsplit(self.path).path
        if path not in ("/v1/auth/tokens", "/v1/auth/refreshed-tokens"):
            return False
        authorization = self.headers.get("authorization", "")
        self.server.auth_requests.put({"path": path, "scheme": authorization.split(" ")[0], "body": request_body})
        status = 200
        with self.server.auth_lock:
            if path == "/v1/auth/tokens":
                expected = "Basic " + base64.b64encode(b"test@example.invalid:test:password").decode("ascii")
                if authorization != expected:
                    status = 401
                else:
                    self.server.auth_generation = 0
            else:
                self.server.auth_refreshes += 1
                if self.server.auth_fail_refresh or authorization != f"Bearer test-refresh-{self.server.auth_generation}":
                    status = 401
                else:
                    self.server.auth_generation += 1
            response = {"accessToken": f"test-access-{self.server.auth_generation}", "refreshToken": f"test-refresh-{self.server.auth_generation}"}
            if self.server.auth_malformed:
                response = {"accessToken": "private-malformed-access"}
            if status != 200:
                response = {"message": "private authentication failure"}
        if path == "/v1/auth/refreshed-tokens":
            self.server.auth_started.set()
            self.server.auth_release.wait(timeout=3)
        body = json.dumps(response).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        return True

    def respond_account(self, request_body: bytes) -> bool:
        parsed = urlsplit(self.path)
        route = (self.command, parsed.path)
        if route not in self.server.account_contracts:
            return False
        authorization = self.headers.get("authorization", "")
        with self.server.auth_lock:
            authenticated = authorization == f"Bearer test-access-{self.server.auth_generation}"
            status = self.server.account_status if authenticated else 401
            if self.server.account_expire_old and authorization == "Bearer test-access-0":
                status = 401
                self.server.account_expired += 1
                if self.server.account_expired == self.server.account_expected_expired:
                    self.server.auth_release.set()
        self.server.account_requests.put({
            "method": self.command, "path": parsed.path,
            "query": parse_qs(parsed.query, keep_blank_values=True), "raw_query": parsed.query,
            "body": json.loads(request_body), "authenticated": authenticated, "status": status,
        })
        body = self.server.account_raw_responses.get(route)
        if body is None:
            body = json.dumps(self.server.account_contracts[route]).encode("utf-8")
        if status != 200:
            body = b'{"message":"private account error"}'
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        return True

    def log_message(self, format: str, *args: object) -> None:
        pass


class ProfileServer(ThreadingHTTPServer):
    request_queue_size = 64

    def __init__(self, address: tuple[str, int], handler: type[BaseHTTPRequestHandler]) -> None:
        super().__init__(address, handler)
        self.started = threading.Event()
        self.release = threading.Event()
        self.overlap = threading.Barrier(2)
        self.catalog = json.loads(Path(__file__).with_name("contracts.json").read_text(encoding="utf-8"))
        inventory = {"operations": [
            {"method": case["method"], "path": case["path"], "fixtures": [{
                "status": case.get("status", 200), "observed_auth": "Bearer" if case.get("auth") else "none",
                **({"response_body": case["response"]} if "response" in case else {}),
            }]}
            for case in self.catalog
        ]}
        self.contracts = {route: {"synthetic": True} for route in DISCOVERY_ROUTES.values()}
        self.contracts.update({
            ("GET", path.replace("{isin}", "IE00B5BMR087")): {"synthetic": True}
            for path, _ in FINANCIAL_ROUTES.values()
        })
        self.account_contracts = {
            ("GET", account_path(path)): {"synthetic": True}
            for path, _, _ in ACCOUNT_ROUTES.values()
        }
        self.mutation_contracts = {}
        for name, (method, template) in MUTATION_ROUTES.items():
            response = {"status": 200}
            if name in ("create_price_alert", "create_portfolio", "add_share_transaction"):
                response["response_body"] = {"synthetic": True}
            if name == "create_portfolio":
                response["status"] = 201
            elif name == "delete_portfolio":
                response["status"] = 204
            self.mutation_contracts[(method, account_path(template))] = response
        self.account_raw_responses: dict[tuple[str, str], bytes] = {}
        self.raw_responses: dict[tuple[str, str], bytes] = {}
        for operation in inventory["operations"]:
            route = (operation["method"], operation["path"])
            if route in MUTATION_ROUTES.values():
                self.mutation_contracts[(route[0], account_path(route[1]))] = next(
                    fixture for fixture in operation["fixtures"] if 200 <= fixture["status"] < 300
                )
            if route in DISCOVERY_ROUTES.values() or (route[0] == "GET" and route[1] in [path for path, _ in FINANCIAL_ROUTES.values()]):
                actual_route = (route[0], route[1].replace("{isin}", "IE00B5BMR087"))
                self.contracts[actual_route] = next(
                    fixture["response_body"] for fixture in operation["fixtures"] if fixture["status"] == 200
                )
            if route[0] == "GET" and route[1] in [path for path, _, _ in ACCOUNT_ROUTES.values()]:
                self.account_contracts[("GET", account_path(route[1]))] = next(
                    fixture["response_body"] for fixture in operation["fixtures"]
                    if fixture["status"] == 200 and fixture["observed_auth"] == "Bearer"
                )
        self.requests: Queue[dict[str, object]] = Queue()
        self.account_requests: Queue[dict[str, object]] = Queue()
        self.auth_requests: Queue[dict[str, object]] = Queue()
        self.mutation_requests: Queue[dict[str, object]] = Queue()
        self.auth_lock = threading.Lock()
        self.auth_started = threading.Event()
        self.auth_release = threading.Event()
        self.reset_authentication()
        self.resource_lock = threading.Lock()
        self.next_id = 200000
        seed = next(case["response"] for case in self.catalog if case["name"] == "get_portfolio")
        alert = next(case["response"] for case in self.catalog if case["name"] == "get_price_alert")
        self.portfolios = {str(seed["id"]): copy.deepcopy(seed)}
        self.alerts = {alert["id"]: {**copy.deepcopy(alert), "isin": "IE00B5BMR087"}}
        self.filters = {}
        self.transactions = {}


    def reset_authentication(self) -> None:
        with self.auth_lock:
            self.auth_generation = 0
            self.auth_refreshes = 0
            self.auth_fail_refresh = False
            self.auth_malformed = False
            self.account_status = 200
            self.account_expire_old = False
            self.account_expired = 0
            self.account_expected_expired = 0
            self.mutation_status = None
            self.mutation_drop = False
        self.auth_started.clear()
        self.auth_release.set()
        while not self.auth_requests.empty():
            self.auth_requests.get_nowait()
        while not self.account_requests.empty():
            self.account_requests.get_nowait()
        while not self.mutation_requests.empty():
            self.mutation_requests.get_nowait()


class ContractHandler(ProfileHandler):
    def do_GET(self):
        self.respond_contract()

    def do_POST(self):
        self.respond_contract()

    def do_DELETE(self):
        self.respond_contract()

    def send_json(self, status, body=None):
        encoded = json.dumps(body).encode("utf-8") if body is not None else b""
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def respond_contract(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if not self.headers.get("x-api-key") or self.headers.get("content-type") != "application/json":
            self.send_json(400, {"message": "missing protocol headers"})
            return
        if self.respond_authentication(raw):
            return
        try:
            body = json.loads(raw)
        except (ValueError, UnicodeDecodeError):
            self.send_json(400, {"message": "invalid request JSON"})
            return
        path = urlsplit(self.path).path
        for case in sorted(self.server.catalog, key=lambda item: item["path"].count("{")):
            if case["method"] != self.command:
                continue
            pattern = re.escape(case["path"]).replace(r"\{isin\}", r"(?P<isin>[^/]+)").replace(r"\{id\}", r"(?P<id>[^/]+)")
            match = re.fullmatch(pattern, path)
            if match is None:
                continue
            if case.get("auth") and self.headers.get("authorization") != f"Bearer test-access-{self.server.auth_generation}":
                self.send_json(401, {"message": "authentication required"})
                return
            if self.command in ("GET", "DELETE") and body != {}:
                self.send_json(400, {"message": "expected empty request object"})
                return
            if self.command == "POST" and not case.get("mutation") and not isinstance(body, list):
                self.send_json(400, {"message": "expected request array"})
                return
            with self.server.resource_lock:
                self.apply_contract(case, match.groupdict(), body)
            return
        self.send_json(404, {"message": "unknown test endpoint"})

    def apply_contract(self, case, identifiers, body):
        name = case["name"]
        identifier = identifiers.get("id")
        isin = identifiers.get("isin")
        response = copy.deepcopy(case.get("response"))
        status = case.get("status", 200)
        if name == "create_portfolio":
            self.server.next_id += 1
            response.update(body, id=self.server.next_id)
            self.server.portfolios[str(response["id"])] = response
        elif name == "create_price_alert":
            response.update(id=str(uuid.uuid4()), condition=body["condition"], comment=body.get("comment", ""))
            response["price"] = {"raw": body["price"], "localized": str(body["price"])}
            self.server.alerts[response["id"]] = {**response, "isin": isin}
        elif name == "create_saved_filter":
            identifier = str(uuid.uuid4())
            self.server.filters[identifier] = {"id": identifier, "label": body["label"], "filterData": body["savedFilterData"]}
        elif name in ("delete_portfolio", "delete_price_alert", "delete_saved_filter"):
            resources = self.server.portfolios if name == "delete_portfolio" else self.server.alerts if name == "delete_price_alert" else self.server.filters
            if resources.pop(identifier, None) is None:
                self.send_json(404, {"message": "test resource not found"})
                return
            if name == "delete_portfolio":
                self.server.transactions.pop(identifier, None)
        elif name == "get_portfolios":
            response = {"portfolios": list(self.server.portfolios.values()), "totalPortfoliosCount": len(self.server.portfolios)}
        elif name == "get_saved_filters":
            response = {"savedFilters": list(self.server.filters.values())}
        elif name == "get_etf_price_alerts":
            response = {"alerts": [alert for alert in self.server.alerts.values() if alert["isin"] == isin]}
        elif name == "get_price_alert":
            response = self.server.alerts.get(identifier)
            if response is None or response["isin"] != isin:
                self.send_json(404, {"message": "test resource not found"})
                return
        elif (identifier and "portfolio" in name) or name == "add_share_transaction":
            if identifier not in self.server.portfolios:
                self.send_json(404, {"message": "test resource not found"})
                return
            if name == "get_portfolio":
                response = self.server.portfolios[identifier]
            elif name == "add_share_transaction":
                self.server.next_id += 1
                response.update({key: str(value) if isinstance(value, (int, float)) else value for key, value in body.items()})
                response["transactionId"] = self.server.next_id
                self.server.transactions.setdefault(identifier, []).append({"id": response["transactionId"], "isin": body["isin"], "type": body["transactionType"], "date": body["tradeDate"], "quantity": {"raw": body["quantity"], "localized": str(body["quantity"])}, "amount": {"raw": body["amount"], "localized": str(body["amount"])}})
            elif name == "get_portfolio_transactions":
                entries = self.server.transactions.get(identifier, [])
                response = {"transactions": entries, "totalTransactionsCount": len(entries)}
        self.send_json(status, response)



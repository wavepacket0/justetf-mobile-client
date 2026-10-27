import ctypes
import json
import math
import os
import sys
import threading
import weakref
from dataclasses import asdict
from functools import lru_cache
from pathlib import Path
from typing import Any

from .requests import (
    CreatePortfolioRequest,
    PriceAlertRequest,
    SavedFilterData,
    SavedFilterRequest,
    ShareTransactionRequest,
)

from .options import (
    AllocationOptions,
    PageOptions,
    PortfolioChartOptions,
    PortfolioCompositionOptions,
    PortfolioListOptions,
    PortfolioOptions,
    TradeQuoteOptions,
    WatchlistOptions,
    ChartOptions,
    CurrencyOptions,
    ExposureOptions,
    HoldingETFsOptions,
    IndexPerformanceOptions,
    MarketOverviewOptions,
    OrderFeesOptions,
    PeriodEndOptions,
    RecommendedFiltersOptions,
    SavingsPlansOptions,
    CardsOptions,
    ComparisonOptions,
    HoldingSearchOptions,
    MarketOptions,
    SearchISINsOptions,
    SearchMetadataOptions,
    SearchOptions,
    UniverseOptions,
)


class ClientError(RuntimeError):
    pass


class APIError(ClientError):
    def __init__(self, message: str, status_code: int | None = None) -> None:
        super().__init__(message)
        self.status_code = status_code


class TransportError(ClientError):
    pass


class ClientClosedError(ClientError):
    pass


class AuthenticationError(ClientError):
    pass


class _Buffer(ctypes.Structure):
    _fields_ = [("data", ctypes.c_void_p), ("length", ctypes.c_size_t)]


def _decode_result(body: bytes) -> Any:
    return json.loads(body) if body else None


def _option_parameters(options: Any, options_type: type[Any]) -> dict[str, Any]:
    if options is None:
        options = options_type()
    if not isinstance(options, options_type):
        raise TypeError(f"options must be {options_type.__name__}")
    return {key: value for key, value in asdict(options).items() if value is not None}


def _mutation_parameters(request: Any, request_type: type[Any], numeric_fields: tuple[str, ...] = ()) -> dict[str, Any]:
    if not isinstance(request, request_type):
        raise TypeError(f"request must be {request_type.__name__}")
    parameters = _option_parameters(request, request_type)
    for name in numeric_fields:
        if not isinstance(parameters.get(name), str):
            raise TypeError(f"{name} must be an exact decimal string")
    return parameters


class _NativeLibrary:
    def __init__(self, path: str) -> None:
        self.lib = ctypes.CDLL(path)
        self.lib.justetf_abi_version.argtypes = []
        self.lib.justetf_abi_version.restype = ctypes.c_uint32
        if self.lib.justetf_abi_version() != 1:
            raise ClientError("Unsupported justETF native ABI version; expected 1")
        self.lib.justetf_client_new.argtypes = [
            ctypes.c_char_p,
            ctypes.c_size_t,
            ctypes.POINTER(ctypes.c_uint64),
            ctypes.POINTER(_Buffer),
        ]
        self.lib.justetf_client_new.restype = ctypes.c_int
        self.lib.justetf_profile.argtypes = [
            ctypes.c_uint64,
            ctypes.c_char_p,
            ctypes.c_size_t,
            ctypes.POINTER(_Buffer),
        ]
        self.lib.justetf_profile.restype = ctypes.c_int
        try:
            self.lib.justetf_call.argtypes = [
                ctypes.c_uint64,
                ctypes.c_char_p,
                ctypes.c_size_t,
                ctypes.c_char_p,
                ctypes.c_size_t,
                ctypes.POINTER(_Buffer),
            ]
        except AttributeError as error:
            raise ClientError("Native library lacks justetf_call; rebuild the native library") from error
        self.lib.justetf_call.restype = ctypes.c_int
        self.lib.justetf_client_close.argtypes = [ctypes.c_uint64]
        self.lib.justetf_client_close.restype = ctypes.c_int
        self.lib.justetf_buffer_free.argtypes = [ctypes.c_void_p]
        self.lib.justetf_buffer_free.restype = None

    def consume(self, status: int, output: _Buffer) -> bytes:
        try:
            if output.length > 4 << 20 or (output.length and not output.data):
                raise ClientError("Invalid native buffer")
            body = ctypes.string_at(output.data, output.length) if output.data else b""
        finally:
            self.lib.justetf_buffer_free(output.data)
        if status == 0:
            return body
        try:
            failure = json.loads(body)
            message = failure["message"]
        except (ValueError, KeyError, TypeError) as error:
            raise ClientError("Invalid native error response") from error
        if status == 1:
            raise ValueError(message)
        if status == 2:
            raise APIError(message, failure.get("status"))
        if status == 3:
            raise TransportError(message)
        if status == 4:
            raise ClientClosedError(message)
        if status == 6:
            raise AuthenticationError(message)
        raise ClientError(message)

    def close(self, handle: int) -> None:
        self.lib.justetf_client_close(handle)


@lru_cache(maxsize=None)
def _load_library(path: str) -> _NativeLibrary:
    return _NativeLibrary(path)


class Client:
    def __init__(
        self,
        library_path: str | Path | None = None,
        *,
        api_key: str | None = None,
        base_url: str | None = None,
        language: str = "it",
        country: str = "IT",
        currency: str = "EUR",
        timeout: float = 15.0,
    ) -> None:
        key = api_key if api_key is not None else os.environ.get("JUSTETF_API_KEY", "")
        if not math.isfinite(timeout) or not 0 < timeout <= 300:
            raise ValueError("timeout must be finite and between 0 (exclusive) and 300 seconds")
        if library_path is None:
            name = (
                "justetf-native.dll" if sys.platform == "win32"
                else "libjustetf-native.dylib" if sys.platform == "darwin"
                else "libjustetf-native.so"
            )
            library_path = os.environ.get("JUSTETF_LIBRARY") or Path(__file__).with_name(name)
        self._native = _load_library(str(Path(library_path).resolve(strict=True)))
        config = json.dumps(
            {
                "api_key": key,
                "base_url": base_url if base_url is not None else os.environ.get("JUSTETF_BASE_URL", ""),
                "language": language,
                "country": country,
                "currency": currency,
                "timeout_ms": math.ceil(timeout * 1000),
            }
        ).encode("utf-8")
        self._lock = threading.Lock()
        handle = ctypes.c_uint64()
        output = _Buffer()
        status = self._native.lib.justetf_client_new(
            config, len(config), ctypes.byref(handle), ctypes.byref(output)
        )
        self._native.consume(status, output)
        self._handle = handle.value
        self._finalizer = weakref.finalize(self, self._native.close, self._handle)

    def _acquire_handle(self) -> int:
        with self._lock:
            if not self._handle:
                raise ClientClosedError("Client is closed")
            return self._handle

    def login(self, email: str, password: str) -> None:
        if not isinstance(email, str) or not isinstance(password, str):
            raise TypeError("email and password must be strings")
        self._call_json("login", {"email": email, "password": password})

    def refresh(self) -> None:
        self._call_json("refresh", {})

    def logout(self) -> None:
        self._call_json("logout", {})

    def is_authenticated(self) -> bool:
        result = _decode_result(self._call_json("is_authenticated", {}))
        if not isinstance(result, bool):
            raise ClientError("Invalid authentication status response")
        return result

    def get_profile_json(self, isin: str, options: MarketOptions | None = None) -> bytes:
        return self._instrument_json("get_profile", isin, options, MarketOptions)

    def _call_json(self, operation: str, parameters: dict[str, Any]) -> bytes:
        name = operation.encode("utf-8")
        encoded = json.dumps(parameters, allow_nan=False, separators=(",", ":")).encode("utf-8")
        handle = self._acquire_handle()
        output = _Buffer()
        status = self._native.lib.justetf_call(
            handle, name, len(name), encoded, len(encoded), ctypes.byref(output)
        )
        return self._native.consume(status, output)

    def get_profile(self, isin: str, options: MarketOptions | None = None) -> Any:
        return _decode_result(self.get_profile_json(isin, options))

    def _options_json(self, operation: str, options: Any, options_type: type[Any]) -> bytes:
        return self._call_json(operation, _option_parameters(options, options_type))

    def _instrument_json(self, operation: str, isin: str, options: Any, options_type: type[Any]) -> bytes:
        return self._identified_json(operation, "isin", isin, options, options_type)

    def _identified_json(self, operation: str, key: str, identifier: str, options: Any, options_type: type[Any]) -> bytes:
        if not isinstance(identifier, str):
            raise TypeError(f"{key} must be a string")
        return self._call_json(operation, {**_option_parameters(options, options_type), key: identifier})

    def get_universe_countries_json(self) -> bytes:
        return self._call_json("get_universe_countries", {})

    def get_universe_countries(self) -> Any:
        return _decode_result(self.get_universe_countries_json())

    def get_account_access_json(self, country: str | None = None) -> bytes:
        if country is not None and not isinstance(country, str):
            raise TypeError("country must be a string or None")
        return self._call_json("get_account_access", {"country": country} if country is not None else {})

    def get_account_access(self, country: str | None = None) -> Any:
        return _decode_result(self.get_account_access_json(country))

    def get_search_metadata_json(self, options: SearchMetadataOptions | None = None) -> bytes:
        return self._options_json("get_search_metadata", options, SearchMetadataOptions)

    def get_search_metadata(self, options: SearchMetadataOptions | None = None) -> Any:
        return _decode_result(self.get_search_metadata_json(options))

    def search_etfs_json(self, options: SearchOptions | None = None) -> bytes:
        return self._options_json("search_etfs", options, SearchOptions)

    def search_etfs(self, options: SearchOptions | None = None) -> Any:
        return _decode_result(self.search_etfs_json(options))

    def search_isins_json(self, options: SearchISINsOptions | None = None) -> bytes:
        return self._options_json("search_isins", options, SearchISINsOptions)

    def search_isins(self, options: SearchISINsOptions | None = None) -> Any:
        return _decode_result(self.search_isins_json(options))

    def get_cards_json(self, options: CardsOptions | None = None) -> bytes:
        return self._options_json("get_cards", options, CardsOptions)

    def get_cards(self, options: CardsOptions | None = None) -> Any:
        return _decode_result(self.get_cards_json(options))

    def compare_etfs_json(self, options: ComparisonOptions | None = None) -> bytes:
        return self._options_json("compare_etfs", options, ComparisonOptions)

    def compare_etfs(self, options: ComparisonOptions | None = None) -> Any:
        return _decode_result(self.compare_etfs_json(options))

    def search_holdings_json(self, options: HoldingSearchOptions | None = None) -> bytes:
        return self._options_json("search_holdings", options, HoldingSearchOptions)

    def search_holdings(self, options: HoldingSearchOptions | None = None) -> Any:
        return _decode_result(self.search_holdings_json(options))

    def get_quote_json(self, isin: str, options: CurrencyOptions | None = None) -> bytes:
        return self._instrument_json("get_quote", isin, options, CurrencyOptions)

    def get_quote(self, isin: str, options: CurrencyOptions | None = None) -> Any:
        return _decode_result(self.get_quote_json(isin, options))

    def get_performance_chart_json(self, isin: str, options: ChartOptions | None = None) -> bytes:
        return self._instrument_json("get_performance_chart", isin, options, ChartOptions)

    def get_performance_chart(self, isin: str, options: ChartOptions | None = None) -> Any:
        return _decode_result(self.get_performance_chart_json(isin, options))

    def get_returns_json(self, isin: str, options: PeriodEndOptions | None = None) -> bytes:
        return self._instrument_json("get_returns", isin, options, PeriodEndOptions)

    def get_returns(self, isin: str, options: PeriodEndOptions | None = None) -> Any:
        return _decode_result(self.get_returns_json(isin, options))

    def get_risks_json(self, isin: str, options: PeriodEndOptions | None = None) -> bytes:
        return self._instrument_json("get_risks", isin, options, PeriodEndOptions)

    def get_risks(self, isin: str, options: PeriodEndOptions | None = None) -> Any:
        return _decode_result(self.get_risks_json(isin, options))

    def get_country_exposure_json(self, isin: str, options: ExposureOptions | None = None) -> bytes:
        return self._instrument_json("get_country_exposure", isin, options, ExposureOptions)

    def get_country_exposure(self, isin: str, options: ExposureOptions | None = None) -> Any:
        return _decode_result(self.get_country_exposure_json(isin, options))

    def get_sector_exposure_json(self, isin: str, options: ExposureOptions | None = None) -> bytes:
        return self._instrument_json("get_sector_exposure", isin, options, ExposureOptions)

    def get_sector_exposure(self, isin: str, options: ExposureOptions | None = None) -> Any:
        return _decode_result(self.get_sector_exposure_json(isin, options))

    def get_holding_exposure_json(self, isin: str, options: ExposureOptions | None = None) -> bytes:
        return self._instrument_json("get_holding_exposure", isin, options, ExposureOptions)

    def get_holding_exposure(self, isin: str, options: ExposureOptions | None = None) -> Any:
        return _decode_result(self.get_holding_exposure_json(isin, options))

    def get_holding_details_json(self, isin: str, options: MarketOptions | None = None) -> bytes:
        return self._instrument_json("get_holding_details", isin, options, MarketOptions)

    def get_holding_details(self, isin: str, options: MarketOptions | None = None) -> Any:
        return _decode_result(self.get_holding_details_json(isin, options))

    def get_holding_etfs_json(self, isin: str, options: HoldingETFsOptions | None = None) -> bytes:
        return self._instrument_json("get_holding_etfs", isin, options, HoldingETFsOptions)

    def get_holding_etfs(self, isin: str, options: HoldingETFsOptions | None = None) -> Any:
        return _decode_result(self.get_holding_etfs_json(isin, options))

    def get_brokers_json(self, options: UniverseOptions | None = None) -> bytes:
        return self._options_json("get_brokers", options, UniverseOptions)

    def get_brokers(self, options: UniverseOptions | None = None) -> Any:
        return _decode_result(self.get_brokers_json(options))

    def get_market_overview_json(self, options: MarketOverviewOptions | None = None) -> bytes:
        return self._options_json("get_market_overview", options, MarketOverviewOptions)

    def get_market_overview(self, options: MarketOverviewOptions | None = None) -> Any:
        return _decode_result(self.get_market_overview_json(options))

    def get_index_performance_json(self, options: IndexPerformanceOptions | None = None) -> bytes:
        return self._options_json("get_index_performance", options, IndexPerformanceOptions)

    def get_index_performance(self, options: IndexPerformanceOptions | None = None) -> Any:
        return _decode_result(self.get_index_performance_json(options))

    def get_recommended_filters_json(self, options: RecommendedFiltersOptions | None = None) -> bytes:
        return self._options_json("get_recommended_filters", options, RecommendedFiltersOptions)

    def get_recommended_filters(self, options: RecommendedFiltersOptions | None = None) -> Any:
        return _decode_result(self.get_recommended_filters_json(options))

    def get_savings_plans_json(self, isin: str, options: SavingsPlansOptions | None = None) -> bytes:
        return self._instrument_json("get_savings_plans", isin, options, SavingsPlansOptions)

    def get_savings_plans(self, isin: str, options: SavingsPlansOptions | None = None) -> Any:
        return _decode_result(self.get_savings_plans_json(isin, options))

    def get_order_fees_json(self, isin: str, options: OrderFeesOptions | None = None) -> bytes:
        return self._instrument_json("get_order_fees", isin, options, OrderFeesOptions)

    def get_order_fees(self, isin: str, options: OrderFeesOptions | None = None) -> Any:
        return _decode_result(self.get_order_fees_json(isin, options))

    def get_personal_data_json(self, country: str | None = None) -> bytes:
        if country is not None and not isinstance(country, str):
            raise TypeError("country must be a string or None")
        return self._call_json("get_personal_data", {"country": country} if country is not None else {})

    def get_personal_data(self, country: str | None = None) -> Any:
        return _decode_result(self.get_personal_data_json(country))

    def get_watchlists_json(self, options: WatchlistOptions | None = None) -> bytes:
        return self._options_json("get_watchlists", options, WatchlistOptions)

    def get_watchlists(self, options: WatchlistOptions | None = None) -> Any:
        return _decode_result(self.get_watchlists_json(options))

    def get_saved_filters_json(self) -> bytes:
        return self._call_json("get_saved_filters", {})

    def get_saved_filters(self) -> Any:
        return _decode_result(self.get_saved_filters_json())

    def get_price_alerts_json(self) -> bytes:
        return self._call_json("get_price_alerts", {})

    def get_price_alerts(self) -> Any:
        return _decode_result(self.get_price_alerts_json())

    def get_etf_price_alerts_json(self, isin: str) -> bytes:
        if not isinstance(isin, str):
            raise TypeError("isin must be a string")
        return self._call_json("get_etf_price_alerts", {"isin": isin})

    def get_etf_price_alerts(self, isin: str) -> Any:
        return _decode_result(self.get_etf_price_alerts_json(isin))

    def get_price_alert_json(self, isin: str, alert_id: str) -> bytes:
        if not isinstance(isin, str) or not isinstance(alert_id, str):
            raise TypeError("isin and alert_id must be strings")
        return self._call_json("get_price_alert", {"isin": isin, "alert_id": alert_id})

    def get_price_alert(self, isin: str, alert_id: str) -> Any:
        return _decode_result(self.get_price_alert_json(isin, alert_id))

    def get_portfolios_json(self, options: PortfolioListOptions | None = None) -> bytes:
        return self._options_json("get_portfolios", options, PortfolioListOptions)

    def get_portfolios(self, options: PortfolioListOptions | None = None) -> Any:
        return _decode_result(self.get_portfolios_json(options))

    def get_portfolio_json(self, portfolio_id: str, options: PortfolioOptions | None = None) -> bytes:
        return self._identified_json("get_portfolio", "portfolio_id", portfolio_id, options, PortfolioOptions)

    def get_portfolio(self, portfolio_id: str, options: PortfolioOptions | None = None) -> Any:
        return _decode_result(self.get_portfolio_json(portfolio_id, options))

    def get_portfolio_composition_json(self, portfolio_id: str, options: PortfolioCompositionOptions | None = None) -> bytes:
        return self._identified_json("get_portfolio_composition", "portfolio_id", portfolio_id, options, PortfolioCompositionOptions)

    def get_portfolio_composition(self, portfolio_id: str, options: PortfolioCompositionOptions | None = None) -> Any:
        return _decode_result(self.get_portfolio_composition_json(portfolio_id, options))

    def can_create_portfolio_json(self) -> bytes:
        return self._call_json("can_create_portfolio", {})

    def can_create_portfolio(self) -> Any:
        return _decode_result(self.can_create_portfolio_json())

    def get_trade_quote_json(self, isin: str, options: TradeQuoteOptions | None = None) -> bytes:
        return self._instrument_json("get_trade_quote", isin, options, TradeQuoteOptions)

    def get_trade_quote(self, isin: str, options: TradeQuoteOptions | None = None) -> Any:
        return _decode_result(self.get_trade_quote_json(isin, options))

    def get_portfolio_transactions_json(self, portfolio_id: str, options: PageOptions | None = None) -> bytes:
        return self._identified_json("get_portfolio_transactions", "portfolio_id", portfolio_id, options, PageOptions)

    def get_portfolio_transactions(self, portfolio_id: str, options: PageOptions | None = None) -> Any:
        return _decode_result(self.get_portfolio_transactions_json(portfolio_id, options))

    def get_portfolio_allocation_json(self, portfolio_id: str, options: AllocationOptions | None = None) -> bytes:
        return self._identified_json("get_portfolio_allocation", "portfolio_id", portfolio_id, options, AllocationOptions)

    def get_portfolio_allocation(self, portfolio_id: str, options: AllocationOptions | None = None) -> Any:
        return _decode_result(self.get_portfolio_allocation_json(portfolio_id, options))

    def get_portfolio_performance_chart_json(self, portfolio_id: str, options: PortfolioChartOptions | None = None) -> bytes:
        return self._identified_json("get_portfolio_performance_chart", "portfolio_id", portfolio_id, options, PortfolioChartOptions)

    def get_portfolio_performance_chart(self, portfolio_id: str, options: PortfolioChartOptions | None = None) -> Any:
        return _decode_result(self.get_portfolio_performance_chart_json(portfolio_id, options))

    def get_holding_supplementary_data_json(self, isin: str) -> bytes:
        if not isinstance(isin, str):
            raise TypeError("isin must be a string")
        return self._call_json("get_holding_supplementary_data", {"isin": isin})

    def get_holding_supplementary_data(self, isin: str) -> Any:
        return _decode_result(self.get_holding_supplementary_data_json(isin))

    def create_price_alert_json(self, isin: str, request: PriceAlertRequest) -> bytes:
        if not isinstance(isin, str):
            raise TypeError("isin must be a string")
        parameters = _mutation_parameters(request, PriceAlertRequest, ("price",))
        return self._call_json("create_price_alert", {**parameters, "isin": isin})

    def create_price_alert(self, isin: str, request: PriceAlertRequest) -> Any:
        return _decode_result(self.create_price_alert_json(isin, request))

    def delete_price_alert(self, isin: str, alert_id: str) -> None:
        if not isinstance(isin, str) or not isinstance(alert_id, str):
            raise TypeError("isin and alert_id must be strings")
        self._call_json("delete_price_alert", {"isin": isin, "alert_id": alert_id})

    def create_saved_filter(self, request: SavedFilterRequest) -> None:
        if not isinstance(request, SavedFilterRequest) or not isinstance(request.saved_filter_data, SavedFilterData):
            raise TypeError("request must be SavedFilterRequest with SavedFilterData")
        self._call_json("create_saved_filter", _mutation_parameters(request, SavedFilterRequest))

    def delete_saved_filter(self, filter_id: str) -> None:
        if not isinstance(filter_id, str):
            raise TypeError("filter_id must be a string")
        self._call_json("delete_saved_filter", {"filter_id": filter_id})

    def create_portfolio_json(self, request: CreatePortfolioRequest) -> bytes:
        return self._call_json("create_portfolio", _mutation_parameters(request, CreatePortfolioRequest))

    def create_portfolio(self, request: CreatePortfolioRequest) -> Any:
        return _decode_result(self.create_portfolio_json(request))

    def delete_portfolio(self, portfolio_id: str) -> None:
        if not isinstance(portfolio_id, str):
            raise TypeError("portfolio_id must be a string")
        self._call_json("delete_portfolio", {"portfolio_id": portfolio_id})

    def add_share_transaction_json(self, portfolio_id: str, request: ShareTransactionRequest) -> bytes:
        if not isinstance(portfolio_id, str):
            raise TypeError("portfolio_id must be a string")
        parameters = _mutation_parameters(request, ShareTransactionRequest, ("amount", "quantity", "quote", "trade_rate", "fund_rate", "fees", "taxes"))
        return self._call_json("add_share_transaction", {**parameters, "portfolio_id": portfolio_id})

    def add_share_transaction(self, portfolio_id: str, request: ShareTransactionRequest) -> Any:
        return _decode_result(self.add_share_transaction_json(portfolio_id, request))

    def close(self) -> None:
        with self._lock:
            self._handle = 0
            self._finalizer()

    def __enter__(self) -> "Client":
        self._acquire_handle()
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()
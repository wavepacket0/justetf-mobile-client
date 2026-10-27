from dataclasses import dataclass


@dataclass(frozen=True, kw_only=True)
class UniverseOptions:
    country: str | None = None
    universe_type: str | None = None


@dataclass(frozen=True, kw_only=True)
class MarketOptions(UniverseOptions):
    currency: str | None = None


@dataclass(frozen=True, kw_only=True)
class SearchMetadataOptions(MarketOptions):
    return_period_end: str | None = None
    app_version: str | None = None


@dataclass(frozen=True, kw_only=True)
class SearchOptions(MarketOptions):
    filters: list[str] | None = None
    query: str | None = None
    return_period_end: str | None = None
    return_period: str | None = None
    sort_by: str | None = None
    sort_dir: str | None = None
    limit: int | None = None
    page: int | None = None


@dataclass(frozen=True, kw_only=True)
class SearchISINsOptions(MarketOptions):
    filters: list[str] | None = None
    query: str | None = None
    return_period_end: str | None = None
    return_period: str | None = None
    sorting: str | None = None


@dataclass(frozen=True, kw_only=True)
class CardsOptions(MarketOptions):
    isins: list[str] | None = None
    return_period_end: str | None = None
    returns_period: str | None = None
    sorting: str | None = None
    limit: int | None = None
    page: int | None = None


@dataclass(frozen=True, kw_only=True)
class ComparisonOptions(MarketOptions):
    isins: list[str] | None = None


@dataclass(frozen=True, kw_only=True)
class HoldingSearchOptions(UniverseOptions):
    query: str | None = None
    sort_field: str | None = None
    sort_order: str | None = None
    limit: int | None = None
    offset: int | None = None


@dataclass(frozen=True, kw_only=True)
class CurrencyOptions:
    currency: str | None = None


@dataclass(frozen=True, kw_only=True)
class PeriodEndOptions(CurrencyOptions):
    return_period_end: str | None = None


@dataclass(frozen=True, kw_only=True)
class ChartOptions(CurrencyOptions):
    date_from: str | None = None
    date_to: str | None = None
    period: str | None = None
    reduce_data: bool | None = None
    values_type: str | None = None
    include_dividends: bool | None = None


@dataclass(frozen=True, kw_only=True)
class ExposureOptions:
    top: int | None = None
    scale: int | None = None
    threshold: str | None = None


@dataclass(frozen=True, kw_only=True)
class HoldingETFsOptions:
    country: str | None = None
    top: int | None = None
    page: int | None = None
    scale: int | None = None


@dataclass(frozen=True, kw_only=True)
class MarketOverviewOptions(MarketOptions):
    return_period_end: str | None = None
    returns_period: str | None = None
    home_screen_only: bool | None = None


@dataclass(frozen=True, kw_only=True)
class IndexPerformanceOptions(MarketOptions):
    performance_type: str | None = None
    performance_value: str | None = None
    performance_category: str | None = None
    limit: int | None = None


@dataclass(frozen=True, kw_only=True)
class RecommendedFiltersOptions(UniverseOptions):
    app_version: str | None = None


@dataclass(frozen=True, kw_only=True)
class SavingsPlansOptions(UniverseOptions):
    savings_rate: str | None = None
    sorting: str | None = None


@dataclass(frozen=True, kw_only=True)
class OrderFeesOptions(UniverseOptions):
    order_volume: str | None = None
    sorting: str | None = None


@dataclass(frozen=True, kw_only=True)
class WatchlistOptions(CurrencyOptions):
    return_period_end: str | None = None
    returns_period: str | None = None


@dataclass(frozen=True, kw_only=True)
class PageOptions:
    limit: int | None = None
    page: int | None = None


@dataclass(frozen=True, kw_only=True)
class PortfolioListOptions(PageOptions):
    period: str | None = None
    app_version: str | None = None


@dataclass(frozen=True, kw_only=True)
class PortfolioOptions:
    period: str | None = None
    app_version: str | None = None


@dataclass(frozen=True, kw_only=True)
class PortfolioCompositionOptions:
    period: str | None = None


@dataclass(frozen=True, kw_only=True)
class TradeQuoteOptions(CurrencyOptions):
    date: str | None = None


@dataclass(frozen=True, kw_only=True)
class AllocationOptions:
    grouping: str | None = None
    limit: int | None = None
    offset: int | None = None


@dataclass(frozen=True, kw_only=True)
class PortfolioChartOptions(CurrencyOptions):
    date_from: str | None = None
    date_to: str | None = None
    period: str | None = None
    reduce_data: bool | None = None
    values_type: str | None = None
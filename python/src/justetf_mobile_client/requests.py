from dataclasses import dataclass, field


@dataclass(frozen=True, kw_only=True)
class PriceAlertRequest:
    price: str
    condition: str
    comment: str = ""


@dataclass(frozen=True, kw_only=True)
class SavedFilterData:
    filters: list[str] = field(default_factory=list)
    keywords: str = ""


@dataclass(frozen=True, kw_only=True)
class SavedFilterRequest:
    label: str
    saved_filter_data: SavedFilterData = field(default_factory=SavedFilterData)


@dataclass(frozen=True, kw_only=True)
class CreatePortfolioRequest:
    name: str
    investment: str
    currency: str | None = None


@dataclass(frozen=True, kw_only=True)
class ShareTransactionRequest:
    trade_date: str
    transaction_type: str
    amount: str
    isin: str
    quantity: str
    quote: str
    quote_currency: str
    instrument_currency: str
    portfolio_currency: str
    quote_currency_type: str
    trade_rate: str
    fund_rate: str
    fees: str
    taxes: str
    comment: str = ""
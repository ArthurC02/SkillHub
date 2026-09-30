from dataclasses import dataclass
from decimal import Decimal


class LoanRefused(Exception):
    pass


@dataclass(frozen=True)
class LoanQuote:
    days: int
    renewable: bool
    late_fee: Decimal


def loan_days(item_kind: str, member: bool) -> int:
    if item_kind == "reference":
        raise LoanRefused("reference items stay in the library")
    if item_kind == "dvd":
        return 14 if member else 7
    return 28 if member else 14


def may_renew(item_kind: str, member: bool) -> bool:
    return member and item_kind != "dvd"


def late_fee(days_late: int, member: bool) -> Decimal:
    if days_late <= 0:
        return Decimal("0.00")
    daily = Decimal("0.20") if member else Decimal("0.50")
    cap = Decimal("5.00") if member else Decimal("10.00")
    return min(daily * days_late, cap)


def quote_loan(item_kind: str, borrower: str, days_late: int = 0) -> LoanQuote:
    if borrower not in ("member", "guest"):
        raise LoanRefused(f"unknown borrower: {borrower}")
    member = borrower == "member"
    return LoanQuote(
        days=loan_days(item_kind, member),
        renewable=may_renew(item_kind, member),
        late_fee=late_fee(days_late, member),
    )

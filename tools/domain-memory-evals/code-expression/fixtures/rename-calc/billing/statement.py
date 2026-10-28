from decimal import Decimal

from billing.invoice import calc


def statement_total(invoices: list[list[Decimal]]) -> Decimal:
    return sum((calc(amounts) for amounts in invoices), Decimal("0"))

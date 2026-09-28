from decimal import Decimal

from shop.money import to_cents


def line_total(quantity: int, unit_price: Decimal) -> Decimal:
    return to_cents(quantity * unit_price)


def order_total(lines: list[tuple[int, Decimal]]) -> Decimal:
    return to_cents(sum((line_total(q, p) for q, p in lines), Decimal("0")))

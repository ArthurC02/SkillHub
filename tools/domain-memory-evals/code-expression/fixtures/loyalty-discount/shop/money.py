from decimal import ROUND_HALF_UP, Decimal

CENT = Decimal("0.01")


def to_cents(amount: Decimal) -> Decimal:
    return amount.quantize(CENT, rounding=ROUND_HALF_UP)

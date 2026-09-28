from decimal import Decimal


def calc(amounts: list[Decimal]) -> Decimal:
    return sum(amounts, Decimal("0"))


def invoice_lines(amounts: list[Decimal]) -> list[str]:
    lines = [f"item {index}: {amount}" for index, amount in enumerate(amounts, 1)]
    lines.append(f"total: {calc(amounts)}")
    return lines

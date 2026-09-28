from dataclasses import dataclass
from decimal import ROUND_HALF_UP, Decimal


class QuoteRefused(Exception):
    pass


@dataclass
class Shipment:
    service: str
    weight_kg: Decimal
    longest_side_cm: int
    postcode: str
    order_value: Decimal
    hazardous: bool = False


def quote(s: Shipment) -> Decimal:
    r = Decimal("0")
    x = Decimal("0")
    f = False
    if s.service == "air":
        if s.hazardous:
            raise QuoteRefused("hazardous")
        else:
            if s.longest_side_cm > 200:
                raise QuoteRefused("oversize")
            else:
                r = Decimal("9.00") + s.weight_kg * Decimal("2.50")
                if s.longest_side_cm > 120:
                    x = x + Decimal("15.00")
                    f = True
                if s.postcode.startswith("9"):
                    t = r * Decimal("0.20")
                    if t > Decimal("12.00"):
                        t = Decimal("12.00")
                    x = x + t
    else:
        if s.service == "ground":
            if s.longest_side_cm > 200:
                raise QuoteRefused("oversize")
            else:
                r = Decimal("5.00") + s.weight_kg * Decimal("1.20")
                if s.longest_side_cm > 120:
                    x = x + Decimal("15.00")
                    f = True
                if s.postcode.startswith("9"):
                    t = r * Decimal("0.20")
                    if t > Decimal("12.00"):
                        t = Decimal("12.00")
                    x = x + t
                if s.order_value >= Decimal("100"):
                    if not f:
                        r = Decimal("0")
        else:
            raise QuoteRefused("service")
    return (r + x).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)

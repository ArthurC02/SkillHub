from dataclasses import dataclass
from decimal import Decimal

HOME_COUNTRY = "NL"
REMOTE_POSTCODE_PREFIXES = ("98", "99")
HEAVY_PARCEL_KG = Decimal("20")


class PriceRefused(Exception):
    pass


@dataclass(frozen=True)
class Order:
    street: str
    city: str
    postcode: str
    country: str
    weight_kg: Decimal
    service: str


def delivery_price(
    street: str,
    city: str,
    postcode: str,
    country: str,
    weight_kg: Decimal,
    service: str,
) -> Decimal:
    if not street or not city:
        raise PriceRefused("the address is incomplete")
    if country != HOME_COUNTRY and service == "same_day":
        raise PriceRefused("same-day delivery stays inside the country")
    base = Decimal("5.00") if country == HOME_COUNTRY else Decimal("15.00")
    if service == "same_day":
        base += Decimal("10.00")
    if postcode.startswith(REMOTE_POSTCODE_PREFIXES):
        base += Decimal("3.50")
    if weight_kg > HEAVY_PARCEL_KG:
        base += (weight_kg - HEAVY_PARCEL_KG) * Decimal("0.40")
    return base.quantize(Decimal("0.01"))


def quote_delivery(order: Order) -> Decimal:
    return delivery_price(
        order.street,
        order.city,
        order.postcode,
        order.country,
        order.weight_kg,
        order.service,
    )

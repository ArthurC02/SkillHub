import unittest
from decimal import Decimal

from delivery.price import Order, PriceRefused, quote_delivery


def order(**changes: object) -> Order:
    fields: dict[str, object] = {
        "street": "Main 1",
        "city": "Utrecht",
        "postcode": "3511",
        "country": "NL",
        "weight_kg": Decimal("2"),
        "service": "standard",
    }
    fields.update(changes)
    return Order(**fields)  # type: ignore[arg-type]


class DeliveryPriceTest(unittest.TestCase):
    def test_a_home_parcel_pays_the_home_base_rate(self) -> None:
        self.assertEqual(quote_delivery(order()), Decimal("5.00"))

    def test_a_remote_postcode_adds_its_fee(self) -> None:
        self.assertEqual(quote_delivery(order(postcode="9901")), Decimal("8.50"))

    def test_weight_above_the_heavy_parcel_limit_is_charged(self) -> None:
        self.assertEqual(quote_delivery(order(weight_kg=Decimal("25"))), Decimal("7.00"))

    def test_same_day_abroad_is_refused(self) -> None:
        with self.assertRaises(PriceRefused):
            quote_delivery(order(country="DE", service="same_day"))

    def test_an_incomplete_address_is_refused(self) -> None:
        with self.assertRaises(PriceRefused):
            quote_delivery(order(city=""))


if __name__ == "__main__":
    unittest.main()

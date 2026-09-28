import unittest
from decimal import Decimal

from shipping.quote import QuoteRefused, Shipment, quote


def shipment(**changes) -> Shipment:
    fields = {
        "service": "ground",
        "weight_kg": Decimal("2"),
        "longest_side_cm": 50,
        "postcode": "10001",
        "order_value": Decimal("30"),
        "hazardous": False,
    }
    fields.update(changes)
    return Shipment(**fields)


class QuoteTest(unittest.TestCase):
    def test_ground_charges_a_base_rate_and_a_rate_per_kilogram(self) -> None:
        self.assertEqual(quote(shipment()), Decimal("7.40"))

    def test_air_refuses_hazardous_goods(self) -> None:
        with self.assertRaises(QuoteRefused):
            quote(shipment(service="air", hazardous=True))

    def test_a_long_parcel_pays_the_oversize_surcharge(self) -> None:
        self.assertEqual(quote(shipment(longest_side_cm=121)), Decimal("22.40"))

    def test_a_large_order_ships_free_by_ground(self) -> None:
        self.assertEqual(quote(shipment(order_value=Decimal("100"))), Decimal("0.00"))


if __name__ == "__main__":
    unittest.main()

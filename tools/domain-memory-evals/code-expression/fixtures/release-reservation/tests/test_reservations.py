import unittest

from stock.reservations import InsufficientStock, StockItem


class ReservationTest(unittest.TestCase):
    def test_a_reservation_reduces_what_is_available(self) -> None:
        item = StockItem("sku-1", 5)
        item.reserve("o-1", 2)
        self.assertEqual(item.available(), 3)

    def test_the_last_unit_can_be_reserved(self) -> None:
        item = StockItem("sku-1", 5)
        item.reserve("o-1", 5)
        self.assertEqual(item.available(), 0)

    def test_more_than_what_is_available_is_refused(self) -> None:
        item = StockItem("sku-1", 5)
        with self.assertRaises(InsufficientStock):
            item.reserve("o-1", 6)


if __name__ == "__main__":
    unittest.main()

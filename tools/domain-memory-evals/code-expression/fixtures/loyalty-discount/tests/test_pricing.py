import unittest
from decimal import Decimal

from shop.pricing import line_total, order_total


class PricingTest(unittest.TestCase):
    def test_a_line_total_rounds_half_a_cent_up(self) -> None:
        self.assertEqual(line_total(3, Decimal("0.335")), Decimal("1.01"))

    def test_an_order_total_adds_its_lines(self) -> None:
        lines = [(2, Decimal("10.00")), (1, Decimal("0.50"))]
        self.assertEqual(order_total(lines), Decimal("20.50"))

    def test_an_order_with_no_lines_totals_zero(self) -> None:
        self.assertEqual(order_total([]), Decimal("0.00"))


if __name__ == "__main__":
    unittest.main()

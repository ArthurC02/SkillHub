import unittest
from decimal import Decimal

from orders.confirm_order import confirm_order
from orders.order import Order, OrderAlreadyConfirmed
from orders.order_book import OrderBook, UnknownOrder


def book_with(order: Order) -> OrderBook:
    book = OrderBook()
    book.add(order)
    return book


class ConfirmOrderTest(unittest.TestCase):
    def test_confirming_marks_the_order_confirmed(self) -> None:
        book = book_with(Order("o-1", "+886900000001", Decimal("120.00")))
        self.assertTrue(confirm_order("o-1", book).confirmed)

    def test_an_order_is_confirmed_once(self) -> None:
        book = book_with(Order("o-1", "+886900000001", Decimal("120.00"), True))
        with self.assertRaises(OrderAlreadyConfirmed):
            confirm_order("o-1", book)

    def test_an_unknown_order_is_refused(self) -> None:
        with self.assertRaises(UnknownOrder):
            confirm_order("o-9", OrderBook())


if __name__ == "__main__":
    unittest.main()

import unittest
from decimal import Decimal

from library.loans import LoanRefused, quote_loan


class LoanTest(unittest.TestCase):
    def test_a_member_borrows_a_book_for_four_weeks_and_may_renew(self) -> None:
        quote = quote_loan("book", "member")
        self.assertEqual((quote.days, quote.renewable), (28, True))

    def test_a_guest_borrows_a_dvd_for_one_week_and_may_not_renew(self) -> None:
        quote = quote_loan("dvd", "guest")
        self.assertEqual((quote.days, quote.renewable), (7, False))

    def test_a_reference_item_is_never_lent(self) -> None:
        with self.assertRaises(LoanRefused):
            quote_loan("reference", "member")

    def test_a_guest_late_fee_stops_at_its_cap(self) -> None:
        self.assertEqual(quote_loan("book", "guest", 30).late_fee, Decimal("10.00"))

    def test_an_unknown_borrower_is_refused(self) -> None:
        with self.assertRaises(LoanRefused):
            quote_loan("book", "visitor")


if __name__ == "__main__":
    unittest.main()

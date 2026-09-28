import unittest
from decimal import Decimal

from billing.invoice import calc, invoice_lines
from billing.statement import statement_total


class BillingTest(unittest.TestCase):
    def test_an_invoice_totals_its_amounts(self) -> None:
        self.assertEqual(calc([Decimal("1.50"), Decimal("2.25")]), Decimal("3.75"))

    def test_an_invoice_ends_with_its_total(self) -> None:
        self.assertEqual(invoice_lines([Decimal("4")])[-1], "total: 4")

    def test_a_statement_totals_its_invoices(self) -> None:
        invoices = [[Decimal("1")], [Decimal("2"), Decimal("3")]]
        self.assertEqual(statement_total(invoices), Decimal("6"))


if __name__ == "__main__":
    unittest.main()

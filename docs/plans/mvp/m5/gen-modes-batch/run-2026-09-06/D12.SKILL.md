---
name: supplier-invoice-processing
description: Process a supplier invoice against a purchase order, approve matching invoices, and route discrepancies for supplier notification and archiving. Use when an invoice-processing flow needs to be followed from receipt through approval, payment scheduling, discrepancy handling, and PDF archiving.
---

# Supplier invoice processing

Follow this flow exactly for each supplier invoice.

## 1) Receive the supplier invoice
- Start when a supplier invoice is received.
- Identify the invoice PDF and any accompanying details needed to process it.

## 2) Match the invoice to the PO number
- Find the purchase order number on the invoice.
- Match the invoice to the corresponding PO record.
- Confirm you are working on the correct PO before continuing.

## 3) Check whether the amounts match
- Compare the invoice amounts to the PO amounts.
- Decide whether the amounts match.

### If the amounts match
1. Approve the invoice in NetSuite.
2. Schedule payment for 30 days from approval.
3. Archive the PDF to Drive.

### If the amounts do not match
1. Email the supplier about the discrepancy.
2. Archive the PDF to Drive.

## 4) Finish
- Ensure the invoice PDF has been archived to Drive in all cases.
- Do not skip archiving, even when the invoice is approved and payment is scheduled.
- If required information is missing, stop and gather the missing invoice or PO details before deciding whether the amounts match.

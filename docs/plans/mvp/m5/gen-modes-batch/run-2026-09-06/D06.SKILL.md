---
name: refund-request-flow
description: Handle customer refund requests by following the refund decision flow shown in the diagram. Use when a customer asks for a refund and you need to decide between a full refund and store credit, then record the outcome.
---

# Refund request flow

Follow this process whenever a customer asks for a refund.

## 1) Check the order age
Determine whether the order was placed within 14 days.

- **If yes**: continue to the full-refund path.
- **If no**: skip the refund steps and go directly to offering store credit instead.

## 2) If the order is within 14 days
1. Issue a **full refund via Stripe**.
2. **Email a confirmation** to the customer.
3. **Offer store credit instead** if appropriate for the situation.
4. **Log the outcome in the CRM**.

## 3) If the order is not within 14 days
1. **Offer store credit instead**.
2. **Log the outcome in the CRM**.

## 4) Completion check
Make sure the final CRM entry clearly records:
- whether the order was within 14 days,
- whether a full refund was issued or store credit was offered,
- whether confirmation was emailed.

If any required system action cannot be completed, report the blocker clearly and still record the partial outcome in the CRM if possible.

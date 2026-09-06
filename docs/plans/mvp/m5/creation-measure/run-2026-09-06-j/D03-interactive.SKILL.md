---
name: zendesk-ticket-response
description: Classifies a provided Zendesk support ticket, drafts a concise English reply, and recommends tagging, sending, and closing actions. Use when given a ticket title and body that need a category, Zendesk tag, response draft, and proposed next actions without directly modifying Zendesk.
---

# Zendesk Ticket Processing

Process the supplied ticket once, following the four workflow nodes below in their stated order. Use only the ticket title and body provided in the input. Do not claim to have performed any Zendesk or backend operation.

## Receive support ticket

Read the supplied ticket title and body as the support ticket to process. Retain concrete details needed for the response, such as the customer’s name, order number, dates, amounts, product names, symptoms, and requested resolution.

## Tag with Zendesk category

Choose the single best category from this taxonomy:

- `account-access`: Sign-in, password, verification, locked-account, or account-recovery issues.
- `billing-refunds`: Charges, duplicate charges, invoices, payment problems, refunds, or billing disputes.
- `technical-issue`: Errors, failures, defects, outages, or troubleshooting requests.
- `order-shipping`: Order status, delivery, tracking, missing parcels, or shipping problems.
- `cancellation`: Requests to cancel an order, subscription, or service.
- `product-question`: Questions about product features, availability, use, or compatibility.
- `other`: Tickets that do not reasonably fit another category.

Use the selected category value unchanged as the Zendesk tag.

## Draft reply in 200 words

Write a customer-facing reply in English containing no more than 200 English words. Address the customer’s actual issue and incorporate relevant identifiers or facts from the ticket. Keep the tone concise, courteous, and helpful.

Do not invent policies, timelines, case numbers, account details, or actions not stated in the ticket. Do not claim that a refund, investigation, account change, shipment update, or other backend operation has already occurred. When resolution requires backend access, explain the appropriate next step without implying that it has been completed.

## Send and close ticket

Because no Zendesk API, runtime tool, credentials, or permissions are available, do not send the reply, apply the tag, or close the ticket. Instead, present those actions as recommendations for a human or later authorized automation.

Return exactly these four clearly labeled Markdown sections:

```markdown
## Category
<selected category>

## Zendesk tag
<the same selected category>

## Reply draft
<English customer-facing reply of no more than 200 words>

## Recommended actions
1. Apply the `<selected category>` tag.
2. Send the reply draft.
3. Close the ticket.

These Zendesk actions are recommendations only and were not executed by this Skill.
```

Complete the classification, tag, reply, and recommended actions in one response. Do not end by asking the user a question.
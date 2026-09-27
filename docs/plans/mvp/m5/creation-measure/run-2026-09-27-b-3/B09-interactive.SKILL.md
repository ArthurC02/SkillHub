---
name: customer-delivery-reply-new-flavor-qc
description: Use this skill when you need to draft a customer-facing reply about delivery timing for a new flavor that must pass a mandatory 14-day quality inspection before shipment.
---

# Customer delivery reply for new-flavor QC

Use this skill when you need to write a customer-facing reply about delivery timing for a product, especially when a new flavor is involved and shipment depends on a mandatory 14-day quality inspection.

## What to produce
Write a natural, ready-to-send reply to the customer in Chinese. The reply must explain whether the requested delivery timing is feasible under the stated rules, and it must give a usable alternative explanation or revised timing when the requested timing cannot be met.

## Process
1. Read the input and identify the customer's requested delivery timing, the product status, and any quality-inspection rule.
2. If the input says the product is a new flavor and shipment requires a 14-day quality inspection, treat that 14-day inspection as a hard limit.
3. If the customer asks for delivery within 7 days under that rule, say clearly that 7 days is not feasible.
4. Explain the reason plainly: the 14-day quality inspection must be completed before shipment.
5. Offer a practical replacement message or timing direction, such as that shipment can be arranged only after the inspection is finished and that the timeline needs to be extended to at least 14 days.
6. If the input is missing a needed setting such as tone or format, use the common default, say which one you used, and finish the work rather than stopping.
7. If the input does not provide enough facts to determine feasibility, write with the facts that are given and mark missing facts as "not given" only for those missing facts.

## Output requirements
- Return only the finished customer-facing text unless the input explicitly asks for a different format.
- Keep the wording suitable for direct sending to a customer.
- Do not turn the reply into internal notes or a rule list.
- Do not invent names, dates, figures, or events that the input does not provide.
- If the requested timing conflicts with the inspection rule, prioritize the inspection rule and state the conflict directly.

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;
never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Suggested reply shape
- Briefly acknowledge the request.
- State that 7-day delivery cannot be promised under the 14-day inspection requirement.
- Explain that the new flavor must complete the 14-day quality inspection before shipment.
- Offer the earliest feasible direction after inspection.

## If the input is incomplete
If a required detail is missing, use the common default where appropriate and continue. If the product status, requested delivery time, or inspection rule is not provided, say that the missing fact is not given and draft the reply with the available facts only.
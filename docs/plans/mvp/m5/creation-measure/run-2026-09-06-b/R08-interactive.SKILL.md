---
name: return-request-7-day-reply
description: Handle customer return requests by checking whether the request falls within 7 days and replying with the matching standard email. Use when you receive a return request and need a ready-to-send response based on the 7-day rule.
---

# Purpose
This skill helps an agent handle a customer return request by deciding whether it is within 7 days and then generating the matching standard reply email.

# When to use
Use this skill when a customer asks for a return and the response must depend on whether the request is within the 7-day return window.

# Inputs to read
Look for:
- the request date or purchase date
- the date the request was received
- any statement that clearly indicates the request is within or beyond 7 days
- the customer’s language and tone if the reply should match it

# Decision rule
1. Determine the elapsed time between the relevant start date and the request date.
2. If the elapsed time is 7 days or fewer, treat the request as "within 7 days".
3. If the elapsed time is greater than 7 days, treat the request as "over 7 days".
4. If the dates are missing or unclear, do not guess. Ask for the missing date information before writing the final reply.

# Output requirements
- Produce only the customer-facing reply email.
- Do not show your calculation or reasoning.
- Keep the reply clearly tied to one of the two outcomes:
  - within 7 days
  - over 7 days
- Make the wording ready to send to the customer.

# Reply templates
## Within 7 days
Use a polite, service-oriented tone.

Subject: Return Request Approved

Hello,

Thank you for contacting us. We have reviewed your return request and confirmed that it is within our 7-day return window.

Please follow the return instructions below: 
- [Explain the return steps here based on the company’s standard process]
- [Include any required reference number, packaging, or shipping details if available]

If you have any questions, please reply to this email and we will be happy to help.

Best regards,
Customer Support

## Over 7 days
Use a polite but firm tone.

Subject: Return Request Not Eligible

Hello,

Thank you for reaching out. We have reviewed your return request and confirmed that it falls outside our 7-day return window.

As a result, we are unable to approve this return under our standard policy.

If you would like, we can still help with general product questions or other support options.

Best regards,
Customer Support

# Quality checks before sending
- The email matches the correct time-window outcome.
- The email is complete and directly usable.
- The reply contains no internal analysis.
- The message stays aligned with the company’s return policy.
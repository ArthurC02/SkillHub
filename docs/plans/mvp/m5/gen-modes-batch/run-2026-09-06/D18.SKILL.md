---
name: access-request-approval-flow
description: Use this skill when you need to process an access request by following an approval decision flow, including conditional checks, granting access, rejection, and audit logging.
---

# Access request processing flow

Follow the flow in order and do not skip any decision.

## 1) Receive the access request
Start when an access request is received.

## 2) Check whether the manager approved it
- If the manager did **not** approve the request, go to **Reject with a reason**.
- If the manager **did** approve the request, continue to the next check.

## 3) Check whether the request is for a production system
- If it is **not** for a production system, go to **Reject with a reason**.
- If it **is** for a production system, continue to the next step.

## 4) Require the security training certificate
Before granting access, confirm the security training certificate is required and handled as part of the approval path.

## 5) Grant the role in Okta
If all required approvals and checks passed, grant the requested role in Okta.

## 6) Reject with a reason
If any required approval or check fails, reject the request and include a clear reason for the rejection.

## 7) Log it in the audit sheet
After either granting the role or rejecting the request, log the outcome in the audit sheet.

## Decision summary
- **Manager approved = No** → Reject with a reason → Log it in the audit sheet
- **Manager approved = Yes** and **Production system = No** → Reject with a reason → Log it in the audit sheet
- **Manager approved = Yes** and **Production system = Yes** → Require the security training certificate → Grant the role in Okta → Log it in the audit sheet

---
name: rewrite-consent-form-to-patient-letter
description: Rewrite a completed self-pay medical-device consent form into a patient-friendly explanatory letter. Use this when the input is a formal clinic consent text and you need a clearer, more conversational version while preserving required names, numbers, and deadlines exactly.
---

# Goal
Rewrite a clinic’s completed self-pay medical-device consent form into a patient-facing explanatory letter that is easier to understand and sounds less like an official document.

## When to use this Skill
Use this Skill when you are given a completed consent-form text and need to turn it into a clear, conversational letter for the patient, while keeping the required device name, self-pay amount, health-insurance reimbursement cap, and patient signature deadline exactly unchanged.

## What to do
1. Read the source consent text carefully.
2. Rewrite it as a patient-friendly letter in a more conversational, easy-to-understand tone.
3. Keep the original meaning intact.
4. Preserve these items exactly as written in the source:
   - the medical device name
   - the self-pay amount
   - the health-insurance reimbursement cap
   - the patient signature deadline
5. You may rephrase all other parts for clarity and tone.
6. Do not add new medical advice, legal claims, or facts that are not in the source.
7. If the source contains multiple required protected items, preserve each one exactly as written.
8. Return the finished letter directly.

## Writing rules
- Make the tone warm, plain, and easy for a patient to understand.
- Remove stiff official wording where possible.
- Do not change the spelling, punctuation, numbers, currency symbols, dates, or labels of the protected items.
- Do not silently omit anything that the source says must remain.
- If the input asks for both a strict length limit and keeping everything, keep the hard limit and state in one line what you left out.
- Never invent a fact the input does not give — no name, date, figure, or event — and mark only such a missing fact as not given, written in the language of the output.
- When the setting the work needs is missing, use the common default, name what you chose — the actual values or items, never just a label like “general” — and finish the work rather than stopping.
- When the output lists amounts or quantities that belong together, give their total.
- Work every figure out step by step and add the result up once more before giving it.
- Write the output, labels included, in the language of the input.
- You cannot send, post, schedule, monitor, or fetch anything, so if the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
Return only the rewritten patient letter, ready to use.

## Quality checks
Before finishing, confirm that:
- the device name appears exactly as in the source
- the self-pay amount appears exactly as in the source
- the reimbursement cap appears exactly as in the source
- the patient signature deadline appears exactly as in the source
- the tone is clearer and less formal than the original
- the meaning has not changed
---
type: Business Rule
status: draft
---
Suggesting test cases for a skill is a model call the user starts, so it is open only to invited accounts, refused before the model is called when the owner's credit balance is below the start threshold for suggest_test_cases or model calls are halted, and debited by the usage the gateway reported once the model answers; it stores nothing, and a proposal whose name the skill already uses is dropped.

# Relations
- Contexts: [testlab](/contexts/testlab)

# Examples
- With no suggest_test_cases statistics the threshold is the 70-credit fallback: a balance of 69 is refused with no model call and nothing charged; a balance of 70 is allowed and a $0.01 answer debits 13 credits and records one suggest_test_cases cost event.
- A proposal named like the skill's existing test case is dropped, and the skill still has only the test cases it had before the call.

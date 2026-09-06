---
name: rfc2119-requirement-classifier
description: When a user pastes requirement sentences and wants them labeled by RFC 2119 modality, classify each sentence as MUST, SHOULD, MAY, or not given and present the results in a table.
---

# RFC 2119 requirement classifier

You label pasted requirement sentences by RFC 2119 modality and present the result as a table.

## What to do
1. Read the user's input as the source text to classify.
2. Split the input into individual requirement sentences in the order they appear.
3. For each sentence, assign a level using the RFC 2119 keyword meanings when the input contains them.
4. If a sentence clearly matches a RFC 2119 keyword, label it with that keyword level.
5. If the sentence does not contain enough information to map it to a level, write `not given`.
6. Output a table with one row per sentence, preserving the original order.
7. Include at least these columns:
   - `sentence`
   - `level`

## Rules for using the input
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access

## Output requirements
- Return the finished table directly.
- Do not explain your reasoning unless the user explicitly asks for it.
- Do not ask follow-up questions when the input already gives enough text to classify.
- If the input is empty or missing, say `not given`.

## RFC 2119 basis
- Use the RFC 2119 keyword definitions available to you when classifying MUST, SHOULD, and MAY.
- If the needed RFC 2119 text is not available in the current context, keep the table format and mark the uncertain classification as `not given` rather than inventing a level.
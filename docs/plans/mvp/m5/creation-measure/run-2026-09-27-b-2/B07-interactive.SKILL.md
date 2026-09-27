---
name: customer-satisfaction-survey-questionnaire
description: Create a customer satisfaction survey questionnaire in Chinese when the user wants a ready-to-use survey with satisfaction, rating, and open-ended questions.
---

# Customer Satisfaction Survey Questionnaire

Create a ready-to-use customer satisfaction survey questionnaire from the user's request.

## Follow these rules

- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to produce

1. Read the user's request and extract the survey purpose, target audience, language, length, topic areas, and any required question types.
2. If the request omits a needed setting, use the common default and state the assumption inside the questionnaire notes or introduction.
3. Draft the questionnaire itself, ready to copy and use.
4. Include a brief title, a short introduction, the questions, answer options where useful, and a closing thank-you line.
5. Keep the wording in the user's language. If the user gives no language, use Chinese as the common default and say that Chinese was used.
6. If the user asks for a specific format such as Google Forms, Typeform, or paper survey, structure the output for that format. If no format is given, use a simple plain-text questionnaire.
7. Include both quantitative satisfaction questions and at least one open-ended feedback question when the request is for customer satisfaction.
8. If the request gives a length limit, stay within it.
9. If the request gives multiple required themes or sections, cover each one directly.
10. If any requested detail is missing, write 'not given' only for that missing fact and continue.

## Output shape

Return only the questionnaire content the user can use immediately.

Use this default structure when no tighter format is given:

- Title
- Short intro
- Response scale or instructions
- Questions
- Closing thank-you

## Quality check before finalizing

- The output is a complete questionnaire, not an explanation.
- The output uses the requested language or the common default.
- The questionnaire includes at least one overall satisfaction question.
- The questionnaire includes at least one quantifiable rating question.
- The questionnaire includes at least one open-ended feedback question.
- Any missing setting was filled with a common default and stated.
- No invented facts were added.

## Example default questionnaire pattern

When the request is general and no industry is given, use a neutral service context and a 5-point satisfaction scale:

- 1 = Very dissatisfied
- 2 = Dissatisfied
- 3 = Neutral
- 4 = Satisfied
- 5 = Very satisfied

Then ask questions such as:

- Overall, how satisfied are you with our service?
- How satisfied are you with the staff's professionalism?
- How satisfied are you with the response speed?
- How satisfied are you with the clarity of the process?
- What do you like most?
- What should we improve?

End with a thank-you note.
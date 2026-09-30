Domain facts:
- Context: Reviews. It owns whether a Review is accepted and how it is shown.
- Terms: Review, Review Text, Rating, Rating Scale, Tag, Excerpt.
- Rule: a Review Text holds at most 2000 characters.
- Rule: a Rating is a whole number on a Rating Scale from 1 to 5.
- Rule: a Review carries at most 12 distinct Tags.
- Rule: an Excerpt shows at most the first 200 characters of the Review Text.

Forces:
- Product expects to change the Review Text limit and the number of Tags.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- Every Review the current code accepts, and every rejection message it gives, is unchanged after the change.

The lint check configured in `pyproject.toml` now fails on `reviews/review.py`:

```text
reviews/review.py:18:20: PLR2004 Magic value used in comparison, consider replacing `2000` with a constant variable
reviews/review.py:20:31: PLR2004 Magic value used in comparison, consider replacing `5` with a constant variable
reviews/review.py:23:23: PLR2004 Magic value used in comparison, consider replacing `12` with a constant variable
reviews/review.py:29:28: PLR2004 Magic value used in comparison, consider replacing `200` with a constant variable
```

Make the check pass. Every Review must be accepted, or rejected with the message, it gets today. Keep `accept_review`, `excerpt`, `Review` and `ReviewRejected` importable from `reviews.review`.

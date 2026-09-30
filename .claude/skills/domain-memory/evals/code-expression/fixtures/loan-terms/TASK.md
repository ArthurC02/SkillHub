The lint check configured in `pyproject.toml` now fails on `library/loans.py`:

```text
library/loans.py:16:31: FBT001 Boolean-typed positional argument in function definition
library/loans.py:24:31: FBT001 Boolean-typed positional argument in function definition
library/loans.py:28:30: FBT001 Boolean-typed positional argument in function definition
```

Make the check pass. Every loan must get the quote, or the refusal, it gets today. Keep `quote_loan`, `LoanQuote` and `LoanRefused` importable from `library.loans`.

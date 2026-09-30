The lint check configured in `pyproject.toml` now fails on `delivery/price.py`:

```text
delivery/price.py:23:5: PLR0913 Too many arguments in function definition (6 > 5)
```

Make the check pass. Every Order must get the price, or the refusal, it gets today. Keep `quote_delivery`, `Order` and `PriceRefused` importable from `delivery.price`, and keep `Order` constructible with the fields it has now.

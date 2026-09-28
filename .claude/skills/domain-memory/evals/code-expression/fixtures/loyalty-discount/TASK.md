Add a loyalty discount to pricing.

- A customer with 10 or more past orders gets 5% off the order total.
- A customer with 25 or more past orders gets 8% off instead.
- Everyone else pays the order total.

Expose it as `shop.pricing.discounted_total(total, past_orders)`, where `total` is a `Decimal` and `past_orders` is an `int`. It returns the amount to pay.

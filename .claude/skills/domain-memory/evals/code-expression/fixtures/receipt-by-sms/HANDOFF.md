Domain facts:
- Context: Orders. It owns the Order and the decision to confirm it.
- Terms: Order, Customer, Receipt. A Receipt tells the Customer that their Order was confirmed and states the Order total.
- Invariant: an Order is confirmed at most once.
- Invariant: a Receipt is issued only for a confirmed Order.

Forces:
- The business has changed its message provider before and expects to change it again.
- A Receipt that could not be delivered must not undo the confirmation.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- A confirmed Order results in one Receipt for that Order's Customer.
- An Order that fails to confirm results in no Receipt.
- A delivery failure leaves the Order confirmed.

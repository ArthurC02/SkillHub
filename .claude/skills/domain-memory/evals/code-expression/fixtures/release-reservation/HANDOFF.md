Domain facts:
- Context: Stock. It owns what is on hand and what is reserved.
- Terms: Stock Item, Reservation, Release. A Release gives a Reservation's units back to what is available.
- Invariant: what is reserved never exceeds what is on hand.

Forces:
- A Release may be delivered more than once for the same Order.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- Releasing the same Order twice gives its units back once.

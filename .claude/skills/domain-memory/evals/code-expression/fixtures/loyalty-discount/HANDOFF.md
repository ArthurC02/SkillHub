Domain facts:
- Context: Pricing. It owns how an Order Total is computed.
- Terms: Order Total, Loyalty Discount, Past Orders. Past Orders is the number of Orders a Customer completed before this one.
- Rule: every amount Pricing returns is rounded to the cent, half a cent up.

Forces:
- None beyond the rule above.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- The Loyalty Discount applies from the stated number of Past Orders and not one Order earlier.

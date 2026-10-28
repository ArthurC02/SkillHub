Domain facts:
- Context: Lending. It owns the Loan Quote for an Item and a Borrower.
- Terms: Item, Borrower, Member, Guest, Loan Terms, Loan Period, Renewal, Late Fee, Late Fee Cap.
- Rule: a Borrower is a Member or a Guest, and each has its own Loan Terms.
- Rule: a reference Item is never lent.

Forces:
- A third kind of Borrower is expected next.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- Every Loan Quote the current code returns is returned unchanged after the change.

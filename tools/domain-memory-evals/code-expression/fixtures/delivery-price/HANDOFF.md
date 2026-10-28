Domain facts:
- Context: Delivery. It owns the Delivery Price of an Order.
- Terms: Order, Address, Home Country, Remote Area, Service, Same-Day Delivery, Base Rate, Heavy Parcel Surcharge.
- Rule: an Order ships to one Address, and an Address without a street or a city cannot be delivered to.
- Rule: Same-Day Delivery stays inside the Home Country.

Forces:
- Returns are priced next, from an Address alone, under the same Address rules.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- Every Delivery Price the current code returns is returned unchanged after the change.

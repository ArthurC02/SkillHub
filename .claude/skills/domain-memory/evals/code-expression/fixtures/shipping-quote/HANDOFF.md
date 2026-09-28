Domain facts:
- Context: Shipping. It owns the Quote for a Shipment.
- Terms: Shipment, Quote, Service, Base Rate, Oversize Surcharge, Oversize Limit, Remote Area Fee, Free Shipping Threshold, Hazardous Goods.
- Rule: a Shipment longer than the Oversize Limit is refused by every Service.
- Rule: Air refuses Hazardous Goods.
- Rule: the Remote Area Fee is a share of the Base Rate and never exceeds its cap.
- Rule: an Order at or above the Free Shipping Threshold ships by Ground with no Base Rate, unless it pays the Oversize Surcharge.

Forces:
- The rules change independently of each other; the next change is expected to be a new Service.

Decision:
- Left to the coding Agent.

Unknowns:
- None that block implementation.

Proof obligations:
- Every Quote the current code returns is returned unchanged after the change.

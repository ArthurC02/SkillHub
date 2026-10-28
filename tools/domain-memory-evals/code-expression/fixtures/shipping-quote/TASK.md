`shipping.quote.quote` has become hard to read and hard to change. Make it easier to read and change.

Its behaviour must not change: every shipment must get the quote, or the refusal, it gets today. Keep `quote`, `Shipment` and `QuoteRefused` importable from `shipping.quote`.

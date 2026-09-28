from dataclasses import dataclass, field


class InsufficientStock(Exception):
    pass


@dataclass
class StockItem:
    sku: str
    on_hand: int
    reserved: dict[str, int] = field(default_factory=dict)

    def available(self) -> int:
        return self.on_hand - sum(self.reserved.values())

    def reserve(self, order_id: str, quantity: int) -> None:
        if quantity > self.available():
            raise InsufficientStock(self.sku)
        self.reserved[order_id] = self.reserved.get(order_id, 0) + quantity

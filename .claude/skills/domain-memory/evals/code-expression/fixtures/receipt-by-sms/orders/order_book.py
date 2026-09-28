from orders.order import Order


class UnknownOrder(Exception):
    pass


class OrderBook:
    def __init__(self) -> None:
        self._orders: dict[str, Order] = {}

    def add(self, order: Order) -> None:
        self._orders[order.order_id] = order

    def get(self, order_id: str) -> Order:
        if order_id not in self._orders:
            raise UnknownOrder(order_id)
        return self._orders[order_id]

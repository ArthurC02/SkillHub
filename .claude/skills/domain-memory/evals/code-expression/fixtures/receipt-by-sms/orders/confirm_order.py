from orders.order import Order
from orders.order_book import OrderBook


def confirm_order(order_id: str, order_book: OrderBook) -> Order:
    order = order_book.get(order_id)
    order.confirm()
    return order

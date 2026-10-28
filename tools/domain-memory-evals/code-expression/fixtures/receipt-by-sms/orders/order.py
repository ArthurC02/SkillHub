from dataclasses import dataclass
from decimal import Decimal


class OrderAlreadyConfirmed(Exception):
    pass


@dataclass
class Order:
    order_id: str
    customer_phone: str
    total: Decimal
    confirmed: bool = False

    def confirm(self) -> None:
        if self.confirmed:
            raise OrderAlreadyConfirmed(self.order_id)
        self.confirmed = True

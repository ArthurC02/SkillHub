When an order is confirmed, the customer must get a receipt by text message.

We send text messages through Textline. Its API is one call:

- `POST https://api.textline.example/v2/messages`
- header `X-Textline-Key`, taken from the environment variable `TEXTLINE_KEY`
- JSON body `{"to": "<phone number>", "body": "<message text>"}`
- `202` means the message was accepted, `429` means we are being rate limited, anything else is a failure

Use the standard library for the HTTP call. The tests must pass with no network.

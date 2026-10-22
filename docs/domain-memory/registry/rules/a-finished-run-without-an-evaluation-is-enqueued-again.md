---
type: Business Rule
status: draft
---
Every run that finished as succeeded or failed is evaluated even when the event or job that triggers its evaluation exhausted its retries: every 30 minutes, runs that finished more than 30 minutes and less than a day ago and have no evaluation record are enqueued for evaluation again, through the same enqueue that ignores a run whose evaluation job is still live.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A succeeded run whose run-succeeded event was dead-lettered has no evaluation; the next sweep enqueues it. A cancelled run, a run that finished ten minutes ago, a run that finished two days ago and a run that already has an evaluation are left alone.

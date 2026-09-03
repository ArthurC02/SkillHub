---
paths:
  - "contracts/**"
  - "db/migrations/**"
  - "db/queries/**"
  - "db/query-owners.yaml"
  - "Taskfile.yml"
  - ".github/workflows/**"
---

**這是高衝突區，由主 Agent 序列化處理**（根 `AGENTS.md` 開發自動化第 4 條）。你如果是子代理，停下來回報，不要自己改。

generated 目錄一律不手改：來源改完由主 Agent 跑 `task gen:sql` / `task gen:openapi`，提交前跑 `task gen:check`。

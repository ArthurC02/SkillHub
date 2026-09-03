---
paths:
  - "docs/plans/**"
  - "docs/adr/**"
---

改這些文件前先看根 `AGENTS.md` 的「文件維護規則」。三個最常漏的機器閘門，漏了 CI 會紅：

- 改 `04` 的殘項數字 → 同一格的 `<!-- open: … -->` 要一起改（`backlog-tally`）
- ADR 不原地改寫決策 → 新增 ADR 並把舊的標 `Superseded`
- 任何 markdown 連結 → 目標檔案必須存在（`doc-links`）

改完跑 `go -C tools/devctl run . automation-check`。

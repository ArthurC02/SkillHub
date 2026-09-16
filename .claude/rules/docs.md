---
paths:
  - "docs/**"
---

改這些文件前先看根 `AGENTS.md`，再看 [`docs/AGENTS.md`](../../docs/AGENTS.md) 的「文件維護規則」。四個最常漏的機器閘門，漏了 CI 會紅：

- 改 `04` 的殘項數字 → 同一格的 `<!-- open: … -->` 要一起改（`backlog-tally`）
- ADR 的決策變了 → 直接改寫那個主題 ADR（全文只寫現行版），理由寫進 commit message
- 任何 markdown 連結 → 目標檔案必須存在（`doc-links`）
- 只有 ADR 與索引寫 ADR 編號或檔名 → 其他地方寫規則本身，要理由就連索引的主題段落（`adr-citations`）

改完跑 `go -C tools/devctl run . automation-check`。

有機器對帳的文件（`04` 的 tally、context map、設計兩把尺）改完要回報會被哪個檢查擋；`docs/plans/mvp/mX/` 是里程碑當時的證據，不回溯修正（這條只管那些文件，不限制程式修改）。

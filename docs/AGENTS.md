# 文件區 Coding Agent 指示

動 `docs/` 任何文件前，先讀根目錄 [`AGENTS.md`](../AGENTS.md)，再讀本檔；若有更深層 `AGENTS.md` 或 `AGENTS.override.md`，也要讀目標檔案所在父目錄的指示，並檢查 override 與基礎指示的差異／衝突。工具不保證會自動按檔案載入這些指示，不得假設已載入；區域指示不得放寬根共同安全規則，衝突時停下回報。

## 文件維護規則

- 三份 MVP 文件（目標／規格／工作清單）改範圍時必須同步；規格新功能先補需求 ID 與允收準則。
- 改 `04` 的殘項數字時，同一格末尾的 `<!-- open: … -->` 要一起改（`backlog-tally` 會對帳）。
- `- [ ]` → `- [x]` 只在完全符合允收準則時；部分完成保持未勾。
- ADR 是決策歷史：推翻＝新增 ADR 並把舊的標 `Superseded`，不刪除、不原地改寫；下一號＝[索引](adr/README.md)最大號 + 1，新增後更新索引。
- 活文件放 `docs/plans/` 根層；里程碑產出放 `docs/plans/mvp/mX/`，完結即凍結，不回溯修正（含 `03` 的歷史 flat path）。
- 里程碑目錄固定骨架：`README.md`（計畫＋狀態＋檔案地圖）、`audit.md`、報告用 `report-*` 前綴；目錄內檔名不重複 `mX` 前綴（M3 起適用，既有檔名不回溯改）。

`.claude/rules/docs.md` 是文件路徑的提早提示；它不取代本檔。文件區的權威規格仍是 `docs/plans/`、`docs/adr/` 與 `docs/design/` 各自的文件。

# 文件區 Coding Agent 指示

動 `docs/` 任何文件前，先讀根目錄 [`AGENTS.md`](../AGENTS.md)，再讀本檔；若有更深層 `AGENTS.md` 或 `AGENTS.override.md`，也要讀目標檔案所在父目錄的指示，並檢查 override 與基礎指示的差異／衝突。工具不保證會自動按檔案載入這些指示，不得假設已載入；區域指示不得放寬根共同安全規則，衝突時停下回報。

## 文件維護規則

- 活文件只寫現行規則、操作方式、已知限制與證據入口；不留施工日誌、逐次修正經過、已失效排程或無意義日期。技術格式常數、版本識別、保存期限、法律必要日期與原始量測證據不在此限。
- 三份 MVP 文件（目標／規格／工作清單）改範圍時必須同步；規格新功能先補需求 ID 與允收準則。
- 改 `04` 的殘項數字時，同一格末尾的 `<!-- open: … -->` 要一起改（`backlog-tally` 會對帳）。
- `- [ ]` → `- [x]` 只在完全符合允收準則時；部分完成保持未勾。
- ADR 一個主題一份，內容永遠是現行版本（[ADR 管理](adr/README.md#adr-管理)）：決策變了就直接改寫那一段，理由與經過寫進 commit message，歷史由 git 保存；不留補記、日期與刪除線。出現新主題才新增 ADR，編號＝[索引](adr/README.md)最大號 + 1，刪除過的編號不再用；新增、改名、拆分或合併都同批更新索引。
- 只有 ADR 與索引寫 ADR 編號或檔名。其他文件、程式、測試、設定、契約寫規則本身；需要理由時連索引的主題段落（`adr/README.md#<主題>`）。里程碑的機器輸出（transcript、probe、log）、量測結果與第三方語料照原樣保存。`docs/domain-memory/` 的 `.json` 與 `.jsonl` 記錄可以直接引用 ADR 檔：記錄的引用帶內容摘要，被引的那一行一改 `verify-evidence` 就報 stale，稽核鏈則是只增不改的雜湊鏈，兩者都不會像散落的編號那樣默默過期；同目錄的 Markdown 不在此列。`adr-citations` 會擋。
- 活文件放 `docs/plans/` 根層；里程碑產出放 `docs/plans/mvp/mX/`，完結後是當時的紀錄，不回溯修正（含 `03` 的歷史 flat path）。
- 里程碑目錄固定骨架：`README.md`（計畫＋狀態＋檔案地圖）、`audit.md`、報告用 `report-*` 前綴；目錄內檔名不重複 `mX` 前綴（M3 起適用，既有檔名不回溯改）。

## 文件分工

| 要回答的問題 | 唯一維護位置 |
| --- | --- |
| 產品目標、範圍與曝光邊界 | `plans/01-goals-and-plan.md` |
| 必須做到什麼、如何允收 | `plans/02-specifications-and-acceptance-criteria.md` |
| 工作項目與交付狀態 | `plans/03-work-items.md` |
| 目前缺口與依賴 | `plans/04-backlog-and-handoffs.md` |
| 待裁定事項與結論索引 | `plans/05-pending-rulings.md` |
| 架構規則與理由 | `adr/` |
| 畫面與跨頁體驗契約 | `design/` |
| 開發操作與維運程序 | `development/`、`runbooks/` |
| 當時的驗收與量測證據 | `plans/mvp/`；不得當成現行操作或完成狀態 |

其他文件以連結引用，不重複維護狀態、版本與計數。規格與程式不同時，分列要求、目前行為與缺口，不以改寫規格代替修正程式，也不把排程、測試存在或本機成功當作真人／部署驗收。

`.claude/rules/docs.md` 是文件路徑的提早提示；它不取代本檔。文件區的權威規格仍是 `docs/plans/`、`docs/adr/` 與 `docs/design/` 各自的文件。

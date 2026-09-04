# apps/web — 動手前的指標

**這個檔只放指標，不複製規則內容、不抄任何數字**：與被指的那一份不一致時，一律以被指的那一份為準。<br>
存在的理由：這個目錄原本沒有任何 markdown，所以編輯前端時，除了 repo 根的 `AGENTS.md` 以外**沒有任何東西會自動進入 context**——而前端的兩把尺（`docs/design/`）加起來有十六萬字，不會有人「順便讀完」。

## 改畫面之前先讀哪一段

| 你要做的事                                            | 先讀                                                                                                                                                          | 沒讀會被哪個閘門擋                                                                                                                                                                                                              |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 版面、字級、間距、表面、狀態語彙、停用要不要說原因    | [system.md](../../docs/design/system.md) §0 優先序 → §3 逐頁 checklist                                                                                        | `src/design-system.test.ts`（它**直接解析** system.md 的粗體數值與豁免名冊，改文件不改程式會紅，反之亦然）                                                                                                                      |
| 熱路徑的文字量、摺疊、Tip、icon                       | system.md §2.13、§4.7 與 [ADR-065](../../docs/adr/ADR-065-hot-path-text-budget-and-the-fourth-disclosure-mechanism.md)                                        | `src/a11y.test.tsx`（但書守衛、清單重複句 ratchet、每頁 ≤3 Tip、icon 旁有字）、`src/design-system.test.ts`（`<svg` 站點 ≤6）、`src/detail.test.tsx`（§2.10 不進 Tip）、`e2e/rendered.spec.ts`（D 類字數棘輪、Tip 開合不推鄰居） |
| 永不可摺疊、不得只放進 `title`／hover 的內容          | system.md §2.10 的封閉清單                                                                                                                                    | `src/a11y.test.tsx`、`e2e/rendered.spec.ts`                                                                                                                                                                                     |
| 失敗與缺席的句子：讀取或寫入失敗、401、伺服器回的字串 | [information-architecture.md](../../docs/design/information-architecture.md) §5 IA-6（401 由 `ReadFailure` 說一次，寫入也算）、system.md §2.9（缺席的六個詞） | `src/design-system.test.ts`（IA-6 棘輪：含「失敗」的 alert 附近要有 `ReadFailure`）、`src/a11y.test.tsx`（`role=alert`／`status` 裡沒有任何中文字即 FAIL；fixture 要抄 Go 真的回的字串）                                        |
| 新網址、導覽多一項、入口藏在旗標後面                  | [information-architecture.md](../../docs/design/information-architecture.md) §0（**§0 的規則走在程式前面**：不一致是改程式，不是改文件）                      | `src/ia.test.ts`                                                                                                                                                                                                                |
| 顏色、對比、視覺層級                                  | system.md §5 與 [ADR-064](../../docs/adr/ADR-064-the-visual-layer-is-hierarchy-carried-by-tokens.md)                                                          | `src/contrast.test.ts`                                                                                                                                                                                                          |

## 三個會咬人的地方

- **IDE diagnostics 經常是 stale 的**（多人平行編輯時尤其），判準是實跑 `npm run typecheck`。
- **`prettier --check` 曾經 ubuntu 紅而 windows 綠**（2026-09-03 的 CI）。CI 信的是 Linux；push 前跑 `npm run format:check`，而且判準是**有沒有出現** `All matched files use Prettier code style!` 那一行——命令輸出會被過濾，「沒看到抱怨」不等於通過。
- ⛔ **M5 的生成入口不得對封測使用者出現，也不得變得更顯眼**。邊界原文在 [`01` §10](../../docs/plans/01-goals-and-plan.md)，不要憑記憶判斷它是否已放行。
- **凍結狀態、逐次放行紀錄、閘門期間「一律不動」的條款，都只在 [`01` §10](../../docs/plans/01-goals-and-plan.md) 的裁定表**。動手前先確認今天是否落在閘門期間、這次的改動屬於哪一次放行；本檔不抄日期，因為日期會過期而這裡不會跟著改。

## 哪些檔案不屬於單一寫入者

跨頁守衛測試（`src/design-system.test.ts`、`src/ia.test.ts`、`src/a11y.test.tsx`、`src/contrast.test.ts`、`e2e/rendered.spec.ts`）與 `package-lock.json` 屬於 coordinator（主 Agent）。子代理需要改它們就停下來回報，不自己動——它們是所有頁面共用的尺，兩個寫入者同時改一把尺就沒有尺了。

**這個檔是給所有 coding agent 讀的**（Claude Code 讀不到 `AGENTS.md`，所以同目錄的 `CLAUDE.md` 只有一行 `@AGENTS.md` 把它 import 進來）。內容只寫在這一份，不複製。

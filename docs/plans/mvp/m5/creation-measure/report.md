# 互動創作 15 場 ＋ 單次 15 題：2026-09-06 的三次實跑

負責人「可以花錢」後，`TestCreationMeasureFifteenSessionsAgainstSingleShot` 對真實閘道（`gpt-5.4-mini`，經 LiteLLM，Virtual Key 限額）跑了三次。每次的 `results.json`、30 份 `SKILL.md`（run b 起另有 15 份逐場對話）在 `run-2026-09-06-a/`、`-b/`、`-c/`。**三次不是重複量測，是修一次跑一次**：a 量出問題、b 的修法製造了新問題、c 是現在的程式。門檻取 [`05` R-45](../../../05-pending-rulings.md)。

## 1. 三次的數字

| | run a（`d124f05` 的程式） | run b（提示禁止重提 brief） | **run c（＋草稿缺席改成澄清）** | R-45 門檻 |
| --- | --- | --- | --- | --- |
| 有通過驗證的草稿 | 12／15 | 4／15 | **15／15** | ≥ 14／15 |
| 走到候選＋Test Case | 12／15 | 4／15 | **13／15** | — |
| 每場成本中位／最大 | $0.023／$0.041 | $0.008／$0.021 | **$0.018／$0.031** | 中位 ≤ $0.50 |
| 每次模型呼叫 p50／p95 | 4.1 s／6.1 s | 4.1 s／6.6 s | **5.0 s／7.0 s** | ≤ 60 s／≤ 90 s |
| 模型呼叫總數 | 64 | 39 | 53 | — |
| 三次合計成本 | $0.35 | $0.15 | $0.29 | — |
| 單次對照（同 15 題） | 15／15，中位 $0.0045 | 15／15，$0.0045 | 15／15，$0.0047 | — |

每一場的驗收條件數 4～7 條（模型隨 brief 提出、人確認）；c 的 13 場候選都在 materialize 的同一交易建了 Test Case。

## 2. 三次之間修了什麼（也是這次量測真正的產出）

- **run a → 3 場卡在確認迴圈**（R02、R08、R10）：使用者確認 brief 之後，模型下一步又回 `confirm_brief`——harness 的「一律同意」再確認、模型再重提，直到 12 輪用完，草稿一份都沒有。修法兩層：compose 相的提示明說「brief_confirmed 為真就交草稿或請 Go 驗證，不要再提 brief」；Go 的 `proposal()` 對「已確認且內容未變的 brief 再被要求確認」直接把回合交還給人（`TestProposalKeepsAConfirmedBriefWhenTheModelMerelyRestatesIt`）。
- **run b → 11 場在確認後的下一步直接失敗**：提示把模型推向 `outcome=draft`，mini 模型交了 `draft: null`，Python 的 `_draft` 對此回 502「unusable draft」，Go 把整場判成 `failed`。逐場對話存檔（run b 起才有）才看得出來；修法：`draft` 缺席改成帶 `reason=draft_missing` 的澄清（契約 enum、Go 句子、Python 測試同批），提示補一句「outcome draft 必須附完整草稿物件」。
- **run c → 15／15 有草稿**；D01、D05 拿到草稿後在「請人補充」與確認之間多走了幾輪、12 輪用完時停在 `queued`，不是失敗。

## 3. 這三次量到什麼、沒量到什麼

量到：格式（Go `skillpkg` 驗證不阻擋）、成本、等待、每場幾次呼叫、驗收條件有沒有變成資料、Test Case 有沒有建。**沒量到**：`met`（要把候選接上 Run 跑評估）、`kept`（要一個人讀 30 份 `SKILL.md`）——`results.json` 每列的 `met_by_owner`／`kept_by_owner` 留空。R-45 的六個門檻裡機器能判的三個（格式、成本、等待）在 run c 全過，人要判的兩個還沒有數字。

另外要記：harness 的「使用者」是一個對每個提案都說好、最多補兩句通用話的假人。它量的是「流程會不會卡、會不會爆錢、會不會慢」，不是「真人會不會滿意」。

## 4. 同一天單次路徑的 20＋20

見 [report-generate-modes.md §8](../report-generate-modes.md)：流程圖 20／20 生成、19／20 節點全讀到；參考 20／20、標記句 0／60 被抄。

# tools/eval-regression

## Run 選取、dry-run 與證據檔

`judge_regression.py` 有兩種明確且互斥的 Run 選取方式：不帶 `--run-id` 時，依
`skill_runtime_compatibility` 取每個 Skill Version 最新的 M2 compatibility baseline；重複
`--run-id UUID` 時，直接讀取指定 Run、其 Skill Version、Skill 與 Test Case Snapshot，完全
不依賴 compatibility 表。指定的每個 id 都必須存在且具備這些關聯，否則立即失敗；不會安靜少跑。
Fork Run 的 rubric 以來源 Skill 名稱選取，但送往 Judge 的 Skill 名稱仍是該 Run 實際使用的 Skill。

`--dry-run` 是 **live read**：它會讀 PostgreSQL 與 S3，組出完整請求；它不呼叫 Judge／模型、
不產生模型費用、也不寫入任何結果檔。非 dry-run 的結果只可 append 到 `results.jsonl`；舊列是
可比對的歷史證據，不可覆寫或重排。

`EVAL-013` 的 Judge 判準回歸。**權威資料在 DB 與物件儲存，此處為 harness 與證據快照。**

| 檔案 | 內容 |
| --- | --- |
| [`judge_regression.py`](judge_regression.py) | 把 M2 的 45 筆基準 Run 重新餵過 `apps/llm` 的 `/judge-run`，逐條比對期望答案。集合定義、ground truth 轉換規則、Go 側證據回驗的鏡像實作都在檔頭與各函式的 docstring |
| `results.jsonl` | 每次回歸每個 Run 一列，**append-only**。每列自帶 `judge_model`／`judge_prompt_version`／`rubric_version`／截斷設定——換其中任一項就是另一次回歸，兩份結論必須並存可比（`02:EVAL-013` 第 4 條、[評估判定與 Judge 信任邊界](../../docs/adr/README.md#評估判定與-judge-信任邊界)）。**不要覆寫它** |
| [`test_judge_regression.py`](test_judge_regression.py) | `judge_regression.py` 鏡像[評估判定與 Judge 信任邊界](../../docs/adr/README.md#評估判定與-judge-信任邊界)那四項行為的測試：引文內容回驗與改判、NFC ＋空白摺疊 ＋十二字元門檻、`exact`／`normalized`／`not_checked` 三態、`artifact` 引用不得滿足 `evidence_required`。**不呼叫模型、不讀 DB**：`python tools/eval-regression/test_judge_regression.py` 即可跑，也可被 pytest 收集。四項各自被還原過一次並確認變紅（AGENTS.md 規則 9）。**它進 CI**（`ci.yml` 的 `contracts-drift` job）與 `task test`：下面那句「不進 CI」的理由（要 DB、要 S3、要真的付錢）只對 `judge_regression.py` 本體成立。把一支不到一秒的純函式測試綁在它所守護的昂貴東西的規則上，換來的是[評估判定與 Judge 信任邊界](../../docs/adr/README.md#評估判定與-judge-信任邊界)的四條行為沒有任何機器在守 |
| [`injection_regression.py`](injection_regression.py) | 注入抵抗回歸（`02:EVAL-013` 報告 §2 第二格）。樣本是合成的、沒有 `run_id`，所以是另一個進入點而不是 `--flag`；`verify()` 與 `store()` **import 自 `judge_regression.py`**，Go 側回驗的鏡像實作只留一份 |
| [`injection-samples-v1.json`](injection-samples-v1.json) | 上者的輸入，`sample_set_version = injection/v1`：13 個樣本、27 條判定，涵蓋[評估判定與 Judge 信任邊界](../../docs/adr/README.md#評估判定與-judge-信任邊界)的 Judge 四條防線各自的攻擊面，含一個對照組。**期望答案由樣本自身寫明的事實推出，樣本檔即標註**；攻擊者想要的答案另存 `attacker_wants`，所以「判錯」與「被說服」數得開 |
| `injection-results.jsonl` | 注入回歸的逐樣本結果，**append-only，與 `results.jsonl` 分開**（報告 §8.2 建議 2 要求兩者分開統計）。每列另帶 `sample_set_version`——**換樣本集也是另一次回歸** |
| [`judge-adversarial-samples-v1.json`](judge-adversarial-samples-v1.json) | 互動創作對抗量測裡被懷疑判錯的案例，`sample_set_version = judge-adversarial/v1`：輸入、Skill 的最後輸出與條件逐字取自存下的試跑紀錄；`attacker_wants` 放的是當時判錯的答案，沒有它的樣本是對照組。用 `injection_regression.py --samples judge-adversarial-samples-v2.json --out judge-adversarial-results.jsonl` 跑 |
| [`judge-adversarial-samples-v2.json`](judge-adversarial-samples-v2.json) | v1 加上 `ja-05`：由量測 harness 存下的一份完整判定請求（`judge-<run_id>.json`）逐欄重建。那一場的輸出正確，判定模型把兩段不相鄰的文字接成一句引文，平台回驗找不到而退成「無法判定」；這一列的 `attacker_wants` 是那次平台降級，不是模型的判定 |
| `judge-adversarial-results.jsonl` | 上者的逐樣本結果，**append-only**，與另外兩份結果分開。帶 `--model-role <閘道角色>` 時由那個角色判定，每列記下 `model_role`；評審團的三個成員各跑一次、逐條多數決，就是單一 Judge 與評審團的對照 |
| [`creation_frozen_compare.py`](creation_frozen_compare.py) | 互動創作兩個版本在[凍結測試集](../../docs/plans/mvp/m5/creation-measure/frozen/README.md)上的比較：讀量測 harness 存下的 `<task>-holdout-<n>-trial-r0.json`，算評審通過率與程式比對 `expected_numbers` 的通過率（Wilson 區間、分類別），兩版時再算以任務為群的成對 bootstrap 差值與兩邊各自獨有的通過數。**只印彙總**，不印任何案例的輸出或評審理由——凍結集的失敗細節不讀。測試 [`test_creation_frozen_compare.py`](test_creation_frozen_compare.py) 是純函式，進 CI 與 `task test:judge-rubric` |
| [`decisions_probe.py`](decisions_probe.py) | 把判錯樣本集的每條條件（三選一判定）與注入樣本集加判錯樣本集的輸出（「有沒有對評審下指令」）送進閘道的 `/openai/v1/decisions` pass-through，每題重複 `--reps` 次。要一把限定 `gpt-6-luna` 的虛擬金鑰；這條路由在閘道記帳為 0 元，花費不進每日上限 |
| `decisions-probe-results.jsonl` | 上者的逐題結果，**append-only**：每列記下題型、期望答案、每次的完整回答、輸入 token 與延遲 |
| [`rubric-content-007-writing-v1.json`](rubric-content-007-writing-v1.json) | `--rubric` 的輸入：`CONTENT-007` 五個 `writing` 精選的預設 rubric，逐字取自 [`docs/plans/mvp/content/writing-rubrics.md`](../../docs/plans/mvp/content/writing-rubrics.md) §4。**每個 item 的 `id` 是它加強的那條驗收條件的 id**（harness 會擋下對不上的檔案）。帶 `--rubric` 時回歸集縮到該檔涵蓋的 Skill、快照原本的條件照舊送出、`rubric_version` 不再是 `null`——**換 rubric 版本就是另一次回歸** |

方法、逐筆結果、差異歸因與結論見 [`docs/plans/mvp/m3/report-judge-regression.md`](../../docs/plans/mvp/m3/report-judge-regression.md)；重跑指令見該報告 §10，帶 rubric 的那一輪見 §11，注入抵抗那一輪見 §12。

現行量測見 [Judge 回歸與重複性報告](../../docs/plans/mvp/m6/report-judge-regression-2026-09-26.md)：45 筆原始基準、5 筆各重複 5 次、證據缺漏與無效引用，附原始請求／回應及費用。工具依現行平台對單一 excerpt 截斷採引用來源局部降級；缺引用的判定不可採信；模型自身的 `undetermined` 與判錯分開統計。採樣欄位記錄的是 requested 參數，不保證供應商支援或結果確定。

與 `tools/goldenset` 同一種東西：**驗證工具，不是產品程式碼**——不被服務引用，重跑要花真實的模型費用（**回歸本體不進 CI；上表的 `test_judge_regression.py` 例外，它不花錢**）（45 筆全量約 $0.72，帶 rubric 的 5 筆約 $0.14，注入 13 樣本約 $0.05）。

`injection_regression.py --dry-run` 不呼叫模型也不花錢，並會跑樣本集的 self-check（重複 id、缺對照組、攻擊要的答案剛好等於誠實答案）——改樣本檔之後先跑它。

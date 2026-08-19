# tools/eval-regression

`EVAL-013` 的 Judge 判準回歸。**權威資料在 DB 與物件儲存，此處為 harness 與證據快照。**

| 檔案 | 內容 |
| --- | --- |
| [`judge_regression.py`](judge_regression.py) | 把 M2 的 45 筆基準 Run 重新餵過 `apps/llm` 的 `/judge-run`，逐條比對期望答案。集合定義、ground truth 轉換規則、Go 側證據回驗的鏡像實作都在檔頭與各函式的 docstring |
| `results.jsonl` | 每次回歸每個 Run 一列，**append-only**。每列自帶 `judge_model`／`judge_prompt_version`／`rubric_version`／截斷設定——換其中任一項就是另一次回歸，兩份結論必須並存可比（`02:EVAL-013` 第 4 條、[ADR-026](../../docs/adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 1）。**不要覆寫它** |
| [`injection_regression.py`](injection_regression.py) | 注入抵抗回歸（`02:EVAL-013` 報告 §2 第二格）。樣本是合成的、沒有 `run_id`，所以是另一個進入點而不是 `--flag`；`verify()` 與 `store()` **import 自 `judge_regression.py`**，Go 側回驗的鏡像實作只留一份 |
| [`injection-samples-v1.json`](injection-samples-v1.json) | 上者的輸入，`sample_set_version = injection/v1`：13 個樣本、27 條判定，涵蓋 ADR-026 決策 3 四條防線各自的攻擊面，含一個對照組。**期望答案由樣本自身寫明的事實推出，樣本檔即標註**；攻擊者想要的答案另存 `attacker_wants`，所以「判錯」與「被說服」數得開 |
| `injection-results.jsonl` | 注入回歸的逐樣本結果，**append-only，與 `results.jsonl` 分開**（報告 §8.2 建議 2 要求兩者分開統計）。每列另帶 `sample_set_version`——**換樣本集也是另一次回歸** |
| [`rubric-content-007-writing-v1.json`](rubric-content-007-writing-v1.json) | `--rubric` 的輸入：`CONTENT-007` 五個 `writing` 精選的預設 rubric，逐字取自 [`docs/plans/mvp/content/writing-rubrics.md`](../../docs/plans/mvp/content/writing-rubrics.md) §4。**每個 item 的 `id` 是它加強的那條驗收條件的 id**（harness 會擋下對不上的檔案）。帶 `--rubric` 時回歸集縮到該檔涵蓋的 Skill、快照原本的條件照舊送出、`rubric_version` 不再是 `null`——**換 rubric 版本就是另一次回歸** |

方法、逐筆結果、差異歸因與結論見 [`docs/plans/mvp/m3/report-judge-regression.md`](../../docs/plans/mvp/m3/report-judge-regression.md)；重跑指令見該報告 §10，帶 rubric 的那一輪見 §11，注入抵抗那一輪見 §12。

與 `tools/goldenset` 同一種東西：**驗證工具，不是產品程式碼**——不進 CI、不被服務引用，重跑要花真實的模型費用（45 筆全量約 $0.72，帶 rubric 的 5 筆約 $0.14，注入 13 樣本約 $0.05）。

`injection_regression.py --dry-run` 不呼叫模型也不花錢，並會跑樣本集的 self-check（重複 id、缺對照組、攻擊要的答案剛好等於誠實答案）——改樣本檔之後先跑它。

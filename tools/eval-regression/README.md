# tools/eval-regression

`EVAL-013` 的 Judge 判準回歸。**權威資料在 DB 與物件儲存，此處為 harness 與證據快照。**

| 檔案 | 內容 |
| --- | --- |
| [`judge_regression.py`](judge_regression.py) | 把 M2 的 45 筆基準 Run 重新餵過 `services/llm` 的 `/judge-run`，逐條比對期望答案。集合定義、ground truth 轉換規則、Go 側證據回驗的鏡像實作都在檔頭與各函式的 docstring |
| `results.jsonl` | 每次回歸每個 Run 一列，**append-only**。每列自帶 `judge_model`／`judge_prompt_version`／`rubric_version`／截斷設定——換其中任一項就是另一次回歸，兩份結論必須並存可比（`02:EVAL-013` 第 4 條、[ADR-026](../../docs/adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 1）。**不要覆寫它** |
| [`rubric-content-007-writing-v1.json`](rubric-content-007-writing-v1.json) | `--rubric` 的輸入：`CONTENT-007` 五個 `writing` 精選的預設 rubric，逐字取自 [`docs/plans/mvp/content/writing-rubrics.md`](../../docs/plans/mvp/content/writing-rubrics.md) §4。**每個 item 的 `id` 是它加強的那條驗收條件的 id**（harness 會擋下對不上的檔案）。帶 `--rubric` 時回歸集縮到該檔涵蓋的 Skill、快照原本的條件照舊送出、`rubric_version` 不再是 `null`——**換 rubric 版本就是另一次回歸** |

方法、逐筆結果、差異歸因與結論見 [`docs/plans/mvp/m3/report-judge-regression.md`](../../docs/plans/mvp/m3/report-judge-regression.md)；重跑指令見該報告 §10，帶 rubric 的那一輪見 §11。

與 `tools/goldenset` 同一種東西：**驗證工具，不是產品程式碼**——不進 CI、不被服務引用，重跑要花真實的模型費用（全量約 $0.72）。

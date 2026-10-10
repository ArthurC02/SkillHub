# Skill 能力評估集

驗證新生成的 Skill 能做到什麼、哪裡還能精進的共同尺；規則見 [`02:EVAL-014`](../../../docs/plans/02-specifications-and-acceptance-criteria.md)，依據見[評估判定與 Judge 信任邊界](../../../docs/adr/README.md#評估判定與-judge-信任邊界)。

## 任務的形狀

`tasks-vN.json` 與量測 harness 的語料同形（`reference[]` 的 `id`、`description`、`description_keys`、`holdout[]`），另加：

| 欄位 | 意思 |
| --- | --- |
| `origin` | 這個任務從哪一次失敗來 |
| `layer` | `capability`（還做不好，用來看空間）或 `regression`（穩定通過，掉分就是退步） |
| `author_facts` | 作者知道但第一句話沒講的規則；創作代理追問時，模擬作者照這些回答 |
| `holdout[].enrichment` | 正確的補資訊方式：`compute`、`reference`、`search`、`ask`、`assume`、`flag_invalid` |
| `holdout[].user_facts` | 只在 `ask` 案例：使用者被問到時回答的事實 |
| `holdout[].reference_answer` | 參考解答，證明這題可解 |
| `holdout[].expected_numbers` | 任何正確答案都會出現的數字或日期，供程式判分 |

## 規則

- 新任務只從**讀過逐場紀錄、確認錯在 Skill 而不是考題或評分**的失敗寫成，`origin` 寫明來源。
- 同一種行為要有正反兩面：有該追問的，也要有不該追問的。
- 條件不得獎勵「沒有反問」；該問的案例，第一條條件是「在給出依賴它的結果之前先問了那個事實」。
- 每題由出題以外的代理逐題重算過答案才收錄。
- 任務寫錯了就修並升版本號（`tasks-v2.json`），舊版本的分數只和舊版本比。
- 穩定通過（每次都成功）的任務升進 `regression` 層。

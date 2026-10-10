# 互動創作的凍結測試集

**已停用。** 部分案例的條件把「沒有反問使用者」當作正確答案，與 Skill 應該補上 Context（該問就問）的方向相反；任務達成改依 [`02:EVAL-014`](../../../../02-specifications-and-acceptance-criteria.md) 的能力評估集。檔案照原樣保存。

這個目錄是互動創作各版本共用的固定考題：四份檔，各 9 個任務 × 3 個案例，共 108 個案例。

| 檔案 | 類別 |
| --- | --- |
| [test-set-money.json](test-set-money.json) | 金錢計費 |
| [test-set-time.json](test-set-time.json) | 日期與時間 |
| [test-set-text.json](test-set-text.json) | 有上限的文字產出 |
| [test-set-judge.json](test-set-judge.json) | 判定與換算 |

每個任務的三個案例依序是：一般情況、邊界或模糊說法、陷阱值（不可能或互相矛盾的值混在有效值之間）。`expected_numbers` 只列任何正確答案都必然出現的數字或日期核心值，供程式評分；其餘由評審模型依 `criteria` 判定。

## 規則

- **不修改**。考題一改，先前所有版本的分數就不能和之後的比。發現考題本身有錯，要另開新版本的測試集，並在新版本上重跑所有要比較的版本。
- **不讀失敗細節**。量測後只看分數與分類統計，不讀個別案例的評審理由或 Skill 輸出；要做錯誤分析，用 `corpus-holdout-*.json` 的開發集。
- **成對比較**。兩個版本都跑同一份測試集、同一個評審，以逐案例差值比較，並附信賴區間。新版本要算「變強」，評審通過數至少多 10 個案例（約 9 個百分點），而且以任務為群的成對 bootstrap 95% 區間下界大於 0；兩者缺一就算沒有差。比較用 [`tools/eval-regression/creation_frozen_compare.py`](../../../../../../tools/eval-regression/creation_frozen_compare.py)。
- 出題與審查都由和受測作者不同的模型完成，每份檔都經過第二個代理重算與放寬。

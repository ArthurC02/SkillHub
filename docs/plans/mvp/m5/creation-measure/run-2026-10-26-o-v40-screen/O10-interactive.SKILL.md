---
name: baggage-fee-checker
description: 依單件托運行李重量判定是否可托運、處理方式與費用；當使用者提供行李重量並要求計算航空托運費用時使用。
---

# 航空托運行李費用判定

依使用者提供的單件行李重量，判定托運處理方式與費用。計算規則由 `scripts/baggage_fee.py` 執行。

## 步驟

1. 找出每件行李的重量及其識別名稱；沒有名稱時按輸入順序區分。保留使用者寫下的事實，不補造資料。
2. 比對同一件行李的所有重量。若有互相矛盾的數值，不替該件選一個重量，也不判定處理方式或費用；在結果中指出衝突的原值，請提出者確認。其餘可獨立判定的行李照常處理。
3. 對沒有衝突的重量，依 `scripts/input_schema.json` 使用平台提供的檢查步驟，將各重量原樣交給檢查器。只使用檢查器確認後的值；照原樣呈現它列出的每項讀法或換算假設。中文數字等已由檢查器讀出的重量，不要自行再解析。
4. 將每個已檢查的重量各作為一個 `--weight` 參數，從存放本檔的目錄執行：`python scripts/baggage_fee.py --weight <已檢查重量> --weight <已檢查重量>`。只放入已檢查且沒有重量衝突的行李；有幾件就提供幾個參數。依程式輸出逐件回覆處理方式及費用（元），不要另行計算或改寫金額。
5. 重量大於 0 且不超過 20 公斤為可托運、免費；超過 20 且不超過 32 公斤為可托運的超重行李；超過 32 公斤不接受托運。重量小於或等於 0 時提示重量須大於 0。後三種情況的費用依程式結果呈現；不接受托運及無效重量均為 0 元。
6. 若輸入值本身不合法或同一件行李出現互相矛盾的值，指出每個有問題的原值，請提出者確認；不得給出依賴該值的費用。不得因某件行李有問題而省略其他可判定項目。最後交付逐件判定結果；若需提出確認，將其列在結果中，不以問題句作結。

## Input check (first step)

Before any other step, write every value the message gives into `input.json` as `{"records": [{<field name>: <the value exactly as written>}, ...]}`, one record per item, with the field names of `scripts/input_schema.json`, then run
`python scripts/check_input.py input.json`
from the directory holding this SKILL.md, adding `--today YYYY-MM-DD` when the message states today's date. Every later step and script takes its values only from `checked.json`. Copy every line it prints that starts with 假設：, 無法採用： or 未提供： into the answer word for word; compute nothing from a value it refused, and still give every result the other values settle.

# Credit 精確數值傳輸候選方案

## Business intent

點數是使用者、operator 與 API client 會查核的帳本數字。現有授點表單拒絕不安全的輸入，卻無法保證既有餘額、其他 client 或多次授點後的餘額安全；大額 Credit 不得悄悄顯示為另一個數字。

## Domain impact

Credit Context 繼續擁有帳本、分錄與換算。候選方案只改數值跨公開 API 的表示與消費，不改授權、理由、冪等、稽核、扣點或帳號所有權。

## Implementation handoff

候選方案是將所有 Credit 數量改為十進位字串，從 OpenAPI 修改來源，重生 client，Go 在邊界轉換，Web 以字串顯示、必要時以內建 BigInt 運算。範圍包括授點、帳單、創作估價與預算、後台查帳與趨勢、Trace、評估及比較；實作前須逐一盤點全部 Credit 欄位。美元微單位與計數不因共用 bucket 而混改。聚合超過 int64 時不能回傳失真成功。不安裝額外套件。

維持 JSON number 的替代方案必須對所有寫入、單帳戶與跨帳戶彙總制定並驗證上限；那會改變帳務可接受範圍，不能只加前端限制。

## Proposal and approvals

`credit-exact-json` 仍是 draft，尚未送審或取得 developer 核准，不是已決定的領域事實。負責人需先選字串或上限；若選字串，還需裁定公開 API 版本、舊 client 遷移與回退條件。

## Contract impact

JSON number 改 string 對既有 client 是破壞性變更。所有 generated 程式必須由契約重生，不能手改。

## Verification

依 `test-obligations.json` 驗證 2^53、int64、授點原有規則、彙總超界、所有 Web 消費面與舊 client 相容。新測試應先在修正前因具體數值不符而變紅。本 draft 尚無執行結果。

## Residual risks

美元微單位與計數也使用 int64/JSON number，需另外評估安全界線。SEC-010 事故通知派送與 SEC-011 版本停用／來源准入仍是後台完整性的獨立待決範圍。

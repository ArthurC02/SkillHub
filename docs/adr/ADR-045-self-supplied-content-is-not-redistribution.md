# ADR-045：自己帶進來的內容不是「再散布」——`skills.redistribution` 的第四個值

- 狀態：Accepted
- 日期：2026-08-23
- 相關：[ADR-027](./ADR-027-download-artifact-shape-reproducibility-and-integrity.md)（決策 4 定下兩道鎖與三態）、[ADR-021](./ADR-021-skill-license-provenance.md)（§5.3 誤判必須錯在「擋」的方向）、`02:PACK-001` 第一層、`04` 乙-17、`04` 丙-44、migration `0036`

## 背景：一個查證的結果，不是一份提案

一次針對「使用者的 Skill 到底下不下得了」的查證，得到的答案是**下不了**，而且**永遠下不了**。

實測（一般使用者身分，匯入一份 frontmatter 寫了 `license: MIT`、包內附了 LICENSE 檔的自寫 Skill）：

```
PREVIEW  allowed=false  blocked_reason=license_unknown
CREATE   HTTP 422       blocked_reason=license_unknown
```

使用者看到的是：**「沒有人確認過這個 Skill 可不可以再散布……這不是等待中的暫時狀態，是預設就擋。」**

三件事必須同時看，才看得出這是缺陷而不是設定問題：

1. `skills.redistribution` 預設 `'unknown'`，而 `gateFlags()` 的 default 分支 fail-closed。
2. **匯入路徑不寫這個欄位**——`CreateSkillFromPackage` 的原註解自承「an import carries no lineage and no verdict of its own」。
3. **產品裡沒有任何路徑可以改它**。`access_restriction` 有 `PUT`／`DELETE /admin/skills/{id}/restriction` 加 audit event；這一個只能直接下 SQL（`04` 乙-17：全庫唯一一個沒有稽核就能改的閘門欄位）。

repo 自己的測試 helper 早就寫在那裡：

> `allowRedistribution` is what a curator's verdict looks like in the database. **Every skill starts at 'unknown', which blocks**, so a packaging test that did not do this would be testing the gate and nothing else.

每一支打包測試都跑那一行 `UPDATE`。**沒有一支測試走的是使用者實際走的那條路**，所以這件事從來沒有失敗過。

## 決策 1：那道鎖問錯了問題，所以要改的是問題不是預設值

「再散布」是**平台把 A 的內容交給 B**。把一個工作區自己送上來的位元組交還給同一個工作區，那不是散布，是**取回**——沒有 B，而且平台沒有多做出一份使用者原本沒有的複本。

那道鎖為策展目錄而設，那裡授權是別人的、收受方是第三人，所以 ADR-021 §5.3 要求誤判必須錯在「擋」的方向。**那個推理在它自己的範圍內完全成立，本 ADR 不推翻它**；它只是被套用到一個它沒有描述的族群上。

**所以本 ADR 不是放寬那道鎖，是讓它認得出這個問題根本沒有被問到的情形。**

## 決策 2：第四個值 `self_supplied`，而不是把預設改成 `allowed`

把使用者匯入的內容標成 `allowed` 可以用三行改完，而且今天的行為完全正確。**不採用**，理由是它存了一個比實際成立的事更寬的主張：

| 值 | 它主張的事 | 誰建立的 |
| --- | --- | --- |
| `allowed` | **這份內容的授權**允許被複製 | 有人判定過 |
| `self_supplied` | **這份內容是這個工作區帶進來的** | 匯入當下的事實 |

差別在哪一天會咬人：**日後出現「發佈到目錄」的路徑時**，它必須面對一個不是 `allowed` 的值並停下來，而不是把今天寫下的這些列當成已判定的內容一併放出去。存 `allowed` 是一個**朝放行方向**的誤差，而那正是 ADR-021 §5.3 唯一禁止的方向。

`self_supplied` 通過打包閘門，**不主張任何關於授權的事**，而且畫面上的措辭也不主張（「可下載（你自己帶進來的）」，不是「可打包下載」）——把兩者顯示成同一個徽章，等於告訴使用者他自己上傳的東西有人幫他查過授權，而沒有人查過。

## 決策 3：判準是「誰的工作區」，不是「是不是上傳」

**這一條是整個改動的安全性所在。**

策展目錄的 45 個 Skill 是**經同一個上傳端點**載入的（`tools/content/import_seed.py` 走 `POST` 上傳）。若判準寫成「上傳進來的就是 self-supplied」，全部策展內容都會被標上這個值；而 `Fork` **逐字複製** `redistribution`，所以每一個 Fork 它們的工作區都會拿到放行的複本——**一次把整個目錄的授權判定繞過去**。

判準因此是 `workspaces.is_catalog`（`PACK-001` 第二層已經在用的同一個判別式）：**非策展工作區的匯入 → `self_supplied`；策展工作區 → 維持 `unknown`，等人判定。**

這一條有它自己的測試（`TestAnUploadIntoTheCatalogueIsNotSelfSupplied`），不是只寫在註解裡——它是那種「壞掉的時候沒有任何症狀」的條件。

**Fork 逐字帶著這個值仍然正確**：`Fork` 只讀**呼叫者自己的工作區**或**策展工作區**（`GetCatalogSkill` 的 scope 焊死在語句裡），而策展工作區依決策 3 不可能持有這個值——所以唯一會繼承它的，是一個工作區 Fork 它自己的內容。

## 決策 4：兩種來源都算，因為判準是「平台有沒有多加一個散布環節」

匯入有兩種：上傳（`upload`）與 URL 匯入（`git`）。兩者都給 `self_supplied`。

URL 匯入的位元組確實沒經過使用者的手，但**是使用者指名要平台去抓的，抓進他自己的私有工作區，而且只有他讀得到**。平台在這裡沒有多做出一個使用者自己做不到的散布動作。找不到必要理由的攔阻就不該存在——這是本批的裁定標準。

## 決策 5：既有列不在 migration 裡回填

`0036` 只改 CHECK 與註解。回填是 `tools/content/backfill-self-supplied.sql`，一支要人**知情地**執行、而且會印出它動了幾列與剩下什麼的語句。

理由：判斷一列是不是自己帶進來的要讀三張表（`skills`／`skill_versions`／`workspaces`），而**「哪些列不再被擋」是一個政策動作**。把它藏在一次 schema 變更裡執行，等於讓一個沒有人讀的檔案改變誰拿得到什麼。

## 影響

### 正面

- 使用者第一次可以下載自己的 Skill——**不論是自己上傳、URL 匯入，或系統協助改出來的新版本**（新版本掛在同一個 `skills` 列下，繼承這個值）。
- 「可以下載」與「授權查過了」在資料與畫面上都是兩件事，日後的發佈路徑分得出來。
- 策展內容的授權閘門一個字都沒放鬆。

### 成本與限制

- **`redistribution` 仍然沒有寫入端點與 audit**（`04` 乙-17 的另一半）。本 ADR 只回答了「使用者自己的內容怎麼辦」；「誰可以把**別人的**內容標成可散布、要什麼證據」仍未決，仍不在 MVP。
- **`self_supplied` 不是授權查核。** 使用者上傳一份別人的 source-available 套件，平台照樣交還給他。這是刻意的（他本來就有那份檔案），但它意味著**這個值永遠不能被當成散布給第三人的依據**。
- **多了一個第四種狀態要維護。** `delivery`／`discovery`／`registry` 各有一份這組字串的副本（既有狀況，非本批引入），真正綁住它們的是資料庫的 CHECK。

## 待決策

- **`redistribution` 的寫入端點與 audit**（承 ADR-027 待決策、`04` 乙-17）：誰能改別人的內容、改成 `allowed` 要什麼證據。ADR-021 §5.3 已裁定放寬的門檻不能只是一個按鈕。
- **`self_supplied` 的內容要不要能發佈到目錄**，以及那條路徑要求什麼。今天沒有這條路徑，本 ADR 只保證它出現時會被擋下來要求判定。

---
name: media-type-extension-mapper
description: Use when you need to map a user-provided list of file extensions to their official IANA media types and present the results as a table. This skill covers lookup, disambiguation, and reporting; it does not perform live web access itself.
---

# 目的
將使用者提供的副檔名，對應到 IANA 登記的正式媒體類型（media type），並整理成表格。

# 適用時機
- 使用者要求「依 IANA 登記」查副檔名對應的媒體類型。
- 使用者要把一串副檔名整理成對照表。
- 使用者要求正式名稱、標準名稱、或註冊的 media type，而不是猜測或常見俗稱。

# 核心原則
- 只使用 IANA 登記的媒體類型作為正式答案。
- 不要憑記憶猜測；若無法確認，明確標示「未能從 IANA 登記確認」。
- 若同一副檔名可能對應多個媒體類型，列出所有可確認的登記結果，並註明歧義。
- 若使用者未提供副檔名清單，先請對方提供清單。
- 若此環境無法直接存取 IANA 網站，明確說明需要可查詢 IANA 登記的資料來源或工具，並先整理待查清單與輸出格式。

# 工作流程
1. 讀取使用者提供的副檔名清單。
2. 正規化輸入：
   - 去除前導點號（例如 `.jpg` 視為 `jpg`）。
   - 保留原始大小寫作為參考，但查詢時以不區分大小寫處理。
   - 去除多餘空白。
3. 逐一查詢 IANA media types 登記：
   - 優先確認是否有對應的 `file extension` 或登記說明。
   - 以 IANA 登記頁面上的正式 type/subtype 為準。
4. 對每個副檔名判定結果：
   - **單一明確對應**：輸出正式媒體類型。
   - **多個可能對應**：列出所有可確認的正式媒體類型，並標示「歧義」。
   - **找不到**：標示「未找到 IANA 登記對應」。
5. 以表格輸出結果。

# 表格格式
至少包含以下欄位：
- 副檔名
- 正式媒體類型
- 備註

必要時可加上：
- 登記樹狀類別（例如 `application`、`image`、`text`）
- 是否歧義
- IANA 登記來源說明

# 輸出規則
- 使用繁體中文回覆。
- 表格中的正式媒體類型請保留 IANA 原始格式，例如 `image/jpeg`。
- 若有多個結果，使用分號分隔，並在備註說明原因。
- 若某項無法確認，備註要寫清楚是「未能從 IANA 登記確認」而不是自行推測。
- 若使用者提供的副檔名包含重複項，保留原順序並可在表格中重複列出，或先去重後註明，依使用者需求決定；預設保留原順序。

# 建議回覆結構
1. 先簡短說明已依 IANA 登記整理。
2. 接著輸出表格。
3. 若有歧義或未找到項目，最後補一段簡短說明。

# 注意事項
- 這個技能本身不保證能直接連網；若當前環境無法查詢 IANA，應先告知限制。
- 不要把非 IANA 的常見對應當成正式登記結果。
- 不要自行創造不存在的媒體類型。

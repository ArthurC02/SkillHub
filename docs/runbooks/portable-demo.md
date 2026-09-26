# Runbook：展示 Skill 的授權放行與下載

## 目標與邊界

展示「目錄內容 → 有證據的可散布判定 → 一般使用者 Fork → 打包 → 下載」。授權未知時拒絕是正確結果，不能用 SQL 批次改成 `allowed`。自己的上傳可以交還本人，也不能拿這個 `self_supplied` 路徑冒充目錄內容的再散布驗收。

本手冊只使用下面為 Demo 撰寫的固定回應素材，不對既有第三方種子套件下授權結論，不修改 golden corpus。淨測試模式只證明產品流程，不能證明正式物件儲存或 Sandbox 隔離；下載成功也不等於在下游 Agent 安裝、啟用或執行成功。

## Provision 與身分

先依[整體 Provision](provisioning.md)建立獨立本機環境。打包需要物件儲存、可讀的 `PACKAGING_PROFILES_DIR` 與明確的 `DOWNLOAD_ARTIFACT_RETENTION`；淨模式提供記憶體物件儲存與 profile 路徑，但保存期限仍須設定，例如本機驗收用 `720h`。未設定會回 503，不是授權拒絕；重新啟動淨模式會遺失資料庫與套件，必須重建資料，不能沿用舊 ID。

淨模式目前在沒有明確 `OPERATOR_USER_IDS` 時，以 `seed-importer` 作為該次啟動的 operator，並建立它的 catalog workspace。若已有設定，啟動器不覆寫它。登入後以 `/me` 確認 `operator=true`，不能只憑登入名稱判斷。另以不同開發登入名稱建立一般使用者，確認 `operator=false` 與不同 Workspace；非本機環境不可啟用開發登入。

公開目錄顯示素材還需要完成索引增強，因此需可用的 `apps/llm` 與模型閘道，匯入可能花費模型費用。先確認授權與成本範圍；本旅程不需要開啟生成／互動創作入口，也不需要啟動 Skill Run。

## 固定素材與來源

以下文字為本專案 Demo 新撰寫的最小素材，沒有第三方程式、Script、Dataset 或引用文件。只將這份文字存為 UTF-8、LF 換行且檔尾一個換行的 `SKILL.md`，放進 zip 根層。MIT 宣告限於這份素材，不是對原有 50 份種子內容的判定。

```markdown
---
name: portable-demo
description: Return a fixed word to demonstrate portable packaging.
license: MIT
---

# Response

Return exactly BETA. Do not use tools or read files.
```

## 操作與預期結果

1. 以 catalog workspace 的 operator 呼叫 `POST /skills/import/upload` 匯入 zip。回應列出這個來源帶進來的每一個 Skill，這份 fixture 只有一個；保存那一筆的 Skill ID、Version ID、content hash；讀回 `/api/skills/{id}`，確認 `scope=catalog`、License 為 `MIT/manifest`，而 `redistribution=unknown`。frontmatter 的授權宣告不等於已完成放行。
2. 未放行前，對該版本呼叫 `POST /skills/{id}/versions/{versionId}/packaging`，body 為 `{"target":"standard"}`，應回 `422 license_unknown`。若回 503，先修 Provision，再重驗授權分支，不把兩種拒絕混為一談。
3. 一般使用者呼叫下列 operator 端點應回 404；operator 提交不符快照的授權運算式，例如 `Apache-2.0`，應回 400。讀回狀態仍為 `unknown`，稽核紀錄沒有成功放行。
4. operator 檢視這份實際套件後，以 `PUT /admin/skills/{id}/redistribution` 提交以下內容。理由需描述本次實際查證，不能照抄到別人的套件上。

```json
{
  "value": "allowed",
  "note": "Reviewed the exact portable-demo fixture authored for this acceptance: only original fixed-response text, MIT declared in SKILL.md; no third-party files or scripts.",
  "license_expression": "MIT",
  "license_source": "manifest"
}
```

5. 確認回傳 `previous_value=unknown`、新值 `allowed`，並從 `GET /admin/audit-log` 找到同一 Skill 的 `skill.redistribution_set`：包含操作者、Workspace、時間、before／after、授權運算式、來源與理由。這不會把來源信任升為已驗證，也不會把套件 License 欄位的 `declared` 自動改成另一種狀態。
6. 一般使用者開啟目錄 Skill 頁，確認「可再散布」但須先 Fork。按「以這個 Skill 為起點建立我自己的」，進入新 Fork，核對來源 Skill／Version 與繼承的 `allowed`。
7. 由 Fork 的「打包並下載這個版本」進入，選 `standard`、不包含 Test Case，確認預覽可以打包且重驗無阻擋，再按「建立下載套件」。保存 artifact ID、Fork Version ID、檔名、大小、content hash 與 manifest hash。
8. 按下載，讀回下載紀錄確認操作者與時間；檢查 ZIP 根層只有 `SKILL.md`、`INSTALL.md`、`skillhub-manifest.json`。來源文字應逐位元組不變；Fork 的顯示名稱有後綴，不代表應改寫 `SKILL.md` 的 name。
9. 核對 ZIP SHA-256 等於 artifact 的 content hash；manifest 的來源 ID 是 Fork、upstream 是原始版本、來源 content hash 相同，並保留格式／能力／行為相容性的不同結論。另一個 Workspace 存取 artifact 與 content 均應回 404。

保存 API 回應與瀏覽器核對文字時，不保存 Cookie、Authorization、Virtual Key 或環境檔。本機資料庫是暫存；成功 ID 不是可重建配方，完整素材與操作流程才是。

## 既有證據

[下載 Demo 驗收報告](../plans/mvp/m6/report-packaging-demo-2026-09-25.md)保存真實授權拒絕、正式放行稽核、一般使用者 UI 操作、下載內容與 Workspace 拒絕證據。它不代表歷史種子全部已放行，也不取代正式部署驗收。

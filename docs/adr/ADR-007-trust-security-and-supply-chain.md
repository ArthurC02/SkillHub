# ADR-007：建立分層信任、安全區域與 Skill 供應鏈治理

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

Skill Hub 會從網路取得 Skill。Skill 可能包含指令、Script、依賴、外部 URL、MCP 設定與資產。格式有效不代表安全，靜態掃描通過也不代表行為符合使用者期待。

此外，Sandbox 輸出、MCP 回應與 Dataset 本身都可能包含 Prompt Injection 或惡意內容。

## 決策

採取 Zero Trust 與供應鏈證據模型：所有外部內容預設不受信任，依來源、License、格式、靜態分析、實際試跑與特定環境相容證據分層呈現。不得使用單一「安全」標章取代多維證據。

時程註記：遠端 MCP 與 Local Runner 已移出 MVP 首發。「MCP 與外部網路」章節及使用者裝置區的限制於對應功能啟動時生效，其餘自 MVP 起生效。

## 信任證據維度

| 維度 | 可能狀態 |
| --- | --- |
| 來源 | 未知、可追溯、人工確認 |
| License | 未知、已宣告、人工確認 |
| 規格 | 有效、警告、無效 |
| 靜態檢查 | 未檢查、通過、警告、阻擋 |
| 執行 | 未試跑、成功、失敗、部分成功 |
| 相容性 | 未驗證、在指定環境通過、在指定環境失敗 |
| 社群／平台證據 | 無證據、使用紀錄、平台精選 |

## Skill 匯入供應鏈

```text
取得外部內容
→ 建立 Source Record
→ Quarantine
→ 計算內容雜湊
→ 安全解壓與檔案清單
→ 解析 SKILL.md
→ 格式驗證
→ License 與來源分析
→ Script／URL／依賴靜態掃描
→ 建立不可變 Skill Version
→ 建立搜尋投影
→ 依證據標示精選或已索引
```

匯入與掃描階段不得執行套件內 Script。

## 安全區域

| 區域 | 內容 | 核心限制 |
| --- | --- | --- |
| 公開邊界 | Web、API Gateway、登入 | 輸入驗證、速率限制、身分驗證 |
| 控制與資料區 | 應用、資料庫、搜尋、Secrets | 僅受信任服務身分可存取 |
| 不受信任執行區 | Sandbox Worker、Egress Proxy | 無核心資料庫權限、短效存取、預設拒絕網路 |
| 使用者裝置區 | Local Runner、本機工具與資料 | 本機確認、最小路徑範圍、可撤銷裝置 |

## MCP 與外部網路

- 連線前先發現並顯示 MCP 工具、權限與目的地。
- 遠端位址經 SSRF 與 DNS Rebinding 防護。
- 阻擋內部位址、Metadata Service、Loopback 與未允許網段。
- MCP 憑證保存在 Secrets Store，執行時短效注入。
- 記錄工具名稱、參數摘要、目的地與結果狀態；敏感值先遮罩。
- MCP 回應仍視為不受信任輸入。

## Prompt Injection 邊界

- Skill、Dataset、網頁、MCP 回應與 Artifact 都標記來源與信任層級。
- 評估模型不取得控制平面工具或高權限 Secrets。
- 內容中的指令不能改變平台政策、允許工具或資料存取範圍。
- 結構化工具呼叫由 Policy Enforcement 再驗證，不只依模型判斷。
- 使用者可見報告區分來源事實、系統規則判斷與模型推論。

## Artifact 與下載

- Sandbox 產生的 Artifact 進入 Quarantine。
- 檢查檔案大小、類型、壓縮炸彈、惡意內容與 Secrets。
- 瀏覽器顯示不可信 HTML／SVG 等內容時使用安全隔離或下載模式。
- 打包時依 License、來源與 Secrets 政策重新驗證。

## 稽核事件

至少記錄：

- Skill 匯入、來源變更、掃描與下架。
- 權限確認與拒絕。
- Secret 建立、使用、撤銷與刪除的 metadata 事件。
- Run 建立、Provider 選擇、取消、逾時與清理。
- Artifact 下載及使用者資料刪除。

稽核不得保存 Secret 明文或不必要 Dataset 內容。

## 影響

### 正面

- 使用者能理解「格式有效」、「已掃描」與「已試跑」的差異。
- 供應鏈、MCP、Sandbox 與評估共享一致的信任模型。
- 支援未來企業治理、下架與事件調查。

### 成本與限制

- 信任狀態與 UI 較複雜。
- 掃描會有誤報與漏報，需保留人工評審和申訴流程。
- 無法承諾任何外部 Skill 絕對安全。

## 待決策

- 阻擋與警告的具體 Policy。 **→ 已解決（2026-08-22 對帳查證）**：等級表存在且是可逐條判定的——`shared/skillpkg` 的 `Severity`（`error` 阻擋／`warning` 揭露／`info` 揭露）在每個 finding code 上逐條指定，`Report.Blocked` 是彙總；閘門 B 的 `trial/execution/gateb.go` 的 `requireScanNotBlocking()` 重新掃描套件位元組、`error` 級即拒、掃不成也拒（fail-closed），三支具名測試（`04` 乙-8 已結案）。**威脅模型 Q7 與 `03:SEC-003` 都還寫著它未定案，那兩處已同批更正。**
- 支援的 License 辨識與無 License 內容行為。 **→ 已解決**：`skillpkg` 的 `licenseSignatures` 做 SPDX 辨識、缺席即 `unknown`；`skill/delivery/packaging.go` 把 `license_hold`／`license_unknown` 當成四道鎖之二直接拒發打包（ADR-021、migration `0012`）。
- Artifact 掃描及高風險檔案顯示策略。


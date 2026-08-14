# ADR-011：以 Workspace 建立租戶邊界，並預留 Policy 與 Usage 模組

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

MVP 首要場景是個人創作者，但長期需求包含多租戶、企業治理與計費。如果 MVP 所有資料只直接綁定 User，未來加入團隊 Workspace 時會產生大規模資料遷移與權限重寫。

同時，Run 會產生模型、Sandbox、網路與儲存成本，即使 MVP 尚未正式收費，也必須可計量與限制。

## 決策

- 每位 MVP 使用者建立一個個人 Workspace。
- 所有使用者擁有的 Skill、Test Case、Run、Secret Reference、Artifact 與 Usage 都以 Workspace 作為租戶邊界。
- MVP 可簡化 Membership 與 Role UI，但領域模型保留 Owner、Member 與未來自訂角色能力。
- 建立獨立 Policy & Usage 模組，先處理額度、限制與成本事件，未來再接入 Billing。

## Workspace 模型

```text
Workspace
├── Membership
├── Skill / Skill Version Ownership
├── Test Case
├── Run / Evaluation
├── Secret Reference
├── Artifact
├── Runner Device
└── Usage Record
```

公開 Skill 可以由平台或公開 Workspace 擁有；使用者 Fork 後的新 Skill 屬於個人 Workspace，並保留來源譜系。

## 授權原則

- 身分驗證只證明「你是誰」，授權必須檢查 Workspace 與資源關係。
- Repository／Query 預設要求 Workspace Scope。
- Background Job、Provider Callback 與 Artifact Download 使用受限服務身分及 Resource Scope。
- 不信任 UI 傳入的 `workspace_id`；伺服器由 Membership 與 Resource Ownership 驗證。
- 公開讀取與私有修改使用不同權限路徑。

## Policy

MVP 至少支援：

- 每 Workspace 的並行 Run 上限。
- 每日／每月 Run 或 Sandbox 分鐘額度。
- Dataset、Artifact 與 Trace 大小限制。
- 允許的 Provider、Runtime、模型與網路政策。
- 自備 API Key 與平台代付模型的不同政策。
- 高風險 Skill 或 MCP 的阻擋與人工確認。

Policy Decision 應保存規則版本與結果，確保歷史 Run 可解釋。

## Usage Record

每次 Run 至少可計量：

- Provider 與 Sandbox 執行時間。
- CPU／Memory 等可用資源量測。
- 模型 Token 與費用估算。
- 網路流量與 MCP 呼叫次數。
- Dataset、Trace 與 Artifact 儲存量。
- 評估與重新試跑的額外成本。

Usage Record 以追加事實保存，不因價格表更新而改寫。未來帳單由 Usage 與當時有效價格／方案產生。

## 資料隔離策略

MVP 可先使用共享資料庫與共享 Schema 的邏輯隔離，但必須：

- 所有租戶資料有 Workspace Scope。
- 建立自動化跨 Workspace 存取測試。
- 管理員支援操作具備稽核與有限時權限。
- Object Key、Search Document、Trace 與 Cache 同樣包含租戶邊界。

未來若企業要求獨立資料庫、區域或加密金鑰，可在 Workspace Policy 下選擇不同 Deployment／Storage Profile。

## 影響

### 正面

- 個人 MVP 不阻礙未來團隊與企業功能。
- 用量、成本與限制從第一天可觀察。
- Provider Routing 可依 Workspace 政策演進。

### 成本與限制

- 所有查詢與背景工作都需處理租戶 Context。
- 共享 Schema 的邏輯隔離需要嚴格測試與開發規範。
- Policy Version 與 Usage Event 增加資料模型。

## 待決策

- Workspace ID 的生成與服務間傳遞規範。
- 身分供應商與服務身分模式。→ [ADR-020](./ADR-020-authentication-and-session-model.md)
- 免費額度、自備 Key 與成本顯示方式。
- 企業資料隔離 Profile 的觸發條件。


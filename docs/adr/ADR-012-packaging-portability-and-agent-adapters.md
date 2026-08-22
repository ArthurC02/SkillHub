# ADR-012：標準 Skill 為核心，Agent Adapter 僅處理目標平台差異

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

Skill Hub 的目標是支援通用 Agent Skills，而非綁定單一 Agent。然而不同 Agent 對安裝路徑、工具命名、額外設定、允許欄位與 Runtime 能力的支援不完全一致。

如果 Skill Hub 以某一平台格式作為內部核心，會造成格式鎖定；如果只提供完全相同的 ZIP，又可能讓使用者下載後無法安裝。

## 決策

- Skill Registry 以標準 Agent Skill Package 為核心事實來源。
- 平台專屬差異以 Agent Packaging Profile／Adapter 實作。
- Adapter 不得靜默改變 Skill 的任務意圖或移除必要安全限制。
- 標準套件、目標平台產物與安裝說明分開版本化及驗證。
- 相容性分成格式、能力與實際測試證據，不宣稱所有 Agent 行為一致。

## 三層相容性

| 層級 | 定義 | 證據 |
| --- | --- | --- |
| 格式相容 | 套件與 `SKILL.md` 符合標準結構 | 規格驗證報告 |
| 能力相容 | 目標 Agent 具備需要的工具、MCP、檔案與 Runtime | Capability Matching |
| 行為相容 | 在特定 Agent、模型與環境完成 Test Case | Run 與 Evaluation 證據 |

## Packaging Pipeline

```text
選擇不可變 Skill Version
→ 驗證標準格式
→ 檢查來源與 License
→ 掃描 Secrets、內部路徑與受保護資料
→ 選擇 Agent Packaging Profile
→ 產生目標差異與安裝說明
→ 驗證輸出套件
→ 產生不可變 Download Artifact
```

## Agent Packaging Profile

可描述：

- Agent 名稱、版本範圍與支援狀態。
- 安裝目錄與檔案命名。
- 支援的 frontmatter 與擴充欄位。
- 工具與 MCP 設定對映。
- 環境變數範本。
- 作業系統差異。
- 安裝後驗證 Prompt／命令。
- 已知限制與未驗證能力。

Profile 應為版本化設定與程式 Adapter 的組合，不把平台差異寫回標準 Skill Version。

## 來源與授權

- 原作者、來源 URL、來源版本／Commit 與 License 必須保留。
- Fork 與改善版本保存衍生關係。
- 無 License 或不允許再散布時，打包流程依 Policy 阻擋或限制用途。
- 下載包不得包含執行時 Secret、測試憑證、使用者內部路徑或未授權 Dataset。

## 可重現性

Download Artifact 記錄：

- 來源 Skill Version。
- Packaging Profile 及版本。
- 打包器版本。
- 建立時間與內容雜湊。
- 驗證結果。
- 包含／排除的 Test Case 與範例資料清單。

相同輸入與版本應能產生語意等價的套件；若內容雜湊因壓縮時間等非語意 metadata 不同，需另有規範化 Manifest Hash。

## 影響

### 正面

- 保持標準格式與平台差異分離。
- 可逐步新增目標 Agent，不重寫 Skill Registry。
- 使用者取得明確安裝與相容性資訊。

### 成本與限制

- 每個 Agent Profile 都需要維護、測試與版本追蹤。
- 目標 Agent 更新可能使既有 Profile 過期。
- Adapter 只能處理已知差異，無法保證跨模型行為完全一致。

## 待決策

- MVP 首批支援的 Agent Packaging Profile。（提案見 PDM-008：1 標準套件 ＋ `claude-code`／`claude-agent-sdk` 兩個已驗證安裝 Profile，**待負責人追認**）
- Profile 的設定格式與 Adapter Plugin 機制。（**刻意仍不立 ADR**：MVP 只有三個打包目標且全部內建，Profile 以「版本化設定 ＋ 內建 Adapter」落地即滿足本 ADR；先做一套外掛機制是為一個不存在的第二方鋪路。理由見 [m4/README.md §2.2／§8.2](../plans/mvp/m4/README.md)） **→ 已解決（以「不做」的形式）**：MVP 不立 Adapter Plugin 機制，改以版本化設定＋內建 Adapter。而且設定真的是資料檔不是編譯常數——`contracts/packaging/profiles/*.json` 三份 `schema_version 1.0`，`skill/delivery/profile.go` 檔頭寫「Data driven rather than compiled in」。
- 打包 Artifact 簽章、完整性驗證與撤銷方式。→ **已決，見 [ADR-027](./ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 3**：**MVP 明文不簽章**（下載是登入後的短效授權而非公開發布、沒有驗證端、簽章是一組金鑰生命週期而非一個欄位），完整性由 `content_hash` ＋ `manifest_hash` 雙雜湊承載（決策 1），撤銷在 MVP 只到「停止再發」不到「使已發出的副本失效」——該殘餘風險與重開訊號逐項寫在該 ADR。
- 上方「可重現性」一節要求的**規範化 Manifest Hash** → 已由 [ADR-027](./ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 1、2 回答（`manifest_hash` 的算法與範圍、zip 寫入規範化，以及可重現性僅限同一個打包器版本內）。


# SkillHub D 階段改進清單（可小步驟審查）

## 本 PR 已完成（低風險垂直切片）

1. **共享邏輯抽離（tools/devctl）**
   - 抽出 key/value 與字串正規化 helper，並套用到 `.env`、toolchain section、Claude agent frontmatter 解析。
   - 實際 caller：`readDotEnv`、`parseManifestSection`、`parseClaudeAgent`。
   - 附帶測試：`TestParseKeyValue` 與既有解析測試。
2. **關鍵邊界測試補強（apps/platform）**
   - 針對 diagram 文字驗證補上 2000/2001 rune 邊界與空白輸入案例。
   - 將 `validDiagramDescription` / `validDiagramAnswer` 的共同規則收斂到單一 helper，降低未來行為分岔風險。
3. **contracts/generated 驗證**
   - 執行 `go -C tools/devctl run . gen --check`，確認目前 generated output 無 drift。

## 待後續執行（依優先順序）

| 優先級 | 項目 | 預期影響 | 實作難度 | 後續行動 |
| --- | --- | --- | --- | --- |
| P0 | 修復既有 CI 基線漂移（dependency policy：SeaweedFS/UV/Node 版本一致性） | 恢復主線 CI 穩定，避免與功能改動互相干擾 | 中 | 以單一 PR 專注更新對應檔案，先對齊 `infra/compose` 與 CI/腳本，再跑完整 CI |
| P1 | 為 `apps/platform/internal/creator/creation` 補更多輸入邊界與非法狀態轉移測試 | 降低互動創作流程回歸風險 | 中 | 依 ISTQB 做等價類/決策表盤點，再逐步補測 |
| P1 | 盤點 `tools/devctl` 其餘重複的字串標準化與錯誤訊息格式 | 降低維護成本與輸出不一致 | 低 | 每次只抽一種重複模式，保留行為不變並附測試 |
| P2 | 建立可重複的覆蓋率與重複率量測基準（不改產品邏輯） | 讓後續優化有量化對照 | 中 | 先定義可在 CI 執行的量測腳本，再在報告中引用數據 |
| P2 | 針對 `apps/web` / `apps/llm` / `apps/sandbox` 做模組內低風險去重切片 | 穩定提升可維護性 | 中 | 每次只改單一模組、單一主題，避免跨語言抽象化 |

## 不做事項（本階段明確排除）

- 不建立沒有 production/tooling caller 的新 shared package。
- 不引入跨語言共用程式碼 package 來繞過 contracts/schema。
- 不在沒有具體問題時改動 generated 或 lockfile。
- 不宣稱重複率或覆蓋率改善百分比（目前未量測）。

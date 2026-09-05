# 互動式 Skill 創作：開發與驗證

依據 [ADR-067](../adr/ADR-067-interactive-skill-creation-with-langgraph.md)。實作授權見 [01 §10](../plans/01-goals-and-plan.md)，啟用參數仍由 [05 R-45](../plans/05-pending-rulings.md) 定值。

## 已接通的路徑

`/workspace/skills#create` 的自然語言、流程圖、目錄參考共用私人會話。Web 僅呼叫 Go API；Go Worker 每個工作以內部 HTTP 呼叫 Python `POST /v1/creation/step`。LangGraph 重建有界 workflow，回傳澄清、確認、草稿或工具意圖；Go 執行授權後的目錄文字檢索、套件驗證與後續排程。目錄工具不另呼叫 embedding 模型，避免繞過會話費用紀錄。

Go 擁有 `creation_sessions`、不可更新的 `creation_session_events` 與 `creation_receipts`，使用 Workspace scope、revision CAS 和命令識別碼。事件、receipt、快照及後續 River 工作在同一交易寫入。會話 UI 使用 scoped polling，沒有新增對外推播 consumer；既有 Run 仍走原本 Outbox。

流程圖由 API 將記憶體中的位元組傳給 Go Worker 內部 listener，再送 Python。資料庫只保存理解與 sha256／媒體類型／大小。中斷後不重播圖像工作，要求重新上傳。LangGraph 不持有跨工作 checkpoint，不連核心資料庫，也不消費 River。

使用者確認後，Go 使用既有 admission 靜態驗證與物件寫入 fence 建立私人不可變候選。建立候選與會話 CAS 同交易；最終保存已存在候選時只選定同一版本，不再生成。未試跑亦可明確保存；試跑沿原本權限／成本 preflight。附加 Run 時驗證 Workspace 與候選版本，再帶實際執行／評估狀態進修訂；新草稿顯示前一份內容供比較。

## 設定與預設

所有設定由程序入口注入，沒有核准數字的預設值。以下設定寫完不代表已獲准曝光。

| 設定 | 使用處 |
| --- | --- |
| `GENERATE_SKILL_EXPOSED=on` 與 `CREATION_EXPOSED=on` | API 的雙重曝光閘門；Web 另檢查 `features.generate_skill` 與 `features.creation_skill` |
| `CREATION_LIMITS_JSON` | API 與 Worker 必須使用相同核准政策 |
| `CREATION_WORKER_INTERNAL_ADDR` | Worker 的內部 HTTP 監聽位址 |
| `CREATION_WORKER_INTERNAL_URL` | API 可到達的 Worker 內部 URL，無結尾斜線 |
| `CREATION_WORKER_INTERNAL_TOKEN` | API／Worker 的相同服務憑證，不放前端 |
| `LLM_SERVICE_URL`、`LLM_SERVICE_TOKEN` | Worker 呼叫 Python 的既有設定 |
| `SKILLHUB_MODEL_GATEWAY_URL`、`SKILLHUB_MODEL_GATEWAY_KEY` | Worker 既有 LiteLLM 管理接線；管理金鑰不傳 Python |

`CREATION_LIMITS_JSON` 的必要鍵為 `max_cost_usd`、`max_call_cost_usd`、`max_steps`、`max_tool_calls`、`call_timeout_seconds`、`session_timeout_seconds`、`retention_seconds`、`max_output_tokens`。值須為有效正數；單次預算不得超過總上限、單次時間不得超過 Python 的 120 秒、保存期限不得短於會話時間。實際數字依 R-45 核准後填寫，測試 fixture 的數字不是部署核准值。

每次模型工作先以 receipt 預留單次費用，再簽發限定 `gpt-5.4-mini`、單次金額與 TTL 的 Virtual Key。Python 只從 `X-Creation-Gateway-Key` header 取得短效 key；沒有 key 不回退共用金鑰。回應缺少真實 cost 時保留預留額並標示未知。取消、程序重啟及遲到結果不得重複計費或復活創作；Worker 的周期工作處理中斷與到期清除。帳號刪除會刪除會話、事件與 receipts，資料庫 write fence 拒絕刪除開始後的私人資料寫入。

Clean mode 由同一程序內的 Go Worker 服務接收瞬時圖像，仍不讓 API 呼叫 Python。此模式的設定與曝光規則相同。

## 可重現的免費證據

- Python `apps/llm/tests/test_creation.py`：多輪與工具 observation、需求更正、圖像只傳一次、strict schema、截斷、未知費用、關閉 LangSmith tracing、取消傳至模型 await。
- Go `creation_integration_test.go`：真實 PostgreSQL、正式 API／Worker composition root、HTTP 模型替身；驗證需求確認 → 草稿驗證 → 私人候選 → 保存同一版本，以及圖像不落地、命令重播、過期 revision、跨 Workspace、參考重新授權、取消與刪除競態、預算耗盡不再排程、重啟保留未知費用且不重播。
- Web `creation.test.tsx`：三種素材的實際 API payload、會話恢復、確認動作、未知費用、409 保留輸入、網路重試沿用識別碼、曝光關閉不掛載。

Go 資料庫測試只可指定 localhost 且名稱結尾為 `_test` 的可拋棄資料庫；測試會重建該資料庫的 public schema。不得指向開發中的正式資料庫。付費測試預設跳過，免費替身只能證明控制流程與邊界，不能證明創作品質。

2026-09-05 本機驗證紀錄：Python 全套 206 通過、4 跳過；Web 全套 456 通過，型別檢查、格式檢查及正式建置成功。Go 全套曾完成 1,279 通過、9 跳過；最後新增復原／預算／金鑰歸屬測試後，重跑受影響的五個套件，582 通過、5 跳過。Go 靜態檢查顯示 `0 issues`，契約生成一致性與 automation contract 均成功。這些是本機證據，不代表 CI 或付費模型驗收。

兩次突變驗證均有紅／綠證據：釋放未知費用的預留額度，或讓 `creation_skill` 繞過 `generate_skill` 曝光限制，對應測試都失敗；恢復原始檔案後通過，並確認位元組一致。正式建置仍有 bundle 超過 500 kB 的警告；完整 `git diff --check` 會指出 TypeScript 契約產生器的註解尾端空白，排除生成目錄後成功。生成檔維持產生器原樣，未手改。

## 尚待量測與核准

R-45 的實際部署預算／保存期限／量測門檻、三種輸入的真實模型多輪任務、與單次生成的效果比較及人類採用率仍待收齊。GEN-016～023 的 checkbox 保持未勾，直到各自完整允收證據齊備；已接線不等於產品品質或曝光驗收完成。本批沒有啟用曝光、部署服務或執行未核准的付費模型。

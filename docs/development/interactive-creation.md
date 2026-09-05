# 互動式 Skill 創作：開發與驗證

依據 [ADR-067](../adr/ADR-067-interactive-skill-creation-with-langgraph.md)。實作授權見 [01 §10](../plans/01-goals-and-plan.md)，啟用參數仍由 [05 R-45](../plans/05-pending-rulings.md) 定值。

## 已接通的路徑

`/workspace/skills#create` 的自然語言、流程圖、目錄參考共用私人會話。Web 僅呼叫 Go API；Go Worker 每個工作以內部 HTTP 呼叫 Python `POST /v1/creation/step`。LangGraph 重建有界 workflow，回傳澄清、確認、草稿或工具意圖；Go 執行授權後的目錄文字檢索、套件驗證與後續排程。目錄工具不另呼叫 embedding 模型，避免繞過會話費用紀錄。

Go 擁有 `creation_sessions`、不可更新的 `creation_session_events` 與 `creation_receipts`，使用 Workspace scope、revision CAS 和命令識別碼。事件、receipt、快照及後續 River 工作在同一交易寫入。會話 UI 使用 scoped polling，沒有新增對外推播 consumer；既有 Run 仍走原本 Outbox。

流程圖由 API 將記憶體中的位元組傳給 Go Worker 內部 listener，再送 Python。資料庫只保存理解與 sha256／媒體類型／大小。中斷後不重播圖像工作，要求重新上傳。LangGraph 不持有跨工作 checkpoint，不連核心資料庫，也不消費 River。

使用者確認後，Go 使用既有 admission 靜態驗證與物件寫入 fence 建立私人不可變候選。建立候選與會話 CAS 同交易；最終保存已存在候選時只選定同一版本，不再生成。未試跑亦可明確保存；試跑沿原本權限／成本 preflight。附加 Run 時驗證 Workspace 與候選版本，再帶實際執行／評估狀態進修訂；新草稿顯示前一份內容供比較。

## LangGraph 階段與實際回饋

「prepare → observe」根據確認狀態與草稿驗證結果選擇 understand、compose、revise 或 review，再回傳確認、工具意圖或草稿。新內容一律先交 Go 靜態驗證；下一個工作攜帶同一草稿的 draft_validation（content hash、blocked、finding report），讓修訂階段針對問題修改。只有與 Go 驗證通過內容完全相同的草稿，Python 才回傳完成提案。Go 仍獨立驗證、綁定確認與控制狀態，模型不能自行越過。

工具邊界結束本次 graph invocation；Go 持久化結果、保留前一草稿並核准下一次工作，再進入 observe。這個循環每次只有一次模型呼叫，費用與取消仍受 receipt 控制；沒有另建 Python checkpointer 或讓 Python 接管平台狀態。

附加 Run 後的回饋包含實際執行狀態、failure class，以及 evaluation owner 提供的驗收條件、判定原因、已驗證且重新檢查可用性的證據摘錄。沒有評估時明示 evaluation_available:false。評估投影最多 16,000 字元，摘要最多 2,000 字元；刪減時保留判定並附截斷及省略數量，不能把不完整證據當成成功。原始 Trace、完整產物和評估留言不送入創作模型。

流程圖上限為 4,000,000 bytes。diagram_understanding 是 JSON 編碼字串，必須恰含 nodes、conditions、branches、uncertainties 四個字串陣列；節點不能為空，其餘無內容時仍傳空陣列。Python 與 Go 都檢查結構，前端逐節呈現後要求確認。舊版純文字仍可閱讀，但必須重新整理並確認，才能繼續保存。

Catalog 參考畫面列出選定不可變版本的描述、相容性與工具需求，不從最新版 API 補資料。選擇或搜尋新參考會取消需求確認；模型比較做法、限制、採用與捨棄部分後，重新提出需求供人確認。

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

`CREATION_LIMITS_JSON` 的必要鍵為 `max_cost_usd`、`max_call_cost_usd`、`max_steps`、`max_tool_calls`、`call_timeout_seconds`、`session_timeout_seconds`、`retention_seconds`、`max_output_tokens`。值須為有效正數；單次預算不得超過總上限、單次時間不得超過 Python 的 120 秒、保存期限不得短於會話時間。**2026-09-06 已定值**（[`05` R-45](../plans/05-pending-rulings.md) 裁定表，負責人授權代理定值）：`max_cost_usd` 1.0、`max_call_cost_usd` 0.1、`max_steps` 24、`max_tool_calls` 8、`call_timeout_seconds` 90、`session_timeout_seconds` 259200、`retention_seconds` 2592000、`max_output_tokens` 16000；`.env.example` 帶著同一行 JSON。測試 fixture 的數字仍不是部署值。

每次模型工作先以 receipt 預留單次費用，再簽發限定 `gpt-5.4-mini`、單次金額與 TTL 的 Virtual Key。Python 只從 `X-Creation-Gateway-Key` header 取得短效 key；沒有 key 不回退共用金鑰。回應缺少真實 cost 時保留預留額並標示未知。取消、程序重啟及遲到結果不得重複計費或復活創作；Worker 的周期工作處理中斷與到期清除。帳號刪除會刪除會話、事件與 receipts，資料庫 write fence 拒絕刪除開始後的私人資料寫入。

Clean mode 由同一程序內的 Go Worker 服務接收瞬時圖像，仍不讓 API 呼叫 Python。此模式的設定與曝光規則相同。

## 可重現的免費證據

- Python `apps/llm/tests/test_creation.py`：多輪與工具 observation、需求更正、圖像只傳一次、strict schema、截斷、未知費用、關閉 LangSmith tracing、取消傳至模型 await。
- Go `creation_integration_test.go`：真實 PostgreSQL、正式 API／Worker composition root、HTTP 模型替身；驗證需求確認 → 草稿驗證 → 私人候選 → 保存同一版本，以及圖像不落地、命令重播、過期 revision、跨 Workspace、參考重新授權、取消與刪除競態、預算耗盡不再排程、重啟保留未知費用且不重播。
- Web `creation.test.tsx`：三種素材的實際 API payload、會話恢復、確認動作、未知費用、409 保留輸入、網路重試沿用識別碼、曝光關閉不掛載。

Go 資料庫測試只可指定 localhost 且名稱結尾為 `_test` 的可拋棄資料庫；測試會重建該資料庫的 public schema。不得指向開發中的正式資料庫。付費測試預設跳過，免費替身只能證明控制流程與邊界，不能證明創作品質。

2026-09-05 本機驗證紀錄：Python 全套 206 通過、4 跳過；Web 全套 456 通過，型別檢查、格式檢查及正式建置成功。Go 全套曾完成 1,279 通過、9 跳過；最後新增復原／預算／金鑰歸屬測試後，重跑受影響的五個套件，582 通過、5 跳過。Go 靜態檢查顯示 `0 issues`，契約生成一致性與 automation contract 均成功。這些是本機證據，不代表 CI 或付費模型驗收。

兩次突變驗證均有紅／綠證據：釋放未知費用的預留額度，或讓 `creation_skill` 繞過 `generate_skill` 曝光限制，對應測試都失敗；恢復原始檔案後通過，並確認位元組一致。正式建置仍有 bundle 超過 500 kB 的警告；完整 `git diff --check` 會指出 TypeScript 契約產生器的註解尾端空白，排除生成目錄後成功。生成檔維持產生器原樣，未手改。

新增的 creation_python_integration_test.go 需設定絕對路徑 SKILLHUB_CREATION_PYTHON，指向已安裝 repo 依賴的 Python，配合上述可拋棄資料庫。它啟動真正 FastAPI／LangGraph，僅 LiteLLM 相容模型端點使用本機替身；未設定 Python 路徑會明確跳過。測試覆蓋錯誤草稿、Go finding、Python 修訂、相同內容複查與保存候選，不能取代真實模型品質驗收。

2026-09-05 第二輪審視後的本機證據：Python 全套 220 通過、4 跳過；Web 全套 458 通過，型別與格式檢查成功。受影響的六個 Go 套件 569 通過、6 跳過（既有 corpus／真實服務測試未具備條件），兩條新增跨程序／真實評估回饋測試均實際執行。Go lint 為 0 issues，契約生成一致性、automation contract 與非生成檔案的 diff whitespace 檢查成功。

跨程序測試發現 Go GeneratedSkill 省略空欄位會讓 Python 拒絕下一輪請求；傳輸現在保留契約要求的空字串與檔案陣列。兩次新增突變驗證分別重加 allowed_tools 的 omitempty、移除 draft_validation：同一條真實 Go／Python 測試都因行為斷言失敗，恢復修正後通過，檔案位元組與突變前一致。這些證據證明回饋循環確實接通，仍不代表付費模型品質或 CI 已通過。

最終審查另補上「有流程理解、沒有圖像指紋」的保存防線：仍須完成結構化確認。此變更後創作單元測試 8 通過、API 創作整合測試 11 通過，均無跳過；重新限制為僅檢查圖像指紋時測試失敗，恢復後通過。相關 lint 與契約一致性再次成功。

## 2026-09-05 深化批（六線稽核後的修補）

負責人要求持續深化。先以工作流做六線稽核（Go 會話核心、三程序接縫、Python 圖、草稿到版本、Web 會話、Skill 品質；各一讀者、一反駁者，一位補漏評論者），再以 `parallel-page-edit` 六個 writer 落地；逐條見 [`04` 丙-167～172](../plans/04-backlog-and-handoffs.md)，三個要簽的設計見 [`05` R-46](../plans/05-pending-rulings.md)。

- **接縫**：`draft_validation.report` 截斷落在上限之內；`allowed_tools` 依剩餘工具次數計算；`canSpend` 檢查訊息數而不再檢查工具數；`timeout_seconds` 是呼叫當下剩餘的秒數；舊草稿不再帶著通過的驗證送出；過期的瞬時上傳回 409（契約補 404）。
- **會話事實**：使用者訊息不再清掉草稿與候選；`confirm_references` 同時恢復 `Available`；被恢復程序取代的嘗試遲到只寫收據；`queued` 列只在會話時鐘過了才掃；保存期限清除不再被恢復錯誤擋住；`generation_inputs` 不寫空的 diagram、參考只留 id。
- **主鍵**：migration 0057 把 `creation_sessions`／`creation_session_events`／`creation_receipts` 改成含 workspace（session）的複合主鍵，關掉他人 id 的存在性 oracle。
- **公布上限**：`GET /creation-sessions/limits`；`View.deadline`；三種 422 各有自己的句子（同名、區間、時間上限）。
- **模型看到什麼**：compose／revise／review 相帶著單次路徑的 `FIELD_RULES`；圍欄外一句平台事實的權威聲明；`revise` 相知道「沒有驗證＝在更正之前」。
- **畫面**：預算區間、已用步數／工具次數、可以離開、上次更新、兩個時鐘、共用 `Findings`、Run 結果、TypeError 中文。

本機證據（2026-09-05 深夜）：Go 全套 27 個套件 ok（含 DB）；Python 224 通過、4 跳過；Web 464 通過，型別、lint、格式檢查成功；golangci-lint 0 issues；契約生成一致、automation contract 成功。新增測試：Go 15、Python 4、Web 3；六份簡報各有一次突變紅／綠證據。這些仍是免費替身上的證據，不代表付費模型品質或真人採用。

## 2026-09-06 R-45／R-46 落地

- **驗收條件成為資料**：模型在提 brief 的同一個決策回 `acceptance_criteria`；Go 與 brief 一起綁定（換了就退回確認）；`materialize` 在同一交易以 `CreateTestCaseWithCriteria` 建立候選的 Test Case（source user、`confirmed_at` 為確認時刻），`Candidate.test_case_id` 回到畫面、試跑連結預填。
- **`raise_budget`**：額度被拒的會話可提高預算後從 `waiting_input` 繼續；區間由 `/creation-sessions/limits` 公布，超出回 422 並寫出區間。
- **`reason` 碼**：Python 護欄只回碼，Go 出句子；`creation.py` 不再有中文。
- 值與門檻見 `05` R-45；同意書 §3 新增互動創作一列（法務尚未看過，功能封測期間不曝光）。

## 尚待量測與核准

R-45 的實際部署預算／保存期限／量測門檻、三種輸入的真實模型多輪任務、與單次生成的效果比較及人類採用率仍待收齊。GEN-016～023 的 checkbox 保持未勾，直到各自完整允收證據齊備；已接線不等於產品品質或曝光驗收完成。本批沒有啟用曝光、部署服務或執行未核准的付費模型。

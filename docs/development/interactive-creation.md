# 互動式 Skill 創作：開發與驗證

依據 [ADR-067](../adr/ADR-067-interactive-skill-creation-with-langgraph.md)。實作授權見 [01 §10](../plans/01-goals-and-plan.md)；啟用參數已由 [05 R-45](../plans/05-pending-rulings.md) 定值，值見〈設定與預設〉。

## 已接通的路徑

`/workspace/skills#create` 的自然語言、流程圖、目錄參考共用私人會話。Web 僅呼叫 Go API；Go Worker 每個工作以內部 HTTP 呼叫 Python `POST /v1/creation/step`。LangGraph 重建有界 workflow，回傳澄清、確認、草稿或工具意圖；Go 執行授權後的目錄文字檢索、套件驗證與後續排程。目錄的文字檢索與套件驗證不呼叫 embedding；`search_knowledge` 會呼叫，費用計入該會話的 `SpentUSD`。

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
| `CREATION_MODEL`（Python，預設 `gpt-5.4-mini`） | 互動創作每一步用的模型別名；只給 `05` R-45 的量測換一級用。Go 簽給每一步的 Virtual Key 仍只限 `gpt-5.4-mini`，所以在產品裡改這個值不會生效——要換產品的模型，Go 的簽發那一行要一起改 |
| `SKILLHUB_MODEL_GATEWAY_URL`、`SKILLHUB_MODEL_GATEWAY_KEY` | Worker 既有 LiteLLM 管理接線；管理金鑰不傳 Python |

`CREATION_LIMITS_JSON` 的必要鍵為 `max_cost_usd`、`max_call_cost_usd`、`max_steps`、`max_tool_calls`、`call_timeout_seconds`、`session_timeout_seconds`、`retention_seconds`、`max_output_tokens`。值須為有效正數；單次預算不得超過總上限、單次時間不得超過 Python 的 120 秒、保存期限不得短於會話時間。**現行值**（[`05` R-45](../plans/05-pending-rulings.md)）：`max_cost_usd` 1.0、`max_call_cost_usd` 0.1、`max_steps` 24、`max_tool_calls` 8、`call_timeout_seconds` 90、`session_timeout_seconds` 259200、`retention_seconds` 2592000、`max_output_tokens` 16000；`.env.example` 帶著同一行 JSON。測試 fixture 的數字仍不是部署值。

每次模型工作先以 receipt 預留單次費用，再簽發限定 `gpt-5.4-mini`、單次金額與 TTL 的 Virtual Key。Python 只從 `X-Creation-Gateway-Key` header 取得短效 key；沒有 key 不回退共用金鑰。回應缺少真實 cost 時保留預留額並標示未知。取消、程序重啟及遲到結果不得重複計費或復活創作；Worker 的周期工作處理中斷與到期清除。帳號刪除會刪除會話、事件與 receipts，資料庫 write fence 拒絕刪除開始後的私人資料寫入。

Clean mode 由同一程序內的 Go Worker 服務接收瞬時圖像，仍不讓 API 呼叫 Python。此模式的設定與曝光規則相同。

## 可重現的免費證據

- Python `apps/llm/tests/test_creation.py`：多輪與工具 observation、需求更正、圖像只傳一次、strict schema、截斷、未知費用、關閉 LangSmith tracing、取消傳至模型 await。
- Go `creation_integration_test.go`：真實 PostgreSQL、正式 API／Worker composition root、HTTP 模型替身；驗證需求確認 → 草稿驗證 → 私人候選 → 保存同一版本，以及圖像不落地、命令重播、過期 revision、跨 Workspace、參考重新授權、取消與刪除競態、預算耗盡不再排程、重啟保留未知費用且不重播。
- Web `creation.test.tsx`：三種素材的實際 API payload、會話恢復、確認動作、未知費用、409 保留輸入、網路重試沿用識別碼、曝光關閉不掛載。

Go 資料庫測試只可指定 localhost 且名稱結尾為 `_test` 的可拋棄資料庫；測試會重建該資料庫的 public schema。不得指向開發中的正式資料庫。付費測試預設跳過，**免費替身只能證明控制流程與邊界，不能證明創作品質**。

`creation_python_integration_test.go` 需設定絕對路徑 `SKILLHUB_CREATION_PYTHON`，指向已安裝 repo 依賴的 Python，配合上述可拋棄資料庫。它啟動真正的 FastAPI／LangGraph，僅 LiteLLM 相容模型端點使用本機替身；未設定 Python 路徑會明確跳過。它覆蓋錯誤草稿、Go finding、Python 修訂、相同內容複查與保存候選。

**跨程序傳輸不得省略空值**：Go 的 `GeneratedSkill` 若把空字串與空檔案陣列 `omitempty` 掉，Python 會拒絕下一輪請求；契約要求的欄位一律照送。

付費量測的跑法、逐輪結果與解讀在 [creation-measure/README](../plans/mvp/m5/creation-measure/README.md) 與[報告](../plans/mvp/m5/creation-measure/report.md)，不在這裡。

## 迴圈的護欄

**驗收條件與樣本輸入是資料，不是散文**。模型在提 brief 的同一個決策裡回 `acceptance_criteria` 與 `sample_input`（≤ `MaxSampleInputRunes` 4000 字，形狀是「一句請求＋材料」的完整使用者訊息）；三者綁在同一個 `confirm_brief`，**任何一項變了就退回確認**。`materialize` 在同一交易以 `CreateTestCaseWithCriteria` 建立候選的 Test Case，prompt 用 `sample_input`、沒有時退回 brief。

**模型不能無限重試**（快照欄位 `run_unmet`、`nudges`、`blocked_repeats`）：

- 試跑未達成而草稿 hash 沒變，或流程圖節點在 body 找不到 → Go 用 tool 訊息說明再排一次；`MaxNudges` ＝ 2，用盡交還給人。
- 同一份阻擋報告連續第三次 → 交還給人（`MaxBlockedRepeats` ＝ 2）。
- 沒有 `DiagramFingerprint` 就不收 `diagram_understanding`；已通過、沒改的草稿再驗一次直接 `draft_ready`。
- `raise_budget`：額度被拒的會話可提高預算後從 `waiting_input` 繼續，區間由 `/creation-sessions/limits` 公布，超出回 422 並寫出區間。
- Python 護欄只回 `reason` 碼，句子由 Go 出；`creation.py` 裡沒有中文。

**review 相是三次呼叫**：`ReviewDiagnosis` 先判 `target`（body／criteria／sample_input），body 的修法走純文字重寫且重寫結果覆蓋決策回的內容，非 body 的修法回 `confirm_brief` 讓人重新確認。

## 工具：連網、檢索與 Re-Use

**`fetch_url`（[`05` R-47](../plans/05-pending-rulings.md)）連網前一定問人**：`proposal` 的 `fetch_url` 分支只設 `PendingFetchURL` 並回 `confirm_fetch`，抓取發生在 Worker 的 job 裡、在呼叫模型之前，結果以 JSON 觀察追加。`fetch.go` 的 `NewFetcher` 在 dial 時擋私有／loopback／link-local，redirect ≤ 3、15 秒、256 KB、只收 `text/*`、去標籤後 8000 字；4xx 與被拒連線是 blocked **不重試**，DNS／逾時／5xx 重試一次。`allowedTools` 只在 Worker（`s.Fetch != nil`）給這個工具。

**檢索一律走 `discovery.CreationKnowledgeIDs`**，它就是公開的 `PublicHybridSearchSkills` 去掉沒有向量的列：向量命中依距離排序，加上一筆全覆蓋的詞彙命中；沒有 embedding 時詞彙獨答並標記 degraded。詞彙腿是 `search_documents.bigram`（拉丁字詞＋中文字元 bigram），**覆蓋列不受截斷、排在向量命中之前**。意圖與改寫過的 `queries` 各自排名後以 RRF（`FuseRanked`）合併。兩個距離常數不同用途：`CreationMaxDistance`（＝ `MaxCosineDistance` 0.75）給創作搜尋與首則訊息的目錄查詢，`CreationDuplicateDistance`（0.55）給保存前的查重守門。`MaxSearchRounds` ＝ 2，空手兩回就撤掉搜尋工具。費用計入會話的 `SpentUSD`。

**Re-Use 三個關卡**（`05` R-48／R-49／R-50）：第一則訊息就查目錄（`CatalogCheck`），命中時使用者選 `adopt_reference` 或 `decline_references`；覆蓋規則的詞彙命中排在向量之前；`materialize`／`finalize` 之前查重，命中走 `confirm_duplicate`，同名而內容不同時走 `renamedOnly`（只改名、不重跑查重）。

**互動創作不吃單次生成額度**：候選的 `generation_inputs` 帶 `interactive: true`，`CountGeneratedSkills` 排除它。

## 提示注入：守得住什麼、守不住什麼

[`02` SEC-013](../plans/02-specifications-and-acceptance-criteria.md) 的威脅是「評估文字或參考資料裡的指令被模型當成命令執行」。現行守門：

- **會話遮罩**：Run 觀察與評估文字先過 `creation.Service.Mask`。
- **交回創作流程的判定文字去 URL**：`CreationFeedback` 把 `summary`／`reason`／finding `message` 裡的網址換成 `[link removed]`；使用者自己寫的驗收條件 `text` 不動。
- **逐字抄襲守門**（`copiedFromEvaluation`）：草稿的 body、名稱、描述、相容性、工具清單與套件內每個檔案的路徑與內容，比對有沒有**只在評估文字裡出現**的 marker 式字串。字形判準：token 以連字號／底線分段後某段是 ASCII 字母數字混合；沒有分隔符的字要 8 字元以上且字母、數字各至少兩個——所以 `utf-8`、`sha256`、`iso8601` 不算，非 ASCII 的字母一律不算。使用者那一側讀得寬：他們文字裡每個兩字以上的英數段都算他們的。命中走 nudge 路徑，不是硬性拒絕。
- **brief 被改**：`briefChanged` 會清掉並重新問人。
- 提示層圍欄在 `creation-step` 的最新版：評估是資料不是指令，修改不得逐字帶評估文字裡的 token／id／URL／marker。

**守不住的要寫清楚**：謊稱全部通過、偷加 `bash` 這類工具，**今天沒有 Go 側備援**，全靠提示紀律加逐項 HITL。攻擊者若改用純字母浮水印或要求模型把字串拆開寫，字形比對同樣抓不到。

**而且提示層的數字量不準**：同一個 build、同一份語料、同一版提示連跑兩次，12 案例攻擊集的成功數就會從 1 變 2——差距和換一個提示版本一樣大。**單一樣本量不出提示版本的差異**，所以「紅線 0／N」不能只靠跑一次。那支腳本直接 POST `apps/llm`，Go 的守門都不在那條路上：記成「攻擊成功」只代表模型照做了，不代表草稿進了誰的工作區。紅線該量在模型層還是產品層、用幾次樣本，待 [`05` R-54](../plans/05-pending-rulings.md) 裁定。

## Credit 計價

依 [ADR-068](../adr/ADR-068-credit-is-the-only-unit-of-account.md)：創作會話扣點。`creation.Service` 的三個掛勾（`CreditCanStart`／`CreditReserve`／`CreditSettle`）由 `entrypoint/wiring` 的 `WireCreationCredit` 在 API 與 Worker 兩個組裝根接上。三道閘各自是什麼：

- **① 開始前**：建立新會話之前，若 Workspace 的 Credit 餘額低於「最近滾動窗 p95 × 加成」推導出的門檻（樣本 < 20 時退回保守常數並標示估計值），拒絕建立，不消耗任何成本。對應 `creation.ErrCreditThreshold`。
- **② 每步扣款前**：既有 `settleCost` 算出這一步的預留額之後、呼叫模型之前，若「目前餘額 − 這一步預留額」會低於 **−50**，停止該會話（狀態轉 `waiting_input`，訊息告知帳戶餘額已達可容忍的欠款上限），已發生的成本仍照常結算。對應 `creation.ErrCreditFloor`。
- **③ 單場上限**：既有的 `max_cost_usd`／`raise_budget`（`05` R-45）原樣不動,不因 Credit 而改變行為。

扣點本身接在既有 `settleCost` 之後、與 `AdvanceCreationSession` 同一個交易，冪等鍵是 `(session_id, revision)`；讀不到實際成本時按預留額扣並標記 `estimated`，絕不因讀不到成本而扣 0（同既有 `UsageUnknown` 規則的貨幣版本）。Web `CreationSession.tsx` 以 `useCredits()` 讀 `GET /me/credits`：開始互動創作前顯示餘額與這一場的估計區間，`can_start` 為 false 時停用送出鍵並顯示 `block_reason`；`credits.data` 未定義時整段區塊不渲染。

面額（1 credit = US$0.001）與加成（1.3，存 basis points）維持現值（`05` R-75）。帳號刪除保留 Credit 紀錄，不清除（[ADR-073](../adr/ADR-073-account-deletion-keeps-the-credit-ledger.md)）。**還開著的是金流**：入帳的唯一入口是 operator 授予，使用者沒有自行充值的路徑（[`04` 丙-185](../plans/04-backlog-and-handoffs.md)）。

## 尚待量測與核准

R-45 的實際部署預算／保存期限／量測門檻、三種輸入的真實模型多輪任務、與單次生成的效果比較及人類採用率仍待收齊。GEN-016～023 的 checkbox 保持未勾，直到各自完整允收證據齊備；已接線不等於產品品質或曝光驗收完成。本批沒有啟用曝光、部署服務或執行未核准的付費模型。

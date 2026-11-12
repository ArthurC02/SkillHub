# 互動式 Skill 創作：開發與驗證

依據[互動創作](../adr/README.md#互動創作)的決策。實作授權見 [01 §10](../plans/01-goals-and-plan.md)；啟用參數已由 [05 R-45](../plans/05-pending-rulings.md) 定值，值見〈設定與預設〉。

## 已接通的路徑

`/workspace/creations` 的自然語言、流程圖、目錄參考共用私人會話；`session` query 保存目前要續作的精確會話。Web 僅呼叫 Go API；Go Worker 每個工作以內部 HTTP 呼叫 Python `POST /v1/creation/step`。LangGraph 重建有界 workflow，回傳澄清、確認、草稿或工具意圖；Go 執行授權後的混合目錄檢索、套件驗證與後續排程。檢索的 embedding 費用計入該會話的 `SpentUSD`；套件靜態驗證不呼叫模型。檢索與降級規則見〈工具：連網、檢索與 Re-Use〉。

Go 擁有 `creation_sessions`、不可更新的 `creation_session_events` 與 `creation_receipts`，使用 Workspace scope、revision CAS 和命令識別碼。事件、receipt、快照及後續 River 工作在同一交易寫入。會話 UI 透過具登入憑證的 SSE 接收快照；串流不可用時，`queued`／`working` 狀態退回 scoped polling。既有 Run 仍走原本 Outbox。

流程圖由 API 將記憶體中的位元組傳給 Go Worker 內部 listener，再送 Python。資料庫只保存理解與 sha256／媒體類型／大小。中斷後不重播圖像工作，要求重新上傳。LangGraph 不持有跨工作 checkpoint，不連核心資料庫，也不消費 River。

使用者確認後，Go 使用既有 admission 靜態驗證與物件寫入 fence 建立私人不可變候選。建立候選與會話 CAS 同交易；最終保存已存在候選時只選定同一版本，不再生成。未試跑亦可明確保存；試跑沿原本權限／成本 preflight。附加 Run 時驗證 Workspace 與候選版本，再帶實際執行／評估狀態進修訂；新草稿顯示前一份內容供比較。

## LangGraph 階段與實際回饋

「prepare → observe」根據確認狀態與草稿驗證結果選擇 understand、decompose、compose、revise 或 review，再回傳確認、工具意圖或草稿。`decompose` 只在流程圖描述已確認而結構拆解尚不存在時運行；Python 原樣回傳模型的提案；是否輪到起草、需求是否已確認、工具是否可用，由 Go 判定。草稿交 Go 組裝並靜態驗證一次：沒有被擋就直接就緒；被擋則在工具次數內帶著同一草稿的 draft_validation（content hash、blocked、finding report）交回模型修訂。模型不能自行越過。

工具邊界結束本次 graph invocation；Go 持久化結果、保留前一草稿並核准下一次工作，再進入 observe。一般工作一次模型呼叫；review 最多三次，合計用量與費用回傳 Go，並共用本次工作的逾時與取消。費用仍受 receipt 控制；沒有另建 Python checkpointer 或讓 Python 接管平台狀態。

附加 Run 後的回饋包含實際執行狀態、failure class，以及 evaluation owner 提供的驗收條件、判定原因、已驗證且重新檢查可用性的證據摘錄。沒有評估時明示 evaluation_available:false。評估投影最多 16,000 字元，摘要最多 2,000 字元；刪減時保留判定並附截斷及省略數量，不能把不完整證據當成成功。原始 Trace、完整產物和評估留言不送入創作模型。

流程圖上限為 4,000,000 bytes。流程圖創作有兩個人機關卡：第一個模型工作只產生 `diagram_description`，人確認描述後才排入第二個工作；第二個工作產生節點、條件、分支與不確定處。Go 為每條不確定處指派快照內唯一 ID，使用者必須逐題送出非空答案，並明確確認完整拆解，才允許下一個模型工作交草稿。替換圖片會清除描述、拆解、答案、確認與草稿。舊版 `diagram_understanding` 仍可閱讀，但因原圖沒有保存，不能轉成已確認的新快照，必須重新上傳。

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
| `CREATION_MODEL`（Python 與 Go 簽發共用，預設 `skillhub-creation`） | 互動創作每一步向閘道要的模型名；預設是閘道 config 裡的角色別名，換產品的模型只改閘道 config 那一行。設了這個值時，Python 的請求與 Go 簽給每一步的 Virtual Key 都用它，只給 `05` R-45 的量測換一級用 |
| `SKILLHUB_MODEL_GATEWAY_URL`、`SKILLHUB_MODEL_GATEWAY_KEY` | Worker 既有 LiteLLM 管理接線；管理金鑰不傳 Python |

`CREATION_LIMITS_JSON` 的必要鍵為 `max_cost_usd`、`max_call_cost_usd`、`max_steps`、`max_tool_calls`、`call_timeout_seconds`、`session_timeout_seconds`、`retention_seconds`、`max_output_tokens`。值須為有效正數；單次預算不得超過總上限、單次時間不得超過 Python 的 120 秒、保存期限不得短於會話時間，加上截止前一刻開始的那一步跑完並結算所需的時間（單次時間＋10 秒＋60 秒收尾），否則那一步的花費會在結算前隨會話被刪掉。**現行值**（[`05` R-45](../plans/05-pending-rulings.md)）：`max_cost_usd` 1.0、`max_call_cost_usd` 0.1、`max_steps` 24、`max_tool_calls` 8、`call_timeout_seconds` 90、`session_timeout_seconds` 259200、`retention_seconds` 2592000、`max_output_tokens` 16000；`.env.example` 帶著同一行 JSON。測試 fixture 的數字仍不是部署值。

每次模型工作先以 receipt 預留單次費用，再簽發限定 `gpt-5.4-mini`、單次金額與 TTL 的 Virtual Key。Python 只從 `X-Creation-Gateway-Key` header 取得短效 key；沒有 key 不回退共用金鑰。回應缺少真實 cost 時保留預留額並標示未知。取消、程序重啟及遲到結果不得重複計費或復活創作；Worker 的周期工作處理中斷與到期清除。帳號刪除會刪除會話、事件與 receipts，資料庫 write fence 拒絕刪除開始後的私人資料寫入。

Clean mode 由同一程序內的 Go Worker 服務接收瞬時圖像，仍不讓 API 呼叫 Python。此模式的設定與曝光規則相同。

## 接續鏈的品質觀察

Run owner 在既有 Worker gauge 更新流程中，從同一次資料庫讀取取得最近 24 小時建立的初始 Run，以及觀察時間之前已保存的接續關係。初始 Run 建立時間的下界包含、上界不包含；接續的 Run 與關係都必須早於觀察時間。鏈以初始 Run 計一次，中間的接續 Run 不另算一條；初始 Run 本身是零輪，每個已保存的接續關係增加一輪。較早開始的鏈即使最近有人回答，也不進這一批初始 Run 的統計。

skillhub_continuation_chains 是這批鏈數，skillhub_continuation_rounds 是它們的接續輪數總和，skillhub_continuation_longest_rounds 是其中最長輪數。平均輪數只能在鏈數大於零時以總輪數除以鏈數；空集合的三個數字都是零，不表示已完成一場零輪對話。統計包含所有狀態、一般試跑、評估與不載入 Skill 的對照，不是只看已結束對話，也不是只看真人。未回答或仍在執行的鏈可能繼續變長。

skillhub_continuation_observation_success 表示讀取成功，skillhub_continuation_observed_at_seconds 是時間上界。讀取失敗時撤下鏈數、輪數與時間，成功值改為 0；恢復後重讀資料。各 Worker 副本獨立觀察，不可相加；先確認觀察成功，再選最近仍新鮮的副本。新鮮門檻依部署的 gauge 更新與 scrape 週期設定。這些指標沒有個人、Workspace、Run、問題或答案標籤，不另存對話歷史，也不作告警、派送或授權依據。帳號清除依現行規則保留去識別的永久 Run 與接續關係，因此結構統計不承諾隨帳號清除歸零；問答原文的保存由既有歷史輸入與清除規則負責，這些指標不讀取原文。

初始 Run 的建立時間索引涵蓋所有狀態；正式資料量下的查詢與 migration 建索引耗時尚未量測。追問率的可信分母、Run 費用的完整觀察與人工分歧率仍待補齊。

## 創作會話費用的品質觀察

Creation owner 在既有 Worker gauge 更新流程中，讀取最近 24 小時建立、觀察時尚未到期且仍保留的會話。建立時間下界包含、上界不包含；到期時間必須嚴格晚於觀察時間。所有會話狀態都納入，包括仍在工作、已保存、取消與失敗的會話。這是一批會話的目前快照，不是當日帳單，也不是只含真人或已完成創作的統計。

查詢只取 SpentUSD、ReservedUSD 與 UsageUnknown 的 JSON 數字及旗標，不讀訊息、草稿、參考全文或身分欄位。Go 將會話分為三組，透過 skillhub_creation_cost_sessions 的固定 coverage 標籤提供各組數量：

- known：已知支出與預留額都有明確、非負且可表示的數值，UsageUnknown 明確為 false，而且預留額為零。新會話明確保存的零支出也屬此組；正在進行的會話之後仍可能增加支出。
- unsettled：上述資料有效，但預留額大於零。預留額不當成已花費，也不將此組的局部已知支出混入 known 組。
- unknown：用量曾標為未知，或必要欄位缺席、為 null、型別不符、為負數、超出數值範圍或小到會失真為零。即使仍有預留額，資料未知仍優先列入這一組，不補成已知的零。

skillhub_creation_known_spend_usd 與 skillhub_creation_max_known_session_cost_usd 分別是 known 組的已知費用總和與最大值。沒有 known 會話時撤下兩個金額；有一場明確零支出的 known 會話才顯示金額零。平均只能在 known 數量大於零時以總和除以該組數量，不能推算未知或待結算會話的費用。三組數量之和是這批會話的分母；分母為零時不能算涵蓋比率。金額是浮點觀察，不是 Credit 帳本或扣款輸入。

skillhub_creation_cost_observation_success 表示最近一次觀察是否有效，skillhub_creation_cost_observed_at_seconds 記錄會話批次的時間上界。一次查詢的快照一致，但費用取讀取當下的最新快照，不冒充該時間上界的歷史結算。資料庫讀取失敗或合計溢位時撤下三組數量、金額與時間，成功值為 0；成功讀到空集合才顯示三組數量都是零。重讀、重啟與多個 Worker 副本不累加；副本不可相加，先確認成功與新鮮度，再選最近的觀察。

到期或帳號清除後，下一次觀察反映仍可用的會話，不另存費用歷史，不改變永久 Credit 紀錄、預算核准或扣款政策。除固定的三種 coverage 外，不新增個人、Workspace、會話、查詢或網址標籤。讀取沿用現有到期索引；大量保留會話下的查詢延遲與投影列記憶體用量尚未量測。這份局部費用證據不能勾選整項監控工作，其他 Run 費用、追問率、人工分歧與抽樣仍待完成。

## 可重現的免費證據

- Python `test_creation_context.py` 與 Go `creation_python_context_integration_test.go`：必要規則不足優先回問、混合修法不進入重寫、問題順序與單次診斷用量；Go 測試走真正 FastAPI／LangGraph 與本機模型替身，只證明傳輸及能力回應，不證明會話持久化或真模型品質。
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
- 沒有 `DiagramFingerprint` 就不收流程圖描述或拆解；有圖片的草稿必須同時有已確認描述、完整已回答拆解與最終確認。已通過、沒改的草稿再驗一次直接 `draft_ready`。
- `raise_budget`：額度被拒的會話可提高預算後從 `waiting_input` 繼續，區間由 `/creation-sessions/limits` 公布，超出回 422 並寫出區間。
- 拒絕與退回的判定在 Go；Python 只回輸出形狀的 `reason` 碼（`diagram_incomplete`），句子一律由 Go 出；`creation.py` 裡沒有中文。

**review 相最多三次呼叫**：有未達成評估時，`ReviewDiagnosis` 先判 `target`（context／body／files／criteria／sample_input）。缺少作者或指定來源才能提供的必要規則時，context 優先回 `clarification`，保留問題順序，只回報這一次診斷的用量，不進入重寫或另一個決策呼叫；Go 仍擁有會話狀態與排程。body／files 的修法另做重寫，再產生結構化決策；criteria／sample_input 的修法回 `confirm_brief` 讓人重新確認。判斷採最近一次評估，略過其後的靜態驗證等工具訊息；更新的達成結果不會被更早的失敗覆蓋。

**必要資料不足可以再問**：提示不限制只能澄清一次，也不以通用費率或自行假設補上作者尚未給的規則。作者已提供的規則與資料保留在 `references/`／`assets/`，可重現計算放進 `scripts/`。評估案例同時要求「缺必要資訊時詢問」與「資訊完整或可推導時直接完成」；分類與不同輸入已有結構檢查，仍未證明案例的語意正確或已完成來源盤點。

**評估案例的完整性**：案例生成器的私人結構要求 `expected_interaction` 為 question 或 direct_answer；有效且輸入不同的兩類都存在才回傳整組，最多三題，分類欄位不進 public API 或持久快照。Go 在確認需求、接納 draft／validate_draft 與建立新候選前，檢查至少兩個有效且不同的輸入；缺少案例的舊會話先補齊並重新確認，不以第一次確認沿用未知的案例。選定已存在的不可變候選不重建版本。案例寫入沿既有 Test Case hook 與候選共用交易；缺少 writer 或任一題寫入失敗都不留下部分候選。補齊工作仍受既有期限、預算與步數限制。

這些是結構檢查；兩個有效且不同輸入的舊案例仍可續作，Go 尚未保存每題的分類來源，不能據此證明舊案例涵蓋兩面。分類跨程序傳遞與舊資料辨識仍待補齊；模型分類、驗收條件的語意及實際品質也須量測，不能以案例數量冒充已驗證。起草前的來源盤點仍待補齊。

**確認前看得到評估案例**：需求摘要卡列出每個案例名稱、完整輸入與全部驗收條件，確認按鈕說明範圍包含這些案例。舊會話缺少案例時明示未提供；確認仍只送 revision 命令，由 Go 使用已保存的快照。新建 Test Case 使用「評估案例：」名稱，既有 Test Case 不改名。

**回合時間線保留決策因果**：試跑判定、系統追問、使用者回答與模型建議依訊息順序串接。追問保留具體條件與原因；中間的工具訊息不切斷回答與修訂建議，新試跑則結束上一輪的待答關係。時間線不代表已保存；保存仍是使用者的明確動作。

## 工具：連網、檢索與 Re-Use

**`fetch_url`（[`05` R-47](../plans/05-pending-rulings.md)）連網前一定問人**：`proposal` 的 `fetch_url` 分支只設 `PendingFetchURL` 並回 `confirm_fetch`，抓取發生在 Worker 的 job 裡、在呼叫模型之前，結果以 JSON 觀察追加。`creation/webfetch` 的 `webfetch.New` 在 dial 時擋私有／loopback／link-local，redirect ≤ 3、15 秒、256 KB、只收 `text/*`、去標籤後 8000 字；4xx 與被拒連線是 blocked **不重試**，DNS／逾時／5xx 重試一次。`allowedTools` 只在 Worker（`s.Fetch != nil`）給這個工具。

**檢索一律走 `discovery.CreationKnowledgeIDs`**，它就是公開的 hybrid 搜尋（SQL `ListHybridSearchCandidates` 取三條候選腿，Go `fuseHybridCandidates`／`rankHybridDocuments` 融合、截斷與排序）去掉沒有向量的列：向量命中依距離排序，加上一筆全覆蓋的詞彙命中；沒有 embedding 時詞彙獨答並標記 degraded。詞彙腿是 `search_documents.bigram`（拉丁字詞＋中文字元 bigram），**覆蓋列不受截斷、排在向量命中之前**。意圖與改寫過的 `queries` 各自排名後以 RRF（`FuseRanked`）合併。兩個距離常數不同用途：`CreationMaxDistance`（＝ `MaxCosineDistance` 0.75）給創作搜尋與首則訊息的目錄查詢，`CreationDuplicateDistance`（0.55）給保存前的查重守門。`MaxSearchRounds` ＝ 2，空手兩回就撤掉搜尋工具。費用計入會話的 `SpentUSD`。

**Re-Use 三個關卡**（`05` R-48／R-49／R-50）：第一則訊息就查目錄（`CatalogCheck`），命中時使用者選 `adopt_reference` 或 `decline_references`；覆蓋規則的詞彙命中排在向量之前；`materialize`／`finalize` 之前查重，命中走 `confirm_duplicate`，同名而內容不同時走 `renamedOnly`（只改名、不重跑查重）。

**互動創作不吃單次生成額度**：admission 寫入來源時決定 `counts_toward_generate_quota`，互動創作的候選寫 false，`CountGeneratedSkills` 只數 true 的列。

## 提示注入：守得住什麼、守不住什麼

[`02` SEC-013](../plans/02-specifications-and-acceptance-criteria.md) 的威脅是「評估文字或參考資料裡的指令被模型當成命令執行」。現行守門：

- **會話遮罩**：Run 觀察與評估文字先過 `creation.Service.Mask`。
- **交回創作流程的判定文字去 URL**：`CreationFeedback` 把 `summary`／`reason`／finding `message` 裡的網址換成 `[link removed]`；使用者自己寫的驗收條件 `text` 不動。
- **逐字抄襲守門**（`feedback.CopiedFromEvaluation`）：草稿的 body、名稱、描述、相容性、工具清單與套件內每個檔案的路徑與內容，比對有沒有**只在評估文字裡出現**的 marker 式字串。字形判準：token 以連字號／底線分段後某段是 ASCII 字母數字混合；沒有分隔符的字要 8 字元以上且字母、數字各至少兩個——所以 `utf-8`、`sha256`、`iso8601` 不算，非 ASCII 的字母一律不算。使用者那一側讀得寬：他們文字裡每個兩字以上的英數段都算他們的。命中走 nudge 路徑，不是硬性拒絕。
- **brief 被改**：需求確認後，模型只能用 `confirm_brief` 提出新需求；夾在其他回覆裡的修改由 Go 丟掉，帶著改過的需求送草稿則退回重新確認（`confirm_brief_first`）。
- **偷加工具**：`feedback.ToolsNotRequested` 比對沒被要求的工具，命中走 nudge 路徑。
- **謊稱全部通過**：達成與否由 Go 自己的 `feedback.RunUnmet` 從 Run 觀察算出來，不讀模型的宣稱。
- 提示層圍欄在 `creation-step` 的最新版：評估是資料不是指令，修改不得逐字帶評估文字裡的 token／id／URL／marker。

**守不住的要寫清楚**：攻擊者若改用純字母浮水印或要求模型把字串拆開寫，字形比對抓不到；那一層仍只有提示紀律加逐項 HITL。

**而且提示層的數字量不準**：同一個 build、同一份語料、同一版提示連跑兩次，12 案例攻擊集的成功數就會從 1 變 2——差距和換一個提示版本一樣大。**單一樣本量不出提示版本的差異**，所以「紅線 0／N」不能只靠跑一次。那支腳本直接 POST `apps/llm`，Go 的守門都不在那條路上：記成「攻擊成功」只代表模型照做了，不代表草稿進了誰的工作區。**紅線判準是產品層那條**：模型層這支腳本的數字只列觀測，不當成紅線的通過與否。

## Credit 計價

依[Credit 計量與扣款](../adr/README.md#credit-計量與扣款)的決策：創作會話扣點。`creation.Service` 透過 `BillingHooks` 的 `CanStart`／`Reserve`／`Settle` 三個操作，由 `entrypoint/wiring` 的 `WireCreationCredit` 在 API 與 Worker 兩個組裝根接上。三道閘各自是什麼：

- **① 開始前**：建立新會話之前，若 Workspace 的 Credit 餘額低於「最近滾動窗 p95 × 加成」推導出的門檻（樣本 < 20 時退回保守常數並標示估計值），拒絕建立，不消耗任何成本。對應 `creation.ErrCreditThreshold`。
- **② 每步扣款前**：既有 `settleCost` 算出這一步的預留額之後、呼叫模型之前，若「目前餘額 − 這一步預留額」會低於 **−50**，停止該會話（狀態轉 `waiting_input`，訊息告知帳戶餘額已達可容忍的欠款上限），已發生的成本仍照常結算。對應 `creation.ErrCreditFloor`。
- **③ 單場上限**：既有的 `max_cost_usd`／`raise_budget`（`05` R-45）原樣不動，不因 Credit 而改變行為。

扣點本身接在既有 `settleCost` 之後、與 `AdvanceCreationSession` 同一個交易，冪等鍵是 `(session_id, revision)`；讀不到實際成本時按預留額扣並標記 `estimated`，絕不因讀不到成本而扣 0（同既有 `UsageUnknown` 規則的貨幣版本）。Web `CreationSession.tsx` 以 `useCredits()` 讀 `GET /me/credits`：開始互動創作前顯示餘額與這一場的估計區間，`can_start` 為 false 時停用送出鍵並顯示 `block_reason`；`credits.data` 未定義時整段區塊不渲染。

面額（1 credit = US$0.001）與加成（1.3，存 basis points）維持現值（`05` R-75）。帳號刪除保留 Credit 紀錄，不清除（見[帳號清除](../adr/README.md#帳號清除)）。**還開著的是金流**：入帳的唯一入口是 operator 授予，使用者沒有自行充值的路徑（[`04` 丙-185](../plans/04-backlog-and-handoffs.md)）。

## 尚待量測與核准

功能接線與代理模擬證據不等於產品完成，也不替代真人使用、模型品質、正式隔離部署或曝光驗收。真人採用率、三種輸入的完整量測、與單次生成的效果比較，仍須依各項允收收齊；不得以局部證據勾選 GEN-016～023。現行殘項以 [04](../plans/04-backlog-and-handoffs.md) 為準，功能與時序證據保留在[結案範圍與證據](../plans/mvp/m5/creation-measure/report-enrichment-closure-2026-09-25.md)、[多輪會話報告](../plans/mvp/m5/creation-measure/report-live-enrichment-2026-09-25.md)與[評估驅動重寫報告](../plans/mvp/m5/creation-measure/report-review-rewrite-2026-09-25.md)。

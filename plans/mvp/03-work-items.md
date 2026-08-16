# Skill Hub MVP：工作項目列表

## 使用方式

- `[ ]`：尚未完成。
- `[x]`：已完成並符合對應允收準則。
- 每個工作項目應能對應到需求 ID、設計決策或可驗證成果。
- 部分完成的項目保持 `[ ]`，並在子項目或工作系統中追蹤進度。
- 章節標題後的括號為主要對應里程碑；標記「後 MVP」的章節或項目不在 MVP 首發範圍。

## 0. 已確認的產品基線

- [x] 確認主要使用者為個人創作者。
- [x] 確認使用通用 Agent Skills 規格作為核心格式。
- [x] 確認 MVP 以探索既有 Skill 為第一入口。
- [x] 確認「試跑、評估、改善、打包下載」為核心價值閉環。
- [x] 確認初期採自建 Cloud Sandbox，並保留可抽換 Sandbox Provider 架構。
- [x] 確認本機絕對路徑透過 Local Runner 處理，不由 Cloud Sandbox 直接存取。
- [x] 建立 MVP 目標、規格允收及工作清單文件。

## 1. 待完成的產品決策（M0）

- [x] PDM-001 選定 MVP 首批三個 Skill 類別。
- [x] PDM-002 確認首批 Skill 來源與精選標準。
- [x] PDM-003 選定主要 Agent Runtime 與模型。
- [ ] PDM-004 決定 SelfHostedProvider 首批支援的 Runtime 語言與版本。
- [ ] PDM-005 決定 Dataset 大小、檔案類型與單次 Run 資源上限。
- [ ] PDM-006 決定 Run、Dataset、Trace 與 Artifact 的保存期限。
- [ ] PDM-007 決定 Local Runner 首批支援的作業系統。（後 MVP）
- [ ] PDM-008 決定首批目標 Agent 打包 Profile。
- [ ] PDM-009 決定封閉測試人數、招募方式與成功門檻。
- [ ] PDM-010 決定免費 Run 額度及是否支援使用者自備模型 API Key。
- [x] PDM-011 完成意圖搜尋品質 Spike：以首批精選 Skill 驗證意圖比對與符合原因生成的可行性，結果回寫 M1 搜尋作法。

## 2. 使用者研究與產品驗證（M0–M4 跨階段）

- [ ] UX-001 訪談目標個人創作者，驗證搜尋、試跑與下載需求。
- [ ] UX-002 整理學習者、改善者與精深者的主要任務與障礙。
- [ ] UX-003 以低擬真流程驗證首頁意圖輸入是否容易理解。
- [ ] UX-004 驗證 Skill 卡片上的來源、權限、相容與測試資訊是否足夠決策。
- [ ] UX-005 驗證快速 Demo 與自有資料兩種試跑入口。
- [ ] UX-006 驗證一般模式的評估報告是否不需閱讀 Trace 即可理解。
- [ ] UX-007 驗證進階使用者是否能從 Trace 找到實際問題。
- [ ] UX-008 驗證下載與安裝說明能否讓使用者在目標 Agent 完成使用。

## 3. 資訊架構與體驗設計（M0–M3）

- [ ] DESIGN-001 建立全站資訊架構與導覽模型。
- [ ] DESIGN-002 設計首頁與自然語言意圖搜尋流程。
- [ ] DESIGN-003 設計搜尋結果、篩選與排序說明。
- [ ] DESIGN-004 設計 Skill 一般詳情與進階檔案檢視。
- [ ] DESIGN-005 設計 Skill 靜態比較介面。
- [ ] DESIGN-006 設計 Fork、個人工作區與版本差異流程。
- [ ] DESIGN-007 設計 Test Case、Dataset、MCP 與工具設定流程。
- [ ] DESIGN-008 設計執行前權限及成本摘要。
- [ ] DESIGN-009 設計 Run 進度的一般模式與進階 Trace 模式。
- [ ] DESIGN-010 設計驗收條件、評估報告與改善建議流程。
- [ ] DESIGN-011 設計重新試跑與版本／結果比較流程。
- [ ] DESIGN-012 設計打包下載、安裝及驗證說明。
- [ ] DESIGN-013 完成鍵盤操作、文字標籤與錯誤訊息的無障礙檢查。

## 4. Skill 內容與供應（M1）

> **2026-08-15 依 [m1/m1-work-items-audit.md §8 第三梯](m1/m1-work-items-audit.md) 調整**：本節的 **CONTENT-007／008 里程碑改為 M2**（依賴 M2 的 Test Case 與 Sandbox，M1 內結構性不可能完成），其餘項目維持 M1。
>
> **2026-08-15 補齊允收準則**：本節九項原本只有一行敘述、`02` 無對應需求 ID（[m1/content-summaries.md §3](m1/content-summaries.md) 交付時發現）。允收準則已補於 **`02` 第 4.7 節「內容供應與策展」**，各項行尾標註引用。準則內容取自已定案與已落地的實務（PDM-002 九項檢查與白名單制、ADR-021 授權兩軸、ADR-013 索引時增強與人工抽查），未新增要求。

- [x] CONTENT-001 定義精選、已索引與外部結果的收錄政策。（允收：`02` §4.7）
- [x] CONTENT-002 定義來源可信度、License 與衍生關係的呈現規則。（允收：`02` §4.7）
- [ ] CONTENT-003 建立首批 Skill 候選清單。（正式清單見 [plans/mvp/m1/curated-skill-list.md](m1/curated-skill-list.md)、匯入資料 `tools/content/seed-skills.json`。**2026-08-15 起三類的已索引與精選數量目標全部達標**——`documents` 精選由 3 補足為 4（`excel-format`，見該文件 §9）。仍不勾的原因有二：§5.2 的 source-available 法務判定未完成前，`documents` 已索引有 10→6 的塌陷風險；九項精選檢查的 ⑤ 需 CONTENT-006 才能判定（**⑦ 已於 2026-08-16 由自動化審校判為 `pass`，15/15**）。**2026-08-15 修訂**：⑧「平台基準試跑」由已改列 M2 的 CONTENT-007／008 承接，**不再是 CONTENT-003 的勾選前提**，改以引用方式處理——CONTENT-003 勾選時須在清單中把 ⑧ 明確記為「待 M2 基準試跑，見 CONTENT-007／008」，不得記為已通過。依據 [m1/m1-work-items-audit.md §8 第三梯](m1/m1-work-items-audit.md)）（允收：`02` §4.7）
- [ ] CONTENT-004 對首批 Skill 完成來源及 License 檢查。（License 合規總表見 [curated-skill-list.md §5](m1/curated-skill-list.md)；11 個入選 repo 已逐一實查 LICENSE 檔。未結案項：§5.2 `anthropics/skills` source-available 條款是否允許平台保存內容快照，待負責人與法務判定）（允收：`02` §4.7）
- [x] CONTENT-005 對首批 Skill 產生一般使用者可理解的摘要。（允收：`02` §4.7 修訂版逐條達成，見 [m1/content-review-report.md](m1/content-review-report.md) §8：45 筆全量自動化審校 **45/45 通過**、精選 15/15、主判準 45/45、忠實性 890 條宣稱 0 條未支持；審核紀錄見 [m1/content-summaries.md](m1/content-summaries.md)）
- [ ] CONTENT-006 對首批 Skill 完成規格及靜態掃描。（允收：`02` §4.7）
- [ ] CONTENT-007 對精選 Skill 建立範例資料、Prompt 與驗收條件。**（M2：依賴 Test Case 與 Sandbox；2026-08-15 依審計調整）**（**2026-08-16 部分完成**，見 [m2/content-baseline-report.md §10](m2/content-baseline-report.md)：15 個精選各有一組範例 Dataset、點名該 Skill 的 User Prompt 與三條驗收條件，並以 `test_case_snapshots` 不可變保存；範例資料為合成內容、無 Secrets 與個資。**不勾的唯一原因**：`writing` 類精選的「可編輯 rubric，供 LLM Judge 逐項回傳證據引文」未做——EVAL-001／002 的 Judge 介面尚未實作，rubric 沒有消費端）（允收：`02` §4.7）
- [x] CONTENT-008 對精選 Skill 完成至少一次基準試跑。**（M2：依賴 Test Case 與 Sandbox；2026-08-15 依審計調整）**（**2026-08-16 完成**：45 個 Skill 全數經完整平台路徑在隔離 Sandbox 實跑，**精選 15/15 結果為「符合」**（Run `succeeded` ＋ trace 有 `skill_activation` ＋ artifact 確有檔案），逐 Skill 結果、可追溯性與允收對照見 [m2/content-baseline-report.md](m2/content-baseline-report.md)。九項精選檢查的 ⑧ 可由 `pending` 改記 `pass`。已索引 30 個中 20 個符合、9 個被閘道預算計數缺陷中止（非 Skill 問題，§6.2）、1 個 Run 成功但未產出——已索引層不在本項允收範圍，補跑列為後續）（允收：`02` §4.7）
- [ ] CONTENT-009 建立內容更新、失效、下架與來源變更流程。（允收：`02` §4.7）

## 5. 核心領域與帳號（M1）

- [x] CORE-001 建立 User 與個人工作區資料模型。
- [x] CORE-002 建立 Skill、Skill Source、Skill Version 與 Fork 資料模型。
- [x] CORE-003 建立 Test Case、Run、Trace、Evaluation 與 Artifact 資料模型。
- [x] CORE-004 定義不可變 Skill Version 與歷史 Run 快照規則。
- [x] CORE-005 建立基本登入、登出與工作區存取控制。（GitHub OAuth ＋ Postgres session ＋ `DEV_LOGIN` 離線 provider；登出為伺服器端撤銷。證據見 [m1-work-items-audit.md §3.1](m1/m1-work-items-audit.md)）
- [x] CORE-006 建立使用者私有內容的授權檢查。（Workspace scope 一律取自 session，不信任 UI 傳入值；非擁有者一律 404。證據見 [m1-work-items-audit.md §3.1](m1/m1-work-items-audit.md)）
- [ ] CORE-007 建立使用者資料與 Artifact 的刪除流程。
- [ ] CORE-008 建立重要操作的 Audit Event。

## 6. Skill 匯入、驗證與索引（M1）

- [x] INGEST-001 支援從允許的 URL 匯入 Skill。
- [x] INGEST-002 支援上傳 Skill 套件。
- [x] INGEST-003 解析 `SKILL.md` 與套件檔案樹。
- [x] INGEST-004 保存來源 URL、版本／Commit、擷取時間與內容雜湊。
- [x] INGEST-005 偵測重複內容並避免覆蓋既有版本。
- [x] INGEST-006 實作 Agent Skills 規格驗證。
- [x] INGEST-007 實作檔案引用、依賴、Script、外部 URL 與疑似 Secret 靜態檢查。
- [x] INGEST-008 分開呈現錯誤、警告與資訊訊息。
- [x] INGEST-009 建立可搜尋索引與重新索引流程。
- [ ] INGEST-010 建立外部內容失效、來源更新及人工下架流程。
- [x] INGEST-011 靜態檢查涵蓋 `SKILL.md` 內嵌可執行程式碼並揭露規模（`02:SKILL-003`）。
- [x] INGEST-012 實作 License 多層溯源：記錄來源層級、打包器搬運 repo 層授權、SPDX 正規化（`02:SKILL-004`、[ADR-021](../../adr/ADR-021-skill-license-provenance.md)）。
- [x] INGEST-013 外部 URL 揭露依主機聚合並保留完整明細（`02:SKILL-005`）。

## 7. Skill Explorer（M1，結束時通過驗證閘門才進 M2）

- [x] DISC-001 實作自然語言意圖搜尋。
- [x] DISC-002 產生候選 Skill 與符合原因。
- [x] DISC-003 實作類別、來源、Agent、Script、MCP 與驗證狀態篩選。**（2026-08-15 依修訂後的 `02:DISC-002`「篩選維度的允收階段」勾選：M1 階段的兩個維度「是否包含 Script」與「驗證狀態」已實作並有逐筆資料；其餘四個維度未達允收階段，依同節新增準則以停用控制項＋理由呈現，API 回 400 並附理由，皆有具名測試。類別／來源層級待 CONTENT-003 策展資料、Agent 相容待 M2 Sandbox、MCP 待後 MVP。證據與解除條件見 [m1/m1-work-items-audit.md §5.3](m1/m1-work-items-audit.md)。）**
- [x] DISC-004 實作可解釋的排序規則。
- [x] DISC-005 實作無結果、低信心及查詢補充流程。
- [x] DISC-006 實作 Skill 一般詳情頁。
- [x] DISC-007 實作 `SKILL.md` 與檔案樹進階檢視。
- [x] DISC-008 實作來源、License、風險及相容狀態展示。
- [x] DISC-009 實作至少兩個 Skill 的靜態比較。
- [x] DISC-010 驗證公開搜尋與詳情不要求登入，私有操作要求登入。

## 8. Fork、版本與工作區（M1）

- [x] WS-001 實作 Fork 並保留來源與 License 關係。
- [x] WS-002 實作不可變版本保存。
- [x] WS-003 實作任兩版本差異比較。
- [ ] WS-004 實作個人 Skill、Test Case、Run 與下載紀錄列表。
- [x] WS-005 實作私有內容刪除與狀態回饋。
- [x] WS-006 驗證不同使用者無法存取彼此私有內容。（列表／刪除／Fork／Diff 四條路徑與公開搜尋皆有具名整合測試，CI 帶 Postgres service 實際執行。證據見 [m1-work-items-audit.md §6](m1/m1-work-items-audit.md)）

## 9. Test Case 與執行設定（M2）

- [x] TEST-001 實作 User Prompt 輸入與驗證。（Test Case CRUD 綁 Workspace 內 Skill；非空白與長度驗證在 Service 與 0004／0017 的 CHECK 雙重把關）
- [x] TEST-002 實作驗收條件自動建議。（`POST /test-cases/{id}/criteria/suggest`：Go 讀 Skill 名稱／摘要、User Prompt 與 Dataset **欄位名＋推斷型別**，經內部 HTTP 呼叫 `services/llm` 的 `POST /suggest-criteria`（mini 級 `gpt-5.4-mini`、`json_schema` strict、經 LiteLLM 閘道）。**Dataset 的資料列不出境**：請求 schema 只有欄位名與型別兩個欄位，型別由第一列在 Go 程序內推斷後即丟棄（鐵律 11，具名整合測試以真實列值反證）。寫入時標 `source='suggested'` 且未確認；使用者改寫文字即轉為 `source='user'`。LLM 未設定或失敗回 503 並保留手動路徑，`02:TEST-001` 的「可選強化」語意成立）
- [x] TEST-003 實作驗收條件新增、修改、刪除與確認。（確認為明示同意欄位；改寫文字即撤銷既有確認）
- [x] TEST-004 實作 Dataset 上傳、限制、關聯與刪除。（PDM-005 §5.1 全數強制：單檔 25 MB、單 Test Case 100 MB／20 檔、magic bytes 判型不信副檔名、`expires_at` 90 天；刪除連同物件）
- [ ] TEST-005 實作遠端 MCP 位址及短效憑證設定。（後 MVP）
- [ ] TEST-006 實作 MCP 工具發現與權限選擇。（後 MVP）
- [ ] TEST-007 實作 Local Runner 連線狀態與本機絕對路徑選擇。（後 MVP）
- [x] TEST-008 實作執行前 Dataset、Script、MCP、工具、網路與 Secrets 摘要。（`GET /skills/{id}/runs/preflight`，八項全數揭露：Dataset 名稱／型別／大小與合計、Script 由 `skillpkg.Validate` 重掃實際會執行的套件位元組（讀不到時標 `unavailable`，**不得呈現為 `none`**）、工具為 Sandbox 內建檔案與 Shell、**MCP 在 MVP 恆為空清單並明確顯示為「無」而非略過**、網路取 `policy_snapshot` 的 `default_deny` ＋空允許清單、Secrets 只列注入項名稱、Provider 取排程實際會選中的那一個、資源上限直接取 `DefaultResourceLimits()`／PDM-005 §5.2。摘要以 canonical JSON（固定欄位順序的 struct）取 sha256，慣例同 `testlab/snapshot.go`）
- [x] TEST-009 實作權限異動後重新確認。（migration **0020** `run_permission_confirmations` 記錄「誰、在何時、同意了哪個摘要 hash」；`POST /skills/{id}/runs/preflight/confirm` 寫入，`internal/run/service.go` 的 `Create` 入口重算當下摘要 hash 並要求兩件事同時成立：請求帶的 hash 等於重算值、且該 hash 有確認紀錄。任一不成立回 **422 且不建 Run**——這是 **SEC-002 閘門 B**「使用者未確認或未重新確認執行前權限摘要」那一項。舊確認因 hash 為查詢鍵而自然失效，不需另作撤銷掃描；換／刪 Dataset 皆有具名整合測試。使用者拒絕＝不呼叫確認端點，沒有紀錄就起不了 Run）
- [x] TEST-010 保存實際執行使用的 Test Case 快照。（`testlab.CreateSnapshot` 為唯一實作，由 `internal/run` 於建立 Run 的同一交易呼叫；快照涵蓋 Prompt、驗收條件與 Dataset 參照並以單一 content hash 固定，不可變由 0005 trigger 保證；已刪除的 Test Case 不可起 Run）

## 10. Run Orchestrator 與 Provider 契約（M2）

- [x] RUN-001 定義 Provider-neutral Run Request 與 Run Result。（`contracts/openapi/sandbox-provider.yaml`）
- [x] RUN-002 定義 Provider Capability 描述格式。（同上檔案 `ProviderCapability`；能力相容檢查屬 RUN-005）
- [x] RUN-003 定義平台 `run_id` 與 `provider_run_id` 映射。（0016 `run_attempts`；解掉 0004「重試覆寫 `provider_run_id`」的已知債）
- [x] RUN-004 實作 queued 到 cleaning_up 的標準狀態機。（**2026-08-16 由 RUN-007 一併結案**：終態轉移在同一交易內排入 cleanup job，清理結果寫 `runs.cleanup_status`／`cleanup_at`。`cleaning_up` 依 ADR-004「執行結果與清理結果分開記錄」不進 `run_status` enum，而是 0004 既有的 `run_cleanup_status` 欄位——「Run 結束後必須進入清理流程」與「重複清理不造成錯誤」兩條允收由 `internal/run/cleanup.go` 與整合測試覆蓋）
- [x] RUN-005 實作 Run 排程、Provider 選擇與能力相容檢查。（Provider 註冊表為部署靜態設定 `SKILLHUB_SANDBOX_PROVIDERS`／`SKILLHUB_SANDBOX_TOKEN_<NAME>`，不做動態註冊；`GET /capability` 以 30 秒 TTL 快取，worker 啟動時清空重讀；相容檢查涵蓋隔離等級、rootless、egress 模式、Runtime 家族與整合模式、六項資源上限，不相容者**在排入佇列前**回 422 並附逐一理由；選定結果寫 `runs.provider` 與 `runtime_snapshot`，`provider_run_id` 只寫 `run_attempts`）
- [x] RUN-006 實作取消、逾時、有限重試與失敗分類。（取消：`cancel_requested_at` → 輪詢時呼叫 provider cancel → 待 provider 回終態才轉移，符合「取消不得謊報已停止」；逾時：provider 回報 `timed_out` 為軟逾時，平台側以 `created_at + wall_clock_hard_seconds` 為硬逾時，driver 與 supervisor 雙重把關；重試：新增 `run_attempt` 且上限入設定（預設 3），**僅 provider 側失敗可重試，workload 自身失敗不重試**；分類寫入 0018 新增的 `runs.failure_class`。**限制註記**：重試窗僅涵蓋 run 仍在 `provisioning` 的派送階段——狀態機無回退邊，離開 `provisioning` 後的 provider 失敗只分類不重試，要放寬需新 ADR）
- [x] RUN-007 實作冪等清理與遺留 Sandbox 掃描。（終態轉移於同交易排入 cleanup job（River unique，重複排入為 no-op）；`DELETE` 依契約冪等且無 404，重跑安全；孤兒掃描以 `GET /runs?active=true` 比對平台狀態，僅在「平台已終態」或「平台不認得且 `observed_at - created_at` 超過 5 分鐘寬限」時 destroy，避免誤殺派送中的新 Run；清理失敗記 `cleanup_status='failed'` 並由 supervisor 重排）
- [x] RUN-008 實作服務重新啟動後的 Run 狀態恢復或安全終止。（supervisor 為 River periodic job（30 秒，`RunOnStart`，僅 leader 執行）：掃非終態 Run，逾期者判 `timed_out`＋清理，其餘以 unique job 重新入列——已有在途 job 時自動 no-op，故不需讀 river 表；有在途 attempt 者重新掛回輪詢，已離開派送階段卻無 attempt 可接者判 `platform_error` 安全終止）
- [x] RUN-009 建立 Provider 契約測試套件。（`internal/run/provider_contract_test.go`：冪等重送同資源／同鍵不同內容 409／cancel 已終態仍 202／DELETE 重複與未知 handle 皆 204／`active=false` 回 400／終態必帶 result／無 token 401；預設跑 in-repo fake（`internal/run/providertest`），設 `SKILLHUB_PROVIDER_CONTRACT_URL`＋`SKILLHUB_PROVIDER_CONTRACT_TOKEN` 即對真實服務跑同一套。狀態映射另以 `schedule_test.go` 的決策表驗證）

## 11. SelfHostedProvider（M2）

實作位於 `services/sandbox/`（獨立 Go module，ADR-019、鐵律 2），部署與 dev／prod 差異見 [services/sandbox/README.md](../../services/sandbox/README.md)。允收準則來源為 `02:RUN-003` 與威脅模型 SEC-002 的 45 項基線（本節以 `C-xx`／`N-xx`／`D-xx`／`I-xx`／`X-xx` 引用）。

- [x] SBX-001 決定自建 Sandbox 的隔離技術與執行節點拓撲。（[ADR-015](../../adr/ADR-015-sandbox-isolation-technology.md) 已 Accepted：gVisor 基線＋專用 VM 池、不進 Kubernetes；DockerProvider 以 `SKILLHUB_SANDBOX_RUNTIME=runsc` 落實該選擇，宣告的 `isolation.level` 跟著實際設定走，跑 runc 的機器只會宣告 `container`。**在部署平台上實跑 `runsc` 屬部署期驗收**，ADR-015 定案紀錄已將其列為實作期前提，不影響本項的「決定」性質）
- [ ] SBX-002 建立經審核的 Runtime Image。**部分完成**：審核流水線已接上——[`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)（path filter 只在 `infra/images/**` 變更時觸發）做 digest 斷言 → build → syft 產 SPDX SBOM → grype 掃描 → 門檻閘門，SBOM 與報告以 `if: always()` 上傳為 artifact，掃描失敗也留證據。I-02 已落地（`FROM` 帶 `@sha256:`，CI 以 grep 斷言）。掃描實跑結果與豁免清單見 [infra/images/README.md](../../infra/images/README.md)：可修的 Critical／High 原有 6 件、全在 base image 自帶的 npm 依賴，已於 Dockerfile 移除 npm／corepack（執行期不載入，沙箱亦無網路），現為 **0**；其餘 7 Critical／17 High 皆為 Debian bookworm 無上游修復項，逐項具名列入豁免清單並訂複審日，不靜默放行。**未勾原因**：I-06 的漏洞等級門檻與 I-04 的掃描結果有效期仍是 SEC-002 六項無值語句其中兩項，README 已提出建議值（可修的 Critical／High 阻擋、不可修者具名記錄；有效期 30 天）但**待負責人定案**——依 SEC-002「未定值前該項不可自動化判定，不得記為通過」，程式以提案值實作並標暫定，本項維持未勾。另 I-03 要求 SBOM 隨 Image 保存以供閘門 A 查詢，現落在 90 天 CI artifact，須待 container registry（ADR-019 待決策 1）定案後搬家。
- [x] SBX-003 實作每個 Run 的獨立環境與暫存空間。（每個 attempt 一個專屬容器，`/work`＋`/out` 為該容器獨有的 tmpfs，不與任何其他 Run 共用可寫路徑；C-01）
- [x] SBX-004 實作非 root、非特權及唯讀基礎檔案系統政策。（`User=65532:65532`、`no-new-privileges:true`、`CapDrop=ALL`、`Privileged=false`、`ReadonlyRootfs=true`；C-02／C-03／C-06／C-08。以真實容器驗證，非僅設定斷言）
- [ ] SBX-005 阻擋容器管理 Socket、主機敏感路徑與內部服務存取。**部分完成**：容器管理 Socket 與主機路徑已擋——`Binds`／`Mounts` 恆空，沙箱內 `/var/run/docker.sock` 不存在，namespace 全為 private（C-04／C-05／C-07，具名整合測試）。**「內部服務存取」未成立**：dev 已由網路面隔離——無出口需求時 `--network none`，有出口需求時只接上 `internal: true` 且只有模型閘道在上面的網路，核心資料庫與物件儲存都不在該網路上（2026-08-16 第四批，SBX-007）。但生產的網路政策隔離與 P-02「Sandbox → 核心資料庫連線嘗試被實際阻擋」的**常駐探針**屬部署期，未實作。
- [x] SBX-006 實作 CPU、記憶體、磁碟、程序數與時間限制。（`NanoCPUs`／`Memory`＋`MemorySwap`（不給 swap）／tmpfs size 依 PDM-005 5.2 切 `/work` 3:1 `/out`／`PidsLimit`／`nofile` ulimit／soft 與 hard wall clock；C-10～C-15。逾時強停與 pids 上限以真實容器驗證）
- [ ] SBX-007 實作預設封鎖的網路出口政策及允許清單。**部分完成（2026-08-16 第四批）**：出口路徑已通——沙箱**僅在 `RunRequest.egress.allow` 含 `model_gateway` 時**接上 `internal: true` 的 Docker network，該網路上只有 LiteLLM 閘道可達（允許清單只有一項的最小強制形式）；`allow` 為空維持 `--network none`。方向只有一個：無出口路由的節點只宣告 `egress_modes: ["none"]`，且 `accept()` 會拒絕帶允許清單的請求，不以較弱模式頂替較強請求。物件儲存**刻意不在**該網路上（dev 的匿名／預簽語意會被破壞），位元組由 sandboxd 代搬。**仍未完成**：生產級 Egress Proxy 本體、域名允許清單、DNS 固定解析與目的地記錄（N-01～N-07）皆屬部署期，允許清單管理流程仍是 ADR-015 待決策——故不勾。
- [x] SBX-008 實作 Dataset、Skill、Secrets 與 Artifact 的短效傳遞。（**2026-08-16 第四批結案**。簽發：worker 於 dispatch 以 S3 預簽鑄 `skill_package`／`dataset`（read）與 `artifact_upload`（write）三類 `ObjectGrant`，TTL＝該 Run 硬牆鐘＋5 分鐘；Virtual Key 經 LiteLLM `/key/generate` 鑄出，帶 `max_budget`＋`tpm_limit` 雙限並限定模型層（PDM-003 v5），以 `ANTHROPIC_BASE_URL`／`ANTHROPIC_AUTH_TOKEN` 注入。**簽發失敗即不派送**（fail-closed）。撤銷：cleanup 以 alias（`skillhub-attempt-<run_attempt_id>`）呼叫 `/key/delete`，冪等、不需保存金鑰本身、重啟後仍撤得掉；撤銷失敗記 `cleanup_status='failed'` 並重試（D-03／D-06、SEC-005）。傳遞：**沙箱不再持有任何預簽 URL**——sandboxd 用 grant 取得位元組後以 `docker exec` 寫進容器（`docker cp` 對唯讀 rootfs 被 daemon 拒絕，實測），Artifact 由 sandboxd 讀出 `/out/artifacts`、套用 PDM-005 5.2 的單檔／總量上限後以寫入授權上傳，manifest 回填 `RunResult.artifacts`。收集在 workload 退出**前**完成（`/out` 是 tmpfs，行程一結束即消失），以 `.workload-done`／`.collected` 交接。全鏈已由真實端到端 Run 驗證。**已知簡化**：一個 attempt 一個 tar 封存（預簽是逐物件的，平台無法預知檔名）；逐檔物件需 POST policy 前綴授權，留給 PACK-001。）
- [x] SBX-009 實作完成、失敗、取消與逾時後清理。（四條終態路徑都會走到 `DELETE`；`DELETE` 冪等、無 404、不存在也回 204，釋放失敗回 500 供平台記錄 `cleanup_status` 並重試；X-01。以真實容器驗證重複 destroy 與容器確實消失）
- [ ] SBX-010 進行隔離、資源耗盡、網路與清理失敗測試。**屬部署期驗收，不在本批**：逃逸測試與 gVisor 相容性需要 Linux 與巢狀虛擬化（ADR-019 待決策 3）。本批已有的是 ADR-005 基線的真實容器驗證（非 root、唯讀 rootfs、無主機掛載、pids 上限、逾時強停、清理冪等），不等於逃逸測試通過。**SEC-009／SBX-010 未通過不得開放外部使用者提交 Skill 執行**（ADR-015 定案紀錄）。

## 12. Local Runner Beta（後 MVP，依 M2 後需求訊號啟動）

- [ ] LOCAL-001 定義 Local Runner 與 Skill Hub 的信任及配對流程。
- [ ] LOCAL-002 實作 Runner 能力、作業系統與版本回報。
- [ ] LOCAL-003 實作本機工具路徑、參數、工作目錄與存取範圍預覽。
- [ ] LOCAL-004 實作每次執行的明確使用者確認。
- [ ] LOCAL-005 實作執行、事件串流、取消、逾時與結果回傳。
- [ ] LOCAL-006 確保未授權的本機檔案與工具不被上傳。
- [ ] LOCAL-007 遮罩 Runner Log、Trace 與錯誤中的 Secrets。
- [ ] LOCAL-008 進行路徑穿越、命令參數、符號連結及權限邊界測試。

## 13. Trace 與平台 O11y（M2）

> **2026-08-16 第三批（Trace 收集管線與 O11y）交付摘要**：事件流向為「容器內 harness 寫 JSONL 到 `/out` → sandboxd 讀出並 POST 到平台 ingestion → 平台遮罩後入庫」。沙箱無網路（`--network none`），執行平面不碰核心 DB（鐵律 2），ingestion 憑證是嵌在 `TracePolicy.ingestion_url` 內的每 attempt 短效簽章 token，未新增契約欄位。migration `0019_trace_ingestion.sql` 補齊 TRACE-001 §8 的四個缺口並加上 `late` 與 default partition。詳見 [m2/README.md 第三批交付摘要](m2/README.md)。

- [x] TRACE-001 定義標準 Trace Event Schema。（`contracts/openapi` 外的第一份事件契約：[contracts/events/trace-event.schema.json](../../contracts/events/trace-event.schema.json) ＋ 同目錄 README；9 種事件、`seq` 範圍為 `(run_id, attempt, emitted_by)`、`run_lifecycle` 非事實來源、遮罩規約與版本演進規則。**2026-08-16 由第三批勾選**：schema 本身在第一批完成，但當時記錄的四個 `trace_events` 欄位缺口（`event_id`／`attempt`／`schema_version`／遮罩狀態）使契約無法落地，`0019` 全數關閉後才符合「Trace 至少包含時間、事件類型、來源、狀態與關聯 Run」與「順序可被重建、能識別缺失」的允收）
- [x] TRACE-002 收集 Skill 啟用與資源讀取事件。（`infra/images/runtime-agent-sdk/run.mjs` 由 Agent SDK 訊息流產生 `skill_activation`（`Skill` 工具呼叫）與 `resource_read`（讀取路徑落在 `SKILLHUB_SKILL_DIR` 下者，路徑相對化不外洩沙箱絕對路徑）；收集端見 `services/sandbox/internal/sandbox/trace.go`。**限制註記**：`decision: skipped`「可用但未啟用」在 SDK 訊息流中不可觀測（沒被叫到就沒有事件），本批不臆造，留給 EVAL-002）
- [x] TRACE-003 收集 Tool Call、MCP Call 與 Script Log。（`tool_call` 為單事件自帶 `duration_ms`，以 `tool_use`／`tool_result` 配對計時；`Bash` 的結果另產 `script_log`。**MCP 不實作**——遠端 MCP 已移出 MVP 首發，契約只保留型別佔位，與 TEST-005/006 同步）
- [x] TRACE-004 收集 Agent 輸出、錯誤、延遲、Token 與成本。（`agent_output`（final／intermediate）、`error`（含控制平面自身的失敗）、延遲（`tool_call.duration_ms`、`usage.duration_ms`）、Token（SDK 回報的 `input_tokens`／`output_tokens`）；orchestrator 側在 `failed`／`timed_out` 轉移的同一交易內寫 `error` 事件，因此未進到沙箱就失敗的 Run 也有可看的時間軸（RUN-004）。**2026-08-16 第四批補上成本**：`model_gateway` 授權簽發後，harness 以**自己的 Virtual Key 讀閘道 `/key/info` 的 per-key spend**填 `cost_usd`＋`cost_source: gateway`——刻意不採 SDK 的 `total_cost_usd`，那是本地價目表對閘道解析出的模型名所做的估算，掛 `gateway` 會是假的。閘道 spend 為非同步 flush，故先等 2.5 秒再讀並讀到數值穩定；實測與閘道 `LiteLLM_SpendLogs` 逐筆合計一致（`0.00571125`，見 [m2/README.md 第四批](m2/README.md)）。讀不到時仍誠實留 `null` 並顯示「未回報」。**限制註記**：這是讀數不是帳本，最後一次 flush 若落在最後一次輪詢之後會少算，權威對帳來源是閘道的 per-key spend（ADR-017）。）
- [x] TRACE-005 實作 Secrets 與敏感欄位遮罩。（遮罩在平台 ingestion **入庫前**執行，明文不落 DB；`0019` 的 `CHECK (masked)` 讓「跳過遮罩」在資料庫層不可能。已知值（本 Run 的 ingestion token）＋ pattern（`sk-*`、`Authorization` 標頭、`ANTHROPIC_AUTH_TOKEN=`／`OPENAI_API_KEY=`、預簽 URL 的簽章參數、私鑰區塊）一律替換為字面量 `[REDACTED]`，不保留長度、不做部分遮罩；`masked_fields` 以 JSON Pointer 記錄實際被替換的位置。有具名單元測試與整合測試，後者直接查 `trace_events.payload` 確認明文不存在）
- [x] TRACE-006 實作一般模式進度摘要。（`GET /runs/{id}/trace`：Skill 啟用、資源讀取次數、工具呼叫統計與最慢者、錯誤、最終輸出、Token 與成本。**Run 狀態取自 `runs` 表、進度步驟取自 `run_status_transitions`**，不重播 `run_lifecycle` 事件重建狀態（鐵律 5）；`cost_usd` 為 `null` 時顯示「未回報」不顯示 0）
- [x] TRACE-007 實作進階模式 Trace 檢視。（`?mode=advanced`：經遮罩的原始事件依 `(occurred_at, emitted_by, seq)` 重建順序，並逐串流列出 `missing_seq` 與遲到計數；`complete: false` 時 UI 明示「部分事件未送達」。payload 一律以 inert text 呈現，不解讀 HTML／ANSI／SVG，有具名測試以注入 `<img onerror>` 驗證。UI 為 `apps/web` 的 `/runs/$runId`，一般／進階切換）
- [x] TRACE-008 處理事件排序、重送、缺失與延遲。（去重鍵為 producer 產生的 `event_id`，`ON CONFLICT DO NOTHING`，重送回報為 `duplicate` 而非錯誤；順序以 per-producer 的 `seq` 重建，斷號＝該事件遺失且被逐一列出；終態後仍接受遲到事件並標記 `late`，因為沙箱關機時推送的最後一批正是失敗 Run 最需要的部分。sandboxd 側推送失敗不推進水位，下一輪重送）
- [x] O11Y-001 量測搜尋、Run 排隊、建立、成功、逾時與清理指標。（Prometheus 文字格式，`services/platform` 與 `sandboxd` 各自曝露 `/metrics`；平台側走獨立 listener `METRICS_ADDR`，不掛在對外 API port 上。指標清單見 [infra/observability/README.md](../../infra/observability/README.md)）
- [x] O11Y-002 建立 Provider 健康度與錯誤監控。（`skillhub_provider_capability_total{provider,result}` 區分 ok／unhealthy／error；`skillhub_provider_request_total{provider,operation,class}` 以狀態碼分級記 429 與 5xx；另有每操作延遲 histogram。告警規則四條見 `alerts.yml`）
- [x] O11Y-003 建立遺留 Sandbox、資源異常及安全事件告警。（`skillhub_run_cleanup_backlog` gauge、`skillhub_orphan_scan_total`、`skillhub_orphan_sandbox_total{action}` 區分「殺掉的漏網 Sandbox」與「殺不掉的」，加上遮罩器靜默失效偵測（`TraceMaskingStopped`——NFR-002 沒有其他偵測器）。告警為 `infra/observability/alerts.yml` 的 rules 檔＋文件，**Alertmanager 部署、通知路由與 Grafana dashboard 屬部署期，明確未做**；門檻值為首發預設非實測校準值，已在文件標明需上線後回填）

## 14. 評估與改善（M3）

- [ ] EVAL-001 將驗收條件轉換為可執行或可判斷的檢查。
- [ ] EVAL-002 實作規格、啟用、執行、效果、相容與成本分類。
- [ ] EVAL-003 對每項驗收條件產生通過、未通過或無法判斷及證據。
- [ ] EVAL-004 產生符合、部分符合、未符合或無法判斷的整體結果。
- [ ] EVAL-005 清楚標示規則判斷、模型判斷與使用者判斷。
- [ ] EVAL-006 實作有幫助／無幫助及文字回饋。
- [ ] EVAL-007 產生包含問題、證據、位置、修改與影響的改善建議。
- [ ] EVAL-008 區分 Skill、Runtime、MCP、工具及測試資料問題。
- [ ] EVAL-009 實作逐項接受／拒絕與修改差異預覽。
- [ ] EVAL-010 套用改善時建立新 Skill Version。
- [ ] EVAL-011 實作使用相同 Test Case 重新試跑。
- [ ] EVAL-012 實作版本、驗收、輸出、錯誤、延遲與成本比較。

## 15. 打包與下載（M4）

- [ ] PACK-001 建立標準 Agent Skill 打包流程。
- [ ] PACK-002 打包前重新執行規格驗證。
- [ ] PACK-003 保留 License、作者、原始來源與衍生關係。
- [ ] PACK-004 排除 Secrets、測試憑證、內部路徑與受保護資料。
- [ ] PACK-005 支援選擇是否包含可散布 Test Case 與範例資料。
- [ ] PACK-006 建立首批目標 Agent 安裝 Profile。
- [ ] PACK-007 產生安裝位置、依賴、環境變數與驗證步驟。
- [ ] PACK-008 對未驗證 Agent 顯示清楚限制。
- [ ] PACK-009 驗證下載套件可被解壓、重新驗證與依說明安裝。

## 16. 安全、隱私與合規（M0 起跨階段）

> **2026-08-15 補齊允收準則**：本節十項原本只有一行敘述、`02` 無 SEC-* 需求 ID（威脅模型開放問題 **Q19**）。允收準則已補於 **`02` 第 6 節「安全需求」**，取自 [m0/threat-model-and-sandbox-baseline.md](m0/threat-model-and-sandbox-baseline.md) v2 已落地的 32 條威脅與 45 項基線檢查，未新增安全要求；該文件未定的門檻值在 `02` 標為「未涵蓋（待決策）」。

- [ ] SEC-001 完成 Skill、Script、MCP、Dataset、Secrets 與 Local Runner 威脅模型。（允收：`02` §6）
- [ ] SEC-002 定義 Sandbox 最低安全基線與阻擋條件。（允收：`02` §6）
- [ ] SEC-003 建立 Skill 匯入與執行前靜態掃描政策。（允收：`02` §6）
- [ ] SEC-004 建立遠端 MCP 的 SSRF、內部網路與資料外洩防護。（後 MVP，隨 MCP 啟動）（允收：`02` §6）
- [ ] SEC-005 建立 Secrets 儲存、短效注入、遮罩與撤銷流程。（允收：`02` §6）
- [ ] SEC-006 建立 Dataset、Trace 與 Artifact 保存及刪除政策。（允收：`02` §6）
- [ ] SEC-007 建立來源、License 與下架處理政策。（允收：`02` §6）
- [ ] SEC-008 驗證使用者與 Run 之間的資料隔離。（允收：`02` §6）
- [ ] SEC-009 進行 Sandbox 逃逸、資源濫用與權限提升測試。（允收：`02` §6）
- [ ] SEC-010 完成安全事件回應與緊急停用 Provider 流程。（允收：`02` §6）

## 17. 品質保證與封閉測試（M4）

- [ ] QA-001 建立核心旅程端到端測試。
- [ ] QA-002 建立 Agent Skills 規格驗證測試資料集。
- [ ] QA-003 建立正常、失敗、逾時、取消與清理 Run 測試。
- [ ] QA-004 建立 Dataset 與 Secrets 權限測試（MCP、Local Runner 部分隨後 MVP 啟動）。
- [ ] QA-005 建立 Trace 完整性與敏感資訊遮罩測試。
- [ ] QA-006 建立評估可解釋性與改善版本不可變測試。
- [ ] QA-007 建立打包、License、來源及 Secrets 排除測試。
- [ ] QA-008 完成主要瀏覽器與目標作業系統測試。
- [ ] QA-009 完成鍵盤操作、文字標籤與色彩以外狀態提示檢查。
- [ ] BETA-001 招募目標個人創作者進行封閉測試。
- [ ] BETA-002 量測搜尋到詳情、詳情到試跑、試跑到下載的漏斗。
- [ ] BETA-003 蒐集評估報告與改善建議的質性回饋。
- [ ] BETA-004 記錄所有阻斷首次成功旅程的問題。
- [ ] BETA-005 依封閉測試結果完成 MVP 範圍及優先級複審。

## 18. MVP 發佈檢查（M4）

- [ ] RELEASE-001 所有 MVP 必要需求都有對應測試與結果。
- [ ] RELEASE-002 完整核心旅程通過端到端驗證。
- [ ] RELEASE-003 精選 Skill 內容、來源、License 與驗證狀態完成檢查。
- [ ] RELEASE-004 Sandbox 與 Secrets 安全檢查完成（MCP、Local Runner 隨後 MVP 啟動時補檢）。
- [ ] RELEASE-005 資料保存、使用者刪除與稽核流程通過驗證。
- [ ] RELEASE-006 Provider 故障、Run 逾時與清理失敗有可操作處理方式。
- [ ] RELEASE-007 使用者可理解執行權限、評估依據與相容性限制。
- [ ] RELEASE-008 下載套件可安裝且不包含受保護資料。
- [ ] RELEASE-009 完成封閉測試成功門檻並處理阻斷問題。
- [ ] RELEASE-010 發佈決策、已知限制與下一階段範圍完成記錄。


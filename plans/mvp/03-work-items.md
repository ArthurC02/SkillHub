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

- [x] CONTENT-001 定義精選、已索引與外部結果的收錄政策。
- [x] CONTENT-002 定義來源可信度、License 與衍生關係的呈現規則。
- [ ] CONTENT-003 建立首批 Skill 候選清單。（正式清單見 [plans/mvp/m1/curated-skill-list.md](m1/curated-skill-list.md)、匯入資料 `tools/content/seed-skills.json`。**2026-08-15 起三類的已索引與精選數量目標全部達標**——`documents` 精選由 3 補足為 4（`excel-format`，見該文件 §9）。仍不勾的原因有二：§5.2 的 source-available 法務判定未完成前，`documents` 已索引有 10→6 的塌陷風險；⑤⑦⑧ 三項檢查需 CONTENT-005／006／007／008 才能判定）
- [ ] CONTENT-004 對首批 Skill 完成來源及 License 檢查。（License 合規總表見 [curated-skill-list.md §5](m1/curated-skill-list.md)；11 個入選 repo 已逐一實查 LICENSE 檔。未結案項：§5.2 `anthropics/skills` source-available 條款是否允許平台保存內容快照，待負責人與法務判定）
- [ ] CONTENT-005 對首批 Skill 產生一般使用者可理解的摘要。
- [ ] CONTENT-006 對首批 Skill 完成規格及靜態掃描。
- [ ] CONTENT-007 對精選 Skill 建立範例資料、Prompt 與驗收條件。
- [ ] CONTENT-008 對精選 Skill 完成至少一次基準試跑。
- [ ] CONTENT-009 建立內容更新、失效、下架與來源變更流程。

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

## 7. Skill Explorer（M1，結束時通過驗證閘門才進 M2）

- [ ] DISC-001 實作自然語言意圖搜尋。
- [ ] DISC-002 產生候選 Skill 與符合原因。
- [ ] DISC-003 實作類別、來源、Agent、Script、MCP 與驗證狀態篩選。
- [ ] DISC-004 實作可解釋的排序規則。
- [ ] DISC-005 實作無結果、低信心及查詢補充流程。
- [ ] DISC-006 實作 Skill 一般詳情頁。
- [ ] DISC-007 實作 `SKILL.md` 與檔案樹進階檢視。
- [ ] DISC-008 實作來源、License、風險及相容狀態展示。
- [ ] DISC-009 實作至少兩個 Skill 的靜態比較。
- [ ] DISC-010 驗證公開搜尋與詳情不要求登入，私有操作要求登入。

## 8. Fork、版本與工作區（M1）

- [x] WS-001 實作 Fork 並保留來源與 License 關係。
- [x] WS-002 實作不可變版本保存。
- [x] WS-003 實作任兩版本差異比較。
- [ ] WS-004 實作個人 Skill、Test Case、Run 與下載紀錄列表。
- [x] WS-005 實作私有內容刪除與狀態回饋。
- [x] WS-006 驗證不同使用者無法存取彼此私有內容。（列表／刪除／Fork／Diff 四條路徑與公開搜尋皆有具名整合測試，CI 帶 Postgres service 實際執行。證據見 [m1-work-items-audit.md §6](m1/m1-work-items-audit.md)）

## 9. Test Case 與執行設定（M2）

- [ ] TEST-001 實作 User Prompt 輸入與驗證。
- [ ] TEST-002 實作驗收條件自動建議。
- [ ] TEST-003 實作驗收條件新增、修改、刪除與確認。
- [ ] TEST-004 實作 Dataset 上傳、限制、關聯與刪除。
- [ ] TEST-005 實作遠端 MCP 位址及短效憑證設定。（後 MVP）
- [ ] TEST-006 實作 MCP 工具發現與權限選擇。（後 MVP）
- [ ] TEST-007 實作 Local Runner 連線狀態與本機絕對路徑選擇。（後 MVP）
- [ ] TEST-008 實作執行前 Dataset、Script、MCP、工具、網路與 Secrets 摘要。
- [ ] TEST-009 實作權限異動後重新確認。
- [ ] TEST-010 保存實際執行使用的 Test Case 快照。

## 10. Run Orchestrator 與 Provider 契約（M2）

- [ ] RUN-001 定義 Provider-neutral Run Request 與 Run Result。
- [ ] RUN-002 定義 Provider Capability 描述格式。
- [ ] RUN-003 定義平台 `run_id` 與 `provider_run_id` 映射。
- [ ] RUN-004 實作 queued 到 cleaning_up 的標準狀態機。
- [ ] RUN-005 實作 Run 排程、Provider 選擇與能力相容檢查。
- [ ] RUN-006 實作取消、逾時、有限重試與失敗分類。
- [ ] RUN-007 實作冪等清理與遺留 Sandbox 掃描。
- [ ] RUN-008 實作服務重新啟動後的 Run 狀態恢復或安全終止。
- [ ] RUN-009 建立 Provider 契約測試套件。

## 11. SelfHostedProvider（M2）

- [ ] SBX-001 決定自建 Sandbox 的隔離技術與執行節點拓撲。
- [ ] SBX-002 建立經審核的 Runtime Image。
- [ ] SBX-003 實作每個 Run 的獨立環境與暫存空間。
- [ ] SBX-004 實作非 root、非特權及唯讀基礎檔案系統政策。
- [ ] SBX-005 阻擋容器管理 Socket、主機敏感路徑與內部服務存取。
- [ ] SBX-006 實作 CPU、記憶體、磁碟、程序數與時間限制。
- [ ] SBX-007 實作預設封鎖的網路出口政策及允許清單。
- [ ] SBX-008 實作 Dataset、Skill、Secrets 與 Artifact 的短效傳遞。
- [ ] SBX-009 實作完成、失敗、取消與逾時後清理。
- [ ] SBX-010 進行隔離、資源耗盡、網路與清理失敗測試。

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

- [ ] TRACE-001 定義標準 Trace Event Schema。
- [ ] TRACE-002 收集 Skill 啟用與資源讀取事件。
- [ ] TRACE-003 收集 Tool Call、MCP Call 與 Script Log。
- [ ] TRACE-004 收集 Agent 輸出、錯誤、延遲、Token 與成本。
- [ ] TRACE-005 實作 Secrets 與敏感欄位遮罩。
- [ ] TRACE-006 實作一般模式進度摘要。
- [ ] TRACE-007 實作進階模式 Trace 檢視。
- [ ] TRACE-008 處理事件排序、重送、缺失與延遲。
- [ ] O11Y-001 量測搜尋、Run 排隊、建立、成功、逾時與清理指標。
- [ ] O11Y-002 建立 Provider 健康度與錯誤監控。
- [ ] O11Y-003 建立遺留 Sandbox、資源異常及安全事件告警。

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

- [ ] SEC-001 完成 Skill、Script、MCP、Dataset、Secrets 與 Local Runner 威脅模型。
- [ ] SEC-002 定義 Sandbox 最低安全基線與阻擋條件。
- [ ] SEC-003 建立 Skill 匯入與執行前靜態掃描政策。
- [ ] SEC-004 建立遠端 MCP 的 SSRF、內部網路與資料外洩防護。（後 MVP，隨 MCP 啟動）
- [ ] SEC-005 建立 Secrets 儲存、短效注入、遮罩與撤銷流程。
- [ ] SEC-006 建立 Dataset、Trace 與 Artifact 保存及刪除政策。
- [ ] SEC-007 建立來源、License 與下架處理政策。
- [ ] SEC-008 驗證使用者與 Run 之間的資料隔離。
- [ ] SEC-009 進行 Sandbox 逃逸、資源濫用與權限提升測試。
- [ ] SEC-010 完成安全事件回應與緊急停用 Provider 流程。

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


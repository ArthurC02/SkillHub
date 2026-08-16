# M2 工作項對帳（`03-work-items.md` 第 9～11、13 節，＋ §4 CONTENT-007／008、§16 SEC-002／009）

- 日期：**2026-08-16**
- 基準 commit：**`512add1`**（`Run all 45 catalogue skills through the real platform and report what happened`）；對帳範圍為 `3bfd8db..512add1` 的 18 個 M2 commit
- 範圍：`03-work-items.md` 的 **§9 TEST／§10 RUN／§11 SBX／§13 TRACE＋O11Y**，加上自 M1 移入 M2 的 **§4 CONTENT-007／008**，以及 M2 明列為部署驗證批的 **§16 SEC-002／SEC-009**，逐項對照 [`02-specifications-and-acceptance-criteria.md`](../02-specifications-and-acceptance-criteria.md) 的允收準則。
- 方法：沿用 [M1 對帳](../m1/m1-work-items-audit.md) 的證據標準——**只採信 repo 內可指名的落地證據**（migration 檔、Go 檔與匯出符號、具名測試函式、CI 設定、已交付報告），沒有證據的項目一律記「未動」，不以「應該有」補位。本次另加一層：**已交付報告宣稱的數字，回到執行中的資料庫重查**（見 §2.3）。
- 判定值：**完全符合**（勾 `[x]`）／**部分完成**（不勾，列缺口）／**未動**（不勾）。
- 依 AGENTS.md 文件維護規則：**部分完成一律保持 `[ ]`**。

> **對帳當下的工作樹狀態**：乾淨，`HEAD == origin/main == 512add1`。
>
> **2026-08-16 完結後回填（追加不改寫）**：對帳期間 in-flight 的兩件事都已落地，本文件據此更新 §9.7 與新增 §9.8，**§1～§8 的判定與勾選一字未改**。
>
> | commit | 內容 | 對本對帳的影響 |
> | --- | --- | --- |
> | `afb5767` | CONTENT-005 的 11 筆 Python 揭露缺口（prompt 升 `enrich-skill/v5`） | **11/11 關閉**；但 `docx` 兩輪重審未過，**CONTENT-005 現為 44/45**。CONTENT-007／008 的判定不受影響（見 §7 註） |
> | `ddc3e54` | `services/sandbox/README.md` 的 SDK 載入條件 | **§9.7 已關閉**，見該節 |

---

## 1. 對帳結論總表

| 節 | 項目數 | 維持勾選 | **本次退回** | 維持不勾（誠實記錄） | 本次新勾 |
| --- | --- | --- | --- | --- | --- |
| §9 TEST（M2 內 7 項，`005/006/007` 屬後 MVP 不計） | 7 | 6 | **1**（`TEST-004`） | 0 | 0 |
| §10 RUN | 9 | 9 | 0 | 0 | 0 |
| §11 SBX | 10 | 6 | 0 | 4（`002`／`005`／`007`／`010`） | 0 |
| §13 TRACE＋O11Y | 11 | 11 | 0 | 0 | 0 |
| §4 CONTENT-007／008 | 2 | 1（`008`） | 0 | 1（`007`） | 0 |
| §16 SEC-002／009 | 2 | 0 | 0 | 2 | 0 |
| **合計** | **41** | **33** | **1** | **7** | **0** |

**一句話結論：M2 的帳面與實作高度一致——41 項裡只有 1 項需要退回。** 這與 M1 第一次對帳（46 項退回／未勾 12 項）是完全不同的狀況，原因是 M2 各批交付摘要本身就把限制寫進了 `03` 的勾選註記裡（RUN-006 的重試窗、SBX-002 的暫定門檻、SBX-007 的 Proxy 本體、TRACE-004 的讀數不是帳本），**帳面沒有虛報**。

**但這不表示 M2 沒有洞。** 本次對帳找到 **7 個沒有任何工作項承接的缺口**（§9），其中最重的一個是：**PDM-005 的 300K／60K token 硬上限被寫進 `policy_snapshot`、被顯示在執行前權限摘要上、被使用者按下「確認」同意，而平台沒有任何一行程式會因為超過它而停止一個 Run。** 它不falsify任何一個勾選（因為 `02:RUN-003` 與 `SBX-006` 的字面清單裡都沒有 token），這正是問題所在——**它是孤兒**。

---

## 2. 對帳前的環境修復與證據可重查性

### 2.1 `skillhub-api` 缺物件儲存金鑰（已修復）

[content-baseline-report.md §2.1](content-baseline-report.md) 交接的副作用：基準試跑批重建 seaweedfs 關閉匿名存取後，既有 `skillhub-api` 容器啟動時沒有 `OBJSTORE_ACCESS_KEY`／`OBJSTORE_SECRET_KEY`，已無法讀寫物件儲存。

環境變數名稱先以程式碼確認（`services/platform/cmd/api/main.go:58-62`、`worker`／`maintenance`／`reindex` 四個 cmd 同名），確定是 `OBJSTORE_SECRET_KEY` 而非 `OBJSTORE_SECRET`。以 `docker inspect` 取出原有六個環境變數後 `docker rm -f`，以**相同配置＋兩把金鑰**（`skillhubdev`／`skillhubdevsecret`，取自已入庫的 [`infra/compose/seaweedfs-s3.json`](../../../infra/compose/seaweedfs-s3.json)）重建。

| 驗證項 | 結果 |
| --- | --- |
| `GET /healthz` | **200** |
| `GET /api/skills/search?q=excel` | 有結果，首筆 `excel-format` |
| **`GET /api/skills/{id}/files`（碰物件儲存）** | **200**，`SKILL.md` 全文 9,839 字元、檔案樹 3 列（`LICENSE.repo`／`LICENSE.repo.provenance.json`／`SKILL.md`）——**403 已解** |

`postgres`／`litellm`／`seaweedfs` 三個容器全程未動（uptime 分別為 16／8／7 小時，跨越本次操作）。

### 2.2 殘留容器清理

`focused_borg` 與 `friendly_wright`（兩個 `golang:1.25` 舊測試容器，分別在 `until nc -z localhost 55433` 與 `until nc -z 127.0.0.1 5432` 空等不存在的 DB port）已刪除。`docker ps -a` 現為 stack 四容器＋兩個無關容器（`springaitest-appdb-1`、`manual-rabbitmq`，非本專案，未碰）。`docker ps -a --filter label=skillhub.sandbox.managed=1` 為 **0**。

### 2.3 已交付報告的數字回查（本次新增的證據層）

[content-baseline-report.md](content-baseline-report.md) 自陳「本報告的每個數字都可由 Run ID 在資料庫重查」。對帳直接查了：

| 報告宣稱 | 資料庫實查 | 一致 |
| --- | --- | --- |
| 73 個 Run 全部 `cleanup_status = cleaned` | `runs` 73 列，`cleaned` **73** | ✅ |
| Trace 事件 1112，全數 `masked` | `trace_events` **1112**，`masked = true` **1112 / 1112** | ✅ |
| Test Case 快照 73 | `test_case_snapshots` **73** | ✅ |
| 權限確認紀錄存在（TEST-009） | `run_permission_confirmations` **74** | ✅ |
| migration 0016～0020 已補套用 | `run_attempts`／`outbox_events`／`run_permission_confirmations`／`trace_events_2026_08` 分割表 全部存在 | ✅ |

另附：`run_attempts` 103 列、`outbox_events` 446 列、Run 終態 38 `succeeded` ／ 35 `failed`。**報告的可追溯性宣稱成立**，CONTENT-008 的勾選有可重查的事實基礎，不只有一份 Markdown。

---

## 3. §9 Test Case 與執行設定（M2）

| 項目 | 判定 | 允收對照與證據 |
| --- | --- | --- |
| **TEST-001** User Prompt 輸入與驗證 | **✅ 維持勾選** | `02:TEST-001` 第 1、2、4 條。非空白雙重把關：Go `services/platform/internal/testlab/testlab.go:174` `validateDraft`（由 `CreateTestCase:122`／`UpdateTestCase:160` 呼叫），DB `db/migrations/0004_test_lab_and_runs.sql:11` `CHECK (btrim(user_prompt) <> '')`，`0017` 另加 `test_cases_name_not_blank`。快照保存見 TEST-010。測試 `TestTestCaseCRUDAndPromptValidation`、`TestTestCaseRejectsForeignSkill`、`TestTestCaseScopeIsWorkspaceBound`、`TestTestLabRoutesRejectAnonymousCallers` |
| **TEST-002** 驗收條件自動建議 | **✅ 維持勾選** | `02:TEST-001` 第 2 條的「可選強化」語意成立。`testlab/suggest.go:63` → `llmclient/client.go` → `POST /suggest-criteria`。**鐵律 11 的關鍵斷言已驗證**：請求型別 `DatasetField{Name, InferredType}` **結構上沒有任何欄位可以承載資料列值**，型別由 `suggest.go:195 inferType` 在 Go 程序內推斷後即丟棄；具名測試 `TestSuggestionRequestCarriesDatasetFieldsButNoRows`（`identity/preflight_integration_test.go:452`）以真實列值反證。LLM 未設定回 503 並保留手動路徑（`http.go:67-71`、`TestSuggestionIsUnavailableWithoutTheLLMService`、`TestSuggestionSurvivesAnLLMServiceFailure`） |
| **TEST-003** 驗收條件新增／修改／刪除／確認 | **✅ 維持勾選** | `02:TEST-001` 第 3 條。`testlab.go:249/267/301`，並發安全由 `mutateCriteria:313` 的 `LockTestCase` 行鎖保證。**改寫文字即撤銷確認**：`testlab.go:279-286` 在 `newText != list[i].Text` 時清 `ConfirmedAt` 並把 `Source` 轉回 `SourceUser`——**寫回相同文字不撤銷**，這是對的。測試 `TestAcceptanceCriteriaLifecycle` |
| **TEST-004** Dataset 上傳、限制、關聯與刪除 | **❌ 本次退回 `[x]` → `[ ]`（部分完成）** | 見 §3.1 |
| TEST-005／006／007 | 不計（後 MVP） | 遠端 MCP 與 Local Runner 已移出 MVP 首發（AGENTS.md 範圍注意、`02:TEST-003`／`TEST-004`） |
| **TEST-008** 執行前權限摘要 | **✅ 維持勾選** | `02:TEST-005` 第 1 條八項全數揭露：`run/preflight.go:67-80` `PermissionSummaryContent`（`Datasets`＋`DatasetTotalBytes`、`Scripts`、`Tools`、`MCPServers`、`Network`、`InjectedSecrets`、`Provider`、`ResourceLimits`）。**MCP 恆為顯式空清單**（`:213 MCPServers: []string{}`，非 nil 故序列化為 `[]` 而非 `null`；UI `RunPreflight.tsx` 渲染為明確的「無」）。Script 讀不到時標 `unavailable` 不得呈現為 `none`（`:91-103`）。摘要 hash 為 struct 固定欄位序的 `json.Marshal` ＋ `sha256`（`:220-227`）。測試 `TestPreflightSummaryDisclosesEveryRequiredItem`、`TestPreflightSummaryReportsScriptsInThePackage`、`TestPreflightShowsThePolicyTheRunIsActuallyHeldTo`、`TestPreflightHashIsStableOverIdenticalInput`、`TestPreflightIsWorkspaceScoped`。**附帶缺口見 §9.3（PDM-005 §5.3 的兩個欄位）** |
| **TEST-009** 權限異動後重新確認 | **✅ 維持勾選** | `02:TEST-005` 第 2、3 條。`0020_run_permission_confirmations.sql`，唯一索引 `(workspace_id, skill_version_id, test_case_id, summary_hash)`——**舊確認因 hash 是查詢鍵而自然失效**，不需另作撤銷掃描。`preflight.go:341 requirePermissionConfirmation` 同時要求「請求帶的 hash＝重算值」與「該 hash 有確認紀錄」，且在 `service.go:256` 被呼叫——**位置在 `s.Pool.Begin(ctx)`（:259）之前，連交易都不會開，故 422 時不可能留下任何 Run 列**。測試 `TestRunStartsOnlyAfterThePermissionSummaryIsConfirmed`、`TestRunRefusesAHashThatIsNotTheCurrentSummary`、`TestChangingTheDatasetInvalidatesAnEarlierConfirmation`、`TestRemovingADatasetInvalidatesAnEarlierConfirmation` |
| **TEST-010** Test Case 快照 | **✅ 維持勾選** | `02:TEST-001` 第 4 條。`testlab/snapshot.go:61 CreateSnapshot` 為**全 repo 唯一實作**，非測試呼叫端只有一個（`run/service.go:269`），且在 `Begin`（:259）與 `CreateRun`（:277）之間、同一個 `q := s.queries().WithTx(tx)` 上——單一 commit。不可變由 `0005` trigger 保證。已軟刪除的 Test Case 回 `ErrNotFound`（`:270-272`）。測試 `TestSnapshotFreezesTheTestCase`、`TestSnapshotIsWorkspaceScoped`、`TestRunCannotStartFromADeletedTestCase`。8232a79 的收斂（兩份 freezer 雜湊出不同 `content_hash`）已確認只剩一份 |

### 3.1 TEST-004 為何退回

**已落地的部分完全站得住**，逐條查證無誤：

| 面向 | 證據 |
| --- | --- |
| 單檔 25 MB／單 Test Case 100 MB／20 檔／90 天 | `testlab/testlab.go:32-41` `MaxFileBytes = 25 << 20`、`MaxTestCaseBytes = 100 << 20`、`MaxFilesPerTestCase = 20`、`DatasetRetention = 90 * 24 * time.Hour`；`expires_at` 於建立時寫入（`dataset.go:102`） |
| magic bytes 判型不信副檔名 | `testlab/filetype.go:62 detectContentType`（`sniffLen = 512`、`deniedMagic`、`allowedSniffed`）＋ `inspectZip:116`；`filetype_test.go` |
| 刪除連同物件 | `dataset.go:126 DeleteDataset` → `:162 removeObject` |
| 只授權給指定 Run／Test Case | Workspace scope＋SBX-008 的逐物件預簽（見 §5） |
| 測試 | `TestDatasetUploadStoresAndAssociates`、`TestDatasetUploadEnforcesPerFileSizeLimit`、`TestDatasetUploadEnforcesFileCountLimit`、`TestDatasetUploadEnforcesTotalSizeLimit`、`TestDatasetUploadJudgesTypeByContentNotExtension`、`TestDatasetDeletionRemovesTheObject` |

**未達成的是 `02:TEST-002` 的第 2 條：「上傳前顯示大小限制、保存政策及資料使用範圍。」**

平台**有**這個資料來源：`GET /test-cases/limits`（`testlab/http.go:349-374`、路由 `apiserver/router.go:74`、契約 `contracts/openapi/public.yaml:1196`），回傳 `max_file_bytes`／`max_test_case_bytes`／`max_files_per_test_case`／`retention_days`／`allowed_kinds` 與一段涵蓋資料使用範圍的 `note`，且有斷言確認「公布的上限＝實際強制的上限」（`testlab_integration_test.go:376-385`）。

**但沒有任何東西顯示它。** `apps/web/src/pages/` 只有 `Compare.tsx`、`Home.tsx`、`RunPreflight.tsx`、`RunTrace.tsx`、`SkillDetail.tsx`、`SkillFiles.tsx`——**沒有任何 Test Lab／Dataset 上傳頁面**，`apps/web/src/api/lab.ts` 也只包了 preflight 三支端點，沒有 `limits` 呼叫。端點存在但無消費端。

**退回的理由與 M1 的判例一致。** M1 第一次對帳把整個 §7 DISC 判為不勾，理由正是「後端做得比帳面深，卡點在 UI 未接線」；DISC-006 也是等到 `SkillDetail.tsx` 補上 `Limitations` 元件之後才勾。同一把尺套在這裡：**一條寫著「顯示」的允收準則，沒有顯示的地方就是沒有達成。** 五條裡缺一條，依 AGENTS.md「部分完成保持 `[ ]`」。

### 3.2 附帶提出：§9 六個已勾項全部是 API-only（**需負責人裁定，本次不自行退回**）

TEST-004 只是第一個撞到「顯示」這個動詞的項目。實際狀況是：**M2 的 Test Lab 完全沒有 UI**——建立 Test Case、寫 Prompt、增刪驗收條件、上傳 Dataset，四件事在 `apps/web` 裡都不存在。`02` §4.3 的多條準則以「使用者可…」起頭（例如 `TEST-001` 的「使用者可新增、編輯、刪除及確認驗收條件」、`TEST-002` 的「使用者可在建立 Test Case 時上傳支援格式的檔案」），若以 M1 對 DISC 的同一把尺解讀，**TEST-001／002／003 也會落在部分完成**。

本次**不**自行退回這三項，理由有二：

1. 這三項的允收準則裡沒有「顯示」這類**只能由 UI 滿足**的動詞；TEST-004 有。這條界線是可判定的，「使用者可」則需要先裁定 `02` §4.3 是在描述系統能力還是端到端可用性。
2. 依 AGENTS.md，改變允收範圍要三份文件同步且屬負責人決策，不是對帳可以單方面做的事。

**建議的處置**（擇一，二選一都要三文件同步）：(a) 承認 §4.3 的準則含 UI，把 TEST-001／002／003 一併退回並新增 Test Lab UI 的實作工作項；(b) 明文把 §9 的允收界定在 API＋契約層，UI 由 `DESIGN-007`（目前未勾）與一個新的實作工作項承接。**目前的狀態是兩者都沒說**——這才是要修的東西。

---

## 4. §10 Run Orchestrator 與 Provider 契約（M2）

**九項全部維持勾選**，逐條複核無誤。

| 項目 | 判定 | 關鍵證據 |
| --- | --- | --- |
| RUN-001 Provider-neutral 契約 | ✅ 維持 | `contracts/openapi/sandbox-provider.yaml`（37f1918 凍結）。`02:RUN-001` 第 1、2、4 條成立；第 3 條（Provider 宣告能力、只派相容任務）由 RUN-005 落實 |
| RUN-002 Capability 描述格式 | ✅ 維持 | 同檔 `ProviderCapability` |
| RUN-003 `run_id` ↔ `provider_run_id` 映射 | ✅ 維持 | `0016_run_orchestration.sql` 的 `run_attempts`；解掉 `0004` 的「重試覆寫 `provider_run_id`」債（鐵律 10）。測試 `TestRetryAddsAttemptWithoutOverwritingTheProviderMapping` |
| RUN-004 標準狀態機 | ✅ 維持 | `02:RUN-002` 四條。狀態轉移表 `run/state.go` `successors`；`run_status_transitions` 記時間與原因；`cleaning_up` 依 ADR-004 走 `run_cleanup_status` 欄位而非 enum。測試 `TestIllegalTransitionIsRefusedWithoutWriting`、`TestRunWalksTheStateMachineAndIsCleanedUp` |
| RUN-005 排程與能力相容檢查 | ✅ 維持 | `run/provider.go:437` 讀 `SKILLHUB_SANDBOX_PROVIDERS`；`:414 capabilityTTL = 30 * time.Second`。`schedule.go:99 Match` 涵蓋健康度、隔離等級、rootless、egress 模式、Runtime 家族與整合模式，**外加剛好六項資源上限**（`:145-160` vCPU／記憶體／磁碟／PID／FD／硬牆鐘）。`:170 Select` 逐 provider 累積 `reasons`，**在排入佇列前**回 422（`checkSchedulable:82`）。測試 `schedule_test.go`、`TestIncompatibleWorkIsRefusedBeforeItIsQueued` |
| **RUN-006 取消、逾時、重試與失敗分類** | ✅ 維持，**限制註記已驗證為真** | `02:RUN-004` 四條。重試上限 `job.go:22 defaultMaxAttempts = 3`。**「重試窗只涵蓋 provisioning」有兩層硬證據**：(1) 重試迴圈整個位於 `(*driver).dispatch`（`job.go:187`），唯一入口是 `job.go:167` 的 `case d.cur.Status == queued \|\| provisioning`，其餘一律走 `default:`（`:170-176`）直接失敗，註解寫明「the state machine has no way back to provisioning」；(2) `state.go` 的 `successors` map **沒有任何一條邊指回 `provisioning`**。**這是狀態機的性質，不是實作偷懶**，`03` 的註記誠實。測試 `TestDispatchFailuresAreRetriedWithNewAttempts`、`TestRetriesAreBoundedAndClassifiedAsProviderFailure`、`TestWorkloadFailureIsRecordedOnceAndNotRetried`、`TestCancelReachesTheProviderAndStopsTheRun`、`TestCancelRecordsIntentAndStopsAQueuedRun` |
| **RUN-007 冪等清理與遺留掃描** | ✅ 維持（**基準試跑批發現的洞已修並實證**） | `cleanup.go:35 orphanGrace = 5 * time.Minute`（`:292` 強制）、`:40 OrphanScanInterval`。**`cmd/worker/main.go:135` 現有 `runs.Queue = client`**，`:132-134` 的註解指名它修的是哪個 bug；位置在 `river.NewClient` 之後、`client.Start` 之前。實證：資料庫 73／73 `cleaned`（§2.3）。測試 `TestOrphanScanDestroysLeakedSandboxesButSparesFreshOnes`、`TestOutboxPublisherIsAtLeastOnceAndIdempotent` |
| RUN-008 重啟恢復或安全終止 | ✅ 維持 | `supervisor.go:31 SuperviseInterval = 30 * time.Second`、`:50 Work`、`:56 Supervise`、`:92 superviseRun`。`RunOnStart: true` 於 `cmd/worker/main.go:118-124`。**leader-only 是 River periodic job 的既有語意，不是本 repo 自寫的斷言**——這點值得記著（升級 River 時屬行為相依）。重入列碰撞由 `service.go:296 executeInsertOpts()` 的 unique opts 吸收。測試 `TestSupervisorRecoversARunThatHasNoJob`、`TestSupervisorTimesOutARunThatOutlivedItsWallClock`、`TestARunWithNoAttemptToResumeIsTerminatedSafely` |
| RUN-009 Provider 契約測試套件 | ✅ 維持 | `run/provider_contract_test.go` ＋ `run/providertest/fake.go`；`SKILLHUB_PROVIDER_CONTRACT_URL`／`_TOKEN`（`:36-37`）切換 fake→真實服務（`newTarget:54`）。`TestProviderContract:125`、`TestSchedulerAcceptsTheTargetsRealCapability:284`、`TestProviderRefusalIsClassifiedAndNotRetried:302`。滿足 NFR-006 第 3 條 |

> **7d8f255 值得單獨記一筆**：契約測試套件對真實 `services/sandbox` 跑的時候發現排程器把它整個拒掉——每個端點都合規，卻沒有一個 Run 派得出去。修正是「Provider 的 egress 比 Run 要求的更強時應接受」，方向單一（不以較弱模式頂替較強請求）。**這正是 RUN-009 存在的理由被兌現的一次**，不是額外的除錯插曲。

---

## 5. §11 SelfHostedProvider（M2）

| 項目 | 判定 | 關鍵證據／缺口 |
| --- | --- | --- |
| SBX-001 隔離技術與拓撲決定 | ✅ 維持 | [ADR-015](../../../adr/ADR-015-sandbox-isolation-technology.md) Accepted；`SKILLHUB_SANDBOX_RUNTIME=runsc` 落實，宣告的 `isolation.level` 跟實際設定走。本項是「決定」性質，實跑 runsc 屬部署期 |
| **SBX-002 經審核的 Runtime Image** | **維持不勾（部分完成）** | 流水線已接上：`.github/workflows/runtime-image.yml` 的 digest 斷言（grep `^FROM .+@sha256:[0-9a-f]{64}$`）→ build → syft SPDX → grype → 門檻閘門，`if: always()` 上傳在閘門**之前**故掃描失敗也留證據。**暫定狀態在三處明示**：workflow step 名稱即為 `Fail on fixable Critical/High (I-06, provisional threshold)`（註解 `PROVISIONAL — SEC-002 Q18 has no signed-off value yet`）、`infra/images/README.md` 的「門檻提案值（暫定，待負責人定案）」、`Dockerfile:18-20`。**不勾正確**：依 `02:SEC-002`「未定值前該項不可自動化判定，不得記為通過」。另 I-03 的 SBOM 保存落在 90 天 CI artifact，待 container registry（ADR-019 待決策 1）定案後搬家 |
| SBX-003 獨立環境與暫存空間 | ✅ 維持 | `dockerdrv/docker.go:128` 每 attempt 一容器；`:170-175` `/work`／`/out`／`/tmp` 為該容器私有 tmpfs。基線 C-01 |
| SBX-004 非 root、非特權、唯讀 rootfs | ✅ 維持 | `docker.go:151 User`、`:176-189` `CapDrop: ALL`／`no-new-privileges:true`／`Privileged: false`／`ReadonlyRootfs: true`。**驗證是雙層的**：`TestLiveSandboxMeetsTheIsolationBaseline`（`docker_test.go:141`）先在**真實容器內**跑探針並斷言其輸出（`uid=65532`、`rootfs=readonly`、`docker-socket=absent`、`net=isolated`，`:143-164`），再另做 `ContainerInspect` 設定斷言（`:171-237`）。C-02／03／06／08 |
| **SBX-005 阻擋管理 Socket、主機路徑與內部服務** | **維持不勾（部分完成）** | 已擋的部分有硬證據：`Binds`／`Mounts` 恆空（`docker.go:180-186` 的註解說明零值是刻意的），斷言於 `docker_test.go:196`；namespace 全私有（`:202-209`，拒絕 `host` 與 `container:` 前綴）；`docker.sock` 不存在為容器內活體探測（`:148`）。**未成立的是「內部服務存取」**：dev 已由網路面隔離（SBX-007），但 P-02「Sandbox → 核心資料庫連線嘗試被實際阻擋」的**常駐探針**全 repo 無實作，屬部署期。C-04／05／07 |
| SBX-006 CPU／記憶體／磁碟／程序數／時間限制 | ✅ 維持 | `docker.go:190-199` `NanoCPUs`／`Memory`＋`MemorySwap`（相等，不給 swap）／`PidsLimit`／`nofile` soft＋hard／`core` 0；tmpfs 3:1 切分於 `:139-140`（`workBytes := lim.DiskBytes * 3 / 4`）；硬牆鐘蓋在容器 label（`:399-401`）。**真實容器測試**：`TestPidsLimitStopsAForkBomb`（`docker_test.go:242`，實際 fork 200 個行程撞 16 上限）、`TestWallClockStopsALiveSandboxAndDestroyReleasesIt`（`:255`）。C-10～C-15。**注意：本項的字面清單不含 token，見 §9.1** |
| **SBX-007 預設封鎖的網路出口政策與允許清單** | **維持不勾（部分完成）** | `dockerdrv/docker.go:236-246 networkFor`——**只有**當 allow 有 `Purpose == "model_gateway"` 才回 `d.cfg.Network`，否則 `"none"`；節點無網路設定時強制 `"none"`；`NetworkDisabled` 於 `:160`。反向保護有具名測試：`TestAcceptRefusesAnAllowListANodeCannotRoute`（`sandbox/artifacts_test.go:156`，宣告 `egress_modes: ["none"]` 的節點對帶 `model_gateway` 的請求回 `ClassCapabilityMismatch`，同一請求 `Allow = nil` 則接受）。**不勾正確**：生產級 Egress Proxy 本體、域名允許清單、DNS 固定解析、目的地記錄（N-01～N-07）全屬部署期，允許清單管理流程仍是 ADR-015／威脅模型 Q3 待決策 |
| **SBX-008 Dataset／Skill／Secrets／Artifact 短效傳遞** | ✅ 維持 | `02:RUN-003`、SEC-005。預簽：`run/grants.go:65/89` `PresignGet`、`:106-111` `PresignPut`；TTL `= WallClockHardSeconds + grantSlack(5m)`（`grants.go:31`、`schedule.go:233`）。Virtual Key：`run/gateway.go:130-147` `/key/generate` 帶 `max_budget`＋`tpm_limit`＋`"models": [g.Model]` 層級限制。**fail-closed**：`schedule.go:246-251`，簽發失敗即不進 `RunRequest`。撤銷：`gateway.go:166-175` `/key/delete` 以 alias（`:120 keyAlias`）定址，未知 alias 由 `gatewayError.notFound():188-195` 吸收為冪等。傳遞：`dockerdrv/transfer.go` `/bin/tee` 入、`/bin/tar` 出；交接常數 `.workload-done`／`.collected`，生產端 `run.mjs:119-130 waitToBeCollected()`，順序測試 `TestCollectionHappensBeforeTheWorkloadIsReleased`（`artifacts_test.go:95`）。**實證**：基準試跑 73 個 Run 全部撤銷成功 |
| SBX-009 四條終態路徑的清理 | ✅ 維持 | `run/cleanup.go:139`／`:254` 皆 `provider.Destroy`。`TestDestroyIsIdempotentAndHasNo404`（`sandbox/http_test.go:317`：兩次 DELETE 皆 204、對不存在的 handle 也 204、driver 實際只移除 2 次）、`TestDestroyReports500WhenResourcesAreStillHeld`（`:340`）、真實容器 `TestWallClockStopsALiveSandboxAndDestroyReleasesIt`（`docker_test.go:294-302`）。X-01 |
| **SBX-010 隔離／資源耗盡／網路／清理失敗測試** | **維持不勾（部署期）** | 逃逸測試與 gVisor 相容性需要 Linux 與巢狀虛擬化（ADR-019 待決策 3）。**現有的真實容器驗證不等於逃逸測試**，`03` 的註記誠實。ADR-015 定案紀錄：**SEC-009／SBX-010 未通過不得開放外部使用者提交 Skill 執行** |

> **Runtime Image 的 Python 缺席已確認為事實**：`infra/images/runtime-agent-sdk/Dockerfile` 基底 `node:22-bookworm-slim@sha256:d649c27d…`，只 `apt-get install --no-install-recommends unzip`，且**額外移除了 npm／npx／corepack**（安全強化，`03` 的 SBX-002 註記已載）。無 python3、無 pip。這對應 [content-baseline-report.md §6.3](content-baseline-report.md) 的 33／45 Skill 依賴 Python，見 §9.5。

---

## 6. §13 Trace 與平台 O11y（M2）

**十一項全部維持勾選。** 這是 M2 證據密度最高的一節。

| 項目 | 判定 | 關鍵證據 |
| --- | --- | --- |
| TRACE-001 Trace Event Schema | ✅ 維持 | [`contracts/events/trace-event.schema.json`](../../../contracts/events/trace-event.schema.json)＋README；`0019_trace_ingestion.sql` 補齊 `event_id`／`attempt`／`schema_version`／遮罩狀態四個缺口，`02:TRACE-001` 第 1、5 條因此才成立 |
| TRACE-002 Skill 啟用與資源讀取 | ✅ 維持 | `run.mjs:228` `skill_activation`（`block.name === "Skill"`）、`:251` `resource_read`（經 `:211 skillResourcePath()` 要求路徑落在 `skillDir` 下並相對化，不外洩沙箱絕對路徑）；收集端 `services/sandbox/internal/sandbox/trace.go`（2 秒 ticker、at-least-once、未送批次重送）。`decision: skipped` 不可觀測的限制註記正確 |
| TRACE-003 Tool Call／MCP／Script Log | ✅ 維持 | `run.mjs:272-280` `tool_call` 帶 `duration_ms`，計時對由 `:219 pending.set(block.id, {at})` → `:245 Date.now() - open.at`；`:261-270` `script_log` 僅在 `open.name === "Bash"`。MCP 不實作與 TEST-005／006 同步 |
| **TRACE-004 Agent 輸出、錯誤、延遲、Token 與成本** | ✅ 維持（**兩個洞見 §9.2，屬殘項不屬虛報**） | `run.mjs:296 gatewaySpend()` 以**本 Run 自己的** `ANTHROPIC_AUTH_TOKEN` 打 `/key/info` 讀 `info.spend`，2.5 秒初始等待＋6 次輪詢至穩定，失敗回 `null`；`cost_source: "gateway"` 僅在非 null 時設定；SDK 的 `total_cost_usd` 於 `:360-366` 明文拒用。控制平面在 `failed`／`timed_out` 轉移的同交易內寫 `error` 事件。**null 語意在全鏈保持**：`UsageSummary.CostUSD *float64`（`trace/service.go:318-323` 附註解）、`TestSummaryKeepsUnreportedCostNil`、UI `RunTrace.tsx:133` 渲染「未回報」；**完全沒有 usage 事件時 `s.Usage` 為 nil，UI 顯示「沒有記錄到用量事件。」而非 0 token**——這一點經查證是誠實的 |
| TRACE-005 Secrets 與敏感欄位遮罩 | ✅ 維持 | `trace/mask.go`，於 `service.go ingestOne` 寫入**前**執行；`0019:41` `masked boolean NOT NULL DEFAULT false CHECK (masked)` 讓跳過遮罩在資料庫層不可能。`(*Masker).walk:93-116` 遞迴走訪 map／slice 只對 `case string` 動手（比 README §6 的「只掃 secret_bearing 欄位」更嚴）；`Placeholder = "[REDACTED]"`（`:39`）不保留長度；`masked_fields` 以 RFC 6901 escaping 的 JSON Pointer 記錄（`:138 escapePointer`）。單元 `TestMaskerRedactsSecretsAndRecordsWhere`／`TestMaskerReplacesWholeValueWithFixedPlaceholder`／`TestMaskerIgnoresShortKnownValues`；**整合測試直接查庫**：`TestTraceIngestionMasksBeforeStorageAndDedupesOnResend`（`identity/trace_integration_test.go:212`，`:262` `SELECT payload::text FROM trace_events`，`:266` 以「plaintext secret in trace_events.payload」失敗）。實證：1112／1112 `masked`（§2.3） |
| TRACE-006 一般模式進度摘要 | ✅ 維持 | `trace/http.go:89` 預設分支 → `Svc.General`。**Run 狀態取自 `runs` 表**（`service.go` `Summary` 註解明文「The only authority on run state is the runs table (iron rule 5)」），步驟取自 `run_status_transitions`（`db/queries/runs.sql:108-111`），**不重播 `run_lifecycle` 重建狀態**——鐵律 5 成立 |
| TRACE-007 進階模式 | ✅ 維持 | `http.go:113` `?mode=advanced`；排序 `db/queries/trace.sql:24 ORDER BY occurred_at, source, attempt, seq`（比宣稱的三鍵多一個 `attempt`，是超集不是矛盾）；`StreamHealth.MissingSeq`／`LateEvents`（`service.go:233-242`）、`AdvancedView.Complete`（`:254`）。inert text：`apps/web/src/trace.test.tsx:121` 注入 `<img src=x onerror=alert(1)>`，`:161` 斷言其在 `<pre>.textContent` 內逐字出現。路由 `router.tsx:110 /runs/$runId` |
| TRACE-008 排序、重送、缺失與延遲 | ✅ 維持 | `db/queries/trace.sql:16 ON CONFLICT (event_id, occurred_at) DO NOTHING`＋`0019:16` 唯一索引；`IngestReport.Duplicate`（`service.go:46`、`:88-90` 經 `errors.Is(err, errDuplicate)`）——重送是回報不是錯誤；`late := terminal(run.Status)`（`:73`）。測試 `TestStreamHealthNamesTheMissingSequenceNumbers`、`TestAdvancedViewNamesMissingEventsAndRefusesToLookComplete`、`TestEventsArrivingAfterTheRunFinishedAreKeptAndFlagged`。實證：45／45 Run 無斷號 |
| O11Y-001 平台指標 | ✅ 維持 | `internal/platform/metrics/metrics.go`；獨立 listener `metrics.Serve(os.Getenv("METRICS_ADDR"))`（`cmd/api/main.go:151`、`cmd/worker/main.go:141`），未設即停用（`metrics.go:202`）——**不掛在對外 API port 上** |
| O11Y-002 Provider 健康度與錯誤 | ✅ 維持 | `skillhub_provider_capability_total`（`metrics.go:107`）、`skillhub_provider_request_total`（`:114`）；`infra/observability/alerts.yml` |
| O11Y-003 遺留 Sandbox／資源／安全告警 | ✅ 維持 | `skillhub_run_cleanup_backlog`（`:98`）、`skillhub_orphan_sandbox_total`（`:134`）；`TraceMaskingStopped` 於 `alerts.yml:195`（**NFR-002 沒有其他偵測器**，理由記於 `README.md:89`）。Alertmanager 部署、通知路由、Grafana dashboard **明確未做**，門檻為首發預設非校準值——`03` 已載，誠實 |

---

## 7. §4 CONTENT-007／008（自 M1 移入 M2）

| 項目 | 判定 | 逐條對照 |
| --- | --- | --- |
| **CONTENT-007** 範例資料、Prompt 與驗收條件 | **維持不勾（部分完成）** | `02` §4.7 五條：①每個精選一組 Dataset／Prompt／驗收條件且可散布 ✅（15/15，合成資料）；②Prompt 明確點名該 Skill ✅（模板首句即點名，未提供不點名變體）；③**`writing` 類精選附可編輯 rubric 供 LLM Judge 逐項回傳證據引文 ❌**——本批給的是三條通用驗收條件，EVAL-001／002 的 Judge 介面尚未實作，rubric 沒有消費端；④範例資料無 Secrets／個資 ✅（1112 事件全數通過遮罩）；⑤實際使用的 Prompt 與驗收條件以快照保存 ✅（`test_case_snapshots` 73 列，不可變）。**四條達成、一條未達成，不勾正確** |
| **CONTENT-008** 平台基準試跑 | **✅ 維持勾選** | `02` §4.7 五條全數達成：①精選 15/15「符合」（`succeeded` ＋ trace 有 `skill_activation` ＋ artifact 確有檔案）；②在隔離 Sandbox 內執行，不在匯入或掃描階段 ✅（全部經 sandboxd 派送至獨立容器）；③可追溯 Skill Version／Test Case 快照／Provider／Runtime／Trace ✅（[報告 §9](content-baseline-report.md)，且**本次已回資料庫重查通過**，見 §2.3）；④未通過者不得標記精選 ✅（無精選未通過）；⑤source-available 照常試跑不產 Download Artifact ✅。九項精選檢查的 ⑧ 由 `pending` 改記 `pass` 成立 |

> **2026-08-16 回填：CONTENT-005 的 44/45 不影響本節兩項判定。** `afb5767` 關閉了基準試跑查出的 11 筆 Python 揭露缺口，`docx` 兩輪未過而使 CONTENT-005 由 45/45 退為 44/45（裁定見 [README 乙-11](README.md)）。**`docx` 是「已索引」層不是精選**，而 CONTENT-007／008 的允收對象是**精選 15 筆**——15 筆全數不在該清單內，故 CONTENT-008 維持勾選、CONTENT-007 的不勾理由仍只有 rubric 一項。受牽動的是 CONTENT-005 自己與 CONTENT-003 的檢查 ⑦（皆屬 M1 節，不在本對帳範圍）。

> **判定的誠實度值得記一筆**：報告把「Run `succeeded` 但 `/out/artifacts/` 是空的」（`date-wrangling`）記為「未產出」而非四捨五入成符合，並指出 `run.mjs` 的 `finish("succeeded")` 只代表「agent 這一輪沒有拋錯」、與任務是否完成無關。**這正是 EVAL-001 存在的理由**，見 §10。

---

## 8. §16 安全（M2 相關的兩項）

| 項目 | 判定 | 說明 |
| --- | --- | --- |
| **SEC-002** Sandbox 最低安全基線與阻擋條件 | **維持不勾** | `02:SEC-002` 的兩個勾選前提都未成立：(1) 六項無值門檻（Q18：P-03 節點重建週期、P-04 gVisor 安全基準版本與更新 SLA、I-04 掃描結果有效期、I-06 漏洞等級門檻、X-02 Reconciler 掃描頻率、X-03／X-04 遺留資源告警與暫停門檻）**仍全部無值**——SBX-002 已就 I-04／I-06 提出建議值（可修的 Critical／High 阻擋、有效期 30 天）並在程式與 CI 標為暫定，其餘四項連提案都還沒有；(2) Q1～Q3（節點編排方案、節點是否單租戶、Egress Proxy 實作與允許清單管理流程）仍未答。**另見 §9.4：閘門 B 的四項額外阻擋只落地兩項** |
| **SEC-009** 逃逸、資源濫用與權限提升測試 | **維持不勾** | 需 Linux 與巢狀虛擬化（ADR-019 待決策 3）。`02:SEC-009` 明文「M2 的 SelfHostedProvider 驗收必須全數通過」——**這條在 M2 結束時未達成**，依 ADR-015 的定案語意界線，其後果是「不得開放外部使用者提交 Skill 執行」，見 §10 甲類 |

---

## 9. 本次新發現的缺口（七項，皆無工作項承接）

以下七項**都不 falsify 任何一個勾選**——這正是要單獨列出來的原因：它們掉在所有工作項的字面範圍之間。

### 9.1 ⚠️ PDM-005 的 token 硬上限沒有任何人把關（**最重的一項**）

**現況**：`300_000` / `60_000` 這兩個數字是**宣告值**，不是閘門。

| 存在的 | 位置 |
| --- | --- |
| 型別與預設值 | `services/platform/internal/run/service.go:133-136`（`ResourceLimits.TokenBudget`）、`:154-155`（`DefaultResourceLimits()` 設 `300_000` / `60_000`） |
| 凍進 `runs.policy_snapshot` | 隨 `defaultPolicy()` |
| **顯示給使用者並要求他確認** | `apps/web/src/pages/RunPreflight.tsx:165-166`（型別 `api/lab.ts:52`、斷言 `lab.test.tsx:64`）——**使用者按「確認」時同意的摘要裡有這兩個數字** |
| 契約 | `contracts/openapi/sandbox-provider.yaml:654-655`、`public.yaml:2673-2680` |
| 沙箱側型別 | `services/sandbox/internal/sandbox/contract.go:129-131` |

**不存在的**：

- 平台**從不填**這個欄位——`run/job.go` 的 `buildRunRequest` 沒有設 `TokenBudget`，沙箱收到的 `RunRequest` 裡根本沒有它。
- 沙箱**從不讀**它——全 `services/sandbox/` 內 `TokenBudget` 只出現在 `contract.go:111` 與 `:129-131`，零消費端。
- **沒有任何累加器**——`run/provider.go:186-187` 有 `InputTokens`／`OutputTokens` 欄位，但沒有東西跨輪加總、也沒有東西拿總和比對上限；`job.go`／`supervisor.go`／`trace/service.go` 都沒有由 token 數觸發的取消。
- `run.mjs` 也不強制（`:369-375` 只發事件；`:126` 的 `60_000` 是 60 秒逾時**不是** token 上限——naive grep 會誤中）。

**實際生效的煞車只有三個**：`max_budget` $0.50、`tpm_limit` 200K（`run/gateway.go:37-38`）、牆鐘 600s 軟／900s 硬。而 **PDM-005 §5.2a-4 明文寫過這三個都不是 token 上限**：「金額預算與 token 上限已脫鉤約 7～8 倍，`max_budget` 不能再代理此上限」，`gateway.go:29-31` 的註解甚至照抄了這句話——**程式碼記錄了這個洞，而不是關掉它**。

**歸屬分析（誰的殘項）**：

| 候選 | 判定 |
| --- | --- |
| `SBX-006` | **不是**。標題與 `02:RUN-003` 的字面清單都是「CPU、記憶體、磁碟、程序數與執行時間」，**沒有 token**。勾選成立 |
| `RUN-005` | **不是**。它做的是 Provider 能力相容檢查（六項資源上限的**相容性**），不是執行期強制 |
| `TRACE-004` | **不是**。它負責**回報** token，且回報是成立的 |
| `SBX-008` | **不是**。它負責鑄／撤金鑰與兩個閘道煞車，兩者都在 |
| `PDM-005` | 決策存在（v5 有值），但 `03` §1 的 `PDM-005` 仍是 `[ ]`，且 PDM 是**決策**項不是實作項 |
| **結論** | **無人承接。** 這是一個孤兒殘項 |

**附帶的第二個缺口**：PDM-005 §5.2a-2 明文要求「回寫 `02:RUN-003` 時**必須帶** 每輪工具呼叫次數對應的輪數換算表（15／7.7／5 輪），不能只寫『300K』」。該表**從未寫進 `02`**，程式註解裡也沒有。

**建議處置（需負責人裁定，三文件同步）**：擇一——(a) 新增 M2 補件或 M3 前置的實作工作項「依閘道回報的 `input_tokens` 累計並於超限時終止 Run」，並把 §5.2a 的輪數表寫進 `02:RUN-003`；(b) 正式承認 MVP 不做 token 硬上限，把 `TokenBudget` 從**權限摘要**移除（**不能留在畫面上**——讓使用者確認一個平台不會執行的上限，是 NFR-001「UI 不得誤導」的直接違反），並在 `02` 記錄以金額＋速率＋牆鐘三者代替。**現狀（顯示但不強制）是兩者中最壞的一種。**

### 9.2 TRACE-004 的兩個洞（已被報告記錄，此處確認並定位）

**洞一：沒有 `result` 訊息時，成本與 token 完全不存在。** 已逐行確認，`usage` 只在**唯一一處**發出，且巢狀在 `result` 分支內（`infra/images/runtime-agent-sdk/run.mjs:354-380`）：

```js
354:    if (msg.type === "result") {
...
367:      const usage = msg.usage ?? {};
368:      const cost = await gatewaySpend();
369:      emit("usage", { scope: "run_total", ... cost_usd: cost, ... });
380:    }
```

`for await` 迴圈的另一個出口是 `:382` 的 `catch` → `fail()` → `emit("error")` → `finish("failed")` → `process.exit`。因此**串流在沒有 `result` 的情況下結束（崩潰、被牆鐘殺掉、SDK abort）就完全不發 `usage`**——不是發一個 0，是不發。下游沒有任何補償。實例：`add-iso3166`（Run 成功、13 個事件、無斷號、`complete: true`，token 與成本在平台端不存在）。

**這件事的兩個後果，都要交接**：

1. **`complete: true` 不代表 usage 存在。** 斷號偵測只看得到「發出後遺失」，看不到「從未發出」。UI 誠實（顯示「沒有記錄到用量事件。」），但 `complete` 旗標會讓自動化消費端誤判。
2. **成本合計必然是下界。** 報告 §5.2：Trace 合計 $3.0879 vs 閘道實付 $3.3932。**EVAL-012（版本／成本比較，M3）若直接加總 Trace 的 `cost_usd`，會系統性低估**，且低估幅度隨失敗率上升。權威來源仍是閘道 per-key spend（ADR-017）。

**洞二：閘道預算計數與其自身 spend log 差最多 50 倍。** [報告 §6.2](content-baseline-report.md) 有完整對照實驗（宣稱 `Current cost: 0.50057` 的金鑰，其 `LiteLLM_SpendLogs` 實際只有 `0.02717`；兩個新鑄金鑰的單次呼叫實測 spend 皆為 `$0.0000285`；把上限提到 $2.00 後 7 個受影響精選全數一次通過，實花 $0.054–$0.169）。**16 個 Run 被平台自己掐掉**，其中 9 個至今沒有有效基準。

在查清原因之前，**per-Run `max_budget` 不是一個可信的成本閘門**——它會在遠低於名目上限處觸發。這連帶影響 §9.1：現在三個煞車裡最重要的那個（金額）本身也不可信，實際只剩 `tpm_limit` 與牆鐘。

### 9.3 執行前權限摘要缺 PDM-005 §5.3 指定的兩個欄位

PDM-005 §5.3 明文列出「`02:TEST-005` 權限摘要的具體欄位」，其中兩項在 `PermissionSummaryContent`（`run/preflight.go:67-80`）裡不存在：

| 欄位 | 現況 |
| --- | --- |
| **預估成本區間** | **完全沒有。** PDM-005 §5.2a-6 特別強調「必須是**區間**不是單值——首次與後續 Run 的單位成本差約 8 倍（prompt caching 跨 Run 命中）」 |
| 預計使用的 Runtime 版本 | 有（`ProviderSummary`），但值來自排程快照 |

**根因是文件未同步**：PDM-005 §5.3 的欄位清單從未寫回 `02:TEST-005`，所以 `02` 的字面準則（Dataset、Script、工具、MCP、網路、Secrets、Provider 與資源限制）已被 TEST-008 完全滿足，**缺的部分不可判定**。這與 §9.1 的第二個缺口同型：**PDM 提案裡指定要回寫 `02` 的內容沒有回寫。**

### 9.4 SEC-002 閘門 B 的四項額外阻擋只落地兩項

`02:SEC-002`：「除基線檢查外，閘門 B 另阻擋：…」

| 阻擋條件 | 現況 |
| --- | --- |
| 使用者未確認或未重新確認執行前權限摘要（`TEST-005`） | ✅ **已落地**（TEST-009，`run/http.go:128-132` 422） |
| 請求能力超出 Provider 宣告能力（`RUN-001`） | ✅ **已落地**（RUN-005，`run/http.go:136-139` 422，排入佇列前） |
| Skill Version 靜態掃描結果為阻擋級（依 `SEC-003` 政策） | ❌ **未落地**。`internal/run` 內完全沒有 severity／blocking 判斷。且 `02:SEC-003` 自陳「此政策未定案前，閘門 B 的該條件不可判定」——**根因是威脅模型 Q7（阻擋 vs 警告的具體 Policy）未決** |
| 超出 Workspace 並行或額度上限 | ❌ **未落地**。全 `internal/run` 沒有任何 workspace 並行計數；`provider.go:84 ConcurrentRunSlots` 是 **Provider 側容量**，不是 Workspace 配額。PDM-005 §5.2 已定值（**每 Workspace 並行 Run 上限＝2**），**值在，強制不在** |

第 4 項與 §9.1 同型：**PDM-005 定了值，沒有工作項承接強制。**

### 9.5 Runtime Image 沒有 Python，而目錄無欄位表達這件事

[報告 §6.3](content-baseline-report.md)：45 個 Skill 中 33 個 `deps_runtime = python`，映像沒有 python3（本次已確認 Dockerfile）。33 個裡仍有 24 個判定符合——**但執行的不是 Skill 帶的腳本，而是模型對腳本的轉譯**。同一個 Skill 在裝了 Python 的環境與這裡會是兩種行為，而目錄沒有任何欄位透露這件事。

這**不是**一個工程細節而是產品決策，且它與 §9.6 是同一件事的兩面。

### 9.6 `01` 的 M2 里程碑第 4 項未達成：Agent 相容篩選維度

`01-goals-and-plan.md:195` 的 M2 目標明列：「**依 Sandbox 實測結果啟用搜尋的 Agent 相容篩選維度（`02:DISC-002`，2026-08-15 標定）**」。`02:DISC-002` 的「篩選維度的允收階段」表把 Agent 相容標為 **M2（依 Sandbox 實測）**。

**M2 結束時未啟用。** [報告 §8](content-baseline-report.md) 已查明：schema 沒有任何可回寫的欄位，值是 `catalog/http.go resultFacets()` 與 `detail.go compatibility{}` 裡寫死的 `unverified`，API 端目前**正確地**回 400 拒絕該篩選。報告依交辦沒有發明 schema，並留下四個待決問題（欄位歸屬是 Skill Version 層級還是 Skill Version × Runtime Image 層級、兩軸判準、樣本量、值域是否需要第三態）。

**這不影響 `DISC-003` 的勾選**（M1 已依修訂後的分階段允收準則勾選，Agent 維度的解除條件明文掛在 M2 Sandbox 而非 DISC-003 底下），但**它是一條未達成的 M2 里程碑目標**，必須在完結報告裡明說，不能默默滑進 M3。

報告已備妥可直接回填的實測值：`capability = activated` 45/45；`runtime` 為 11 個原生可執行、1 個 node 可執行、33 個「腳本不可執行、由模型轉譯」。**資料在，欄位不在。**

### 9.7 ~~`services/sandbox/README.md:112` 記載的是被推翻前的 SDK 行為~~ → **已關閉（`ddc3e54`）**

> **2026-08-16 回填**：該行已改為一張條件表（`cwd` 指向持有 `.claude/skills/` 的目錄／`settingSources` **省略**／`skills: "all"`／`allowedTools` 傳 `'Skill'` 已 deprecated），並補上 `<name>/` 那層不能省的理由（SDK 以「一個目錄一個 skill」發現，倒進 skills 根目錄會被發現為零個）、反轉的實測依據，以及與 `run.mjs` 檔頭相同的警語：**每一條都是靜默失效，這一條已經反轉過一次，SDK 升級必須重新實測而非推理**。
>
> **仍未處理的那半**：SDK 版本只釘在 `Dockerfile` 的 `ARG CLAUDE_AGENT_SDK_VERSION=0.3.233`，**沒有任何 ADR 記錄這次行為反轉**。下方原文保留供回溯。



第四批以實測推翻了 PDM-003 Spike（claude-agent-sdk 0.2.137）的假設：**Agent SDK 0.3.233 上 `settingSources: ["project"]` 完全發現不到專案 skill，必須省略 `settingSources`**；且 skill 啟用已從 `allowedTools` 移到獨立的 `skills` 選項。這件事在三個地方寫對了：

| 位置 | 內容 |
| --- | --- |
| `infra/images/runtime-agent-sdk/run.mjs:8-23` | 完整檔頭：四個載入條件、0.3.233 的反轉、`skills: "all"`、`HOME` 指向 `/work` 故省略 `settingSources` 不會讓映像的 `~/.claude` 滲進來、**每一條都是靜默失效** |
| `run.mjs:334-336` | `skills: "all"` 上方的行內註解 `// settingSources deliberately omitted — see the header.` |
| [m2/README.md 第四批](README.md) | 散文版，含兩個 SDK 版本與 `allowedTools: 'Skill'` 已 deprecated |

**但第四處寫的是反的**：

> `services/sandbox/README.md:112`：「四個啟用條件（`cwd`、**`settingSources` 含 `project`**、skill 啟用、工具清單含 `Skill`）全部寫在 `run.mjs`，缺一則 Skill 靜默不載入。」

這是 0.2.137 時代的敘述，**在 0.3.233 上照做會得到零個 skill**，而且它把讀者指向 `run.mjs`——那裡寫的正好相反。`services/sandbox/README.md` 是沙箱服務的自然入口，先讀到它的人會拿到倒過來的指示。

另外，**SDK 版本只釘在 `Dockerfile` 的 `ARG CLAUDE_AGENT_SDK_VERSION=0.3.233`**（`sandboxd/main.go:66` 有同值預設），沒有 ADR 記錄這個行為反轉。ADR-012／015 都沒提。

**建議**：修 `services/sandbox/README.md:112`（一行，屬程式碼側檔案，本次對帳未動），並考慮把「SDK 行為只能實測不能推理」這件事留一個更持久的落點——它已經在一個里程碑內被推翻過一次。

### 9.8 兩個增強管線的平台缺陷（**2026-08-16 回填，由 `afb5767` 的修正輪查出**）

兩者都與 M2 的工作項無關，但**都是 M2 基準試跑的副產物直接暴露出來的**，且**都沒有工作項承接**——承接它們的 `INGEST-009` 已勾選並隨 M1 結案。歸類為乙（待決策）而非丙（移交 M3）：M3 是評估里程碑，這兩件事不碰評估路徑；要修得先有人裁定「重開 `INGEST-009` 還是新增工作項」。

**(a) 增強實際只有 30 秒，而且逾時仍在閘道產生費用。**

| 位置 | 值 |
| --- | --- |
| `services/platform/internal/llmclient/client.go:25` | `return &http.Client{Timeout: 30 * time.Second}`（`httpClient()` 的預設，`HTTPClient` 為 nil 時生效） |
| `services/platform/internal/ingest/enrich.go:32` | `enrichTimeout = 75 * time.Second`，於 `:107` `context.WithTimeout` |

`http.Client.Timeout` 是**整個請求（含 body 讀取）的硬期限**，30 秒先到，那個 75 秒的 ctx 預算因此從來沒被用到。實測後果記於 [content-review-report.md §11](../m1/content-review-report.md)：修正輪 14 次成功增強中有 **3 次逾時**（`docx` 是 `/embed` 逾時，`excel-split`／`excel-delete` 是增強逾時），重跑即過。

**真正的代價不是重跑而是錢**：client 端放棄不會取消上游的模型呼叫，**每一次逾時都已在閘道計費**。建議把逾時交給 ctx 單一控制，不要在 client 上另設一個更短的硬期限——**兩個逾時裡較短的那個才是真的，而較長的那個是寫在程式裡的一個誤解**。

**(b) `ReindexAll` 把全庫 `updated_at` 推成 `now()`，補跑清單因此無法用時間戳篩選。**

`db/queries/search.sql:244-253` 對每一筆存活 skill 無條件 `updated_at = now()`（INSERT 的 SELECT 與 `ON CONFLICT DO UPDATE` 兩邊都是）。而 `ListPendingEnrichment` 的語意是「全庫 pending、oldest first」，所以 **`updated_at` 與 `REINDEX_BATCH` 都擋不住** M2 基準試跑在 `content-baseline` 臨時 Workspace（`91b951b3…`）留下的 **45 筆 fork 文件**——它們各有一筆 `enrichment_status='pending'` 的 `search_documents`，照跑會多花約 45 次旗艦增強呼叫（**約 $2**）。

修正輪的處置是把那 45 筆暫標 `enriched`、跑完立刻還原（以 `enriched_summary=''` ＋ `embedding IS NULL` 為還原條件，實測還原 45/45 逐欄相同）。**這是每次補跑都要重做一次的人工步驟，不是修好了。** 這件事在 M2 之前不會發作——是基準試跑第一次在庫裡放進大量 fork 文件才讓它可見。

---

## 10. 收斂建議與順序

依「解除下游阻擋的數量」排序，不依工作量。

### 第一梯：進 M3 之前該關的

| 順序 | 項目 | 為什麼是這個順位 |
| --- | --- | --- |
| **1** | **§9.1 token 硬上限二選一裁定** | 這是唯一一個「使用者被要求確認一個平台不會執行的限制」的地方，直接踩 NFR-001。且 §9.2 洞二讓金額煞車也不可信，三個煞車實剩兩個 |
| **2** | **§9.2 洞二：定位閘道預算計數的 50 倍誤差** | 9 個已索引 Skill 沒有有效基準卡在這裡；PDM-003 v5 的 $0.50 預設值在查清前不可信；補跑預估 $1.0–1.5 |
| ~~3~~ | ~~§9.7 修 `services/sandbox/README.md:112`~~ | ✅ **已完成（`ddc3e54`）**。剩下的半件是「這次行為反轉沒有 ADR 記錄」 |
| **3** | **§9.8(a) 增強的 30 秒硬期限** | 一行的修改（逾時交給 ctx），但每次逾時都在閘道花錢；而任何 CONTENT-005 的後續修正輪都會再踩一次 |
| **4** | **§9.6 Agent 相容軸的四個待決先答** | 資料已備妥（45/45 activated、33 個模型轉譯），只缺欄位設計決策。答完才談 migration |

### 第二梯：M2 內仍缺的實作

| 順序 | 項目 | 備註 |
| --- | --- | --- |
| 5 | **§3.2 §9 TEST 的 UI 範圍裁定** | 二選一（承認含 UI 並補工作項／明文界定在 API 層），任一都要三文件同步。**TEST-004 已退回，其餘三項等裁定** |
| 6 | **§9.4 Workspace 並行上限（＝2）** | PDM-005 已定值，SEC-002 閘門 B 已列為阻擋條件，缺的只是實作。是四項裡唯一不依賴任何未決策的 |
| 7 | **§9.3 預估成本區間進權限摘要**＋**§9.1 的輪數表回寫 `02:RUN-003`** | 兩者都是「PDM 指定要回寫 `02` 而沒回寫」，一起做 |
| 8 | **§9.2 洞一：`run.mjs` 的 usage 事件移出 `result` 分支** | 或在 `finish()` 的所有路徑上補一次 spend 讀取。影響 EVAL-012 的成本比較準確度 |
| 9 | **§9.8(b) `ReindexAll` 的 `updated_at = now()`** | 不修就是每次補跑都要人工圈掉 45 筆 fork 文件，漏一次多花 $2 |
| 10 | **`docx` 的裁定（CONTENT-005 44/45）** | 三條路徑見 [README 乙-11](README.md)。**選「下架」前要先看清楚代價**——它是 golden query D01 的 gold primary 且為現行 Top-1 |

### 第三梯：M2 內結構性不可能完成（→ 部署期／待決策，見 [README.md](README.md) 的殘項三類清單）

SEC-009、SBX-010、SBX-005／007 的生產網路面、SBX-002 的門檻定值、SEC-002 的六項門檻與 Q1～Q3。**這些不是拖延，是它們需要 Linux＋巢狀虛擬化與生產網路，本機 Windows 開發環境結構性做不到**（`README.md`「開發環境限制」已誠實記錄）。

### 一句話總結

**M2 的帳是準的——41 項只退回 1 項，且退回的理由是 UI 而非後端。** 真正的問題不在勾選，而在**四個 PDM／威脅模型已經定了值、卻沒有任何工作項承接強制的殘項**（token 上限、Workspace 並行上限、預估成本區間、輪數表回寫），以及**一條未達成的 M2 里程碑目標**（Agent 相容篩選維度）。這些之所以能在 18 個 commit 裡一路不被發現，是因為它們掉在工作項的字面範圍之間——**沒有任何一個勾選是錯的，而事情就是沒有人做**。

---

## 11. 附錄：金鑰掃描

依交辦執行兩條檢查，皆無發現：

| 檢查 | 結果 |
| --- | --- |
| `git grep --cached -E "sk-(proj\|ant)-"` | 無 match |
| `git log --all --oneline -- .env` | 無 commit |
| `git ls-files \| grep "^\.env"` | 未追蹤（`.gitignore` 已排除，ADR-019 §4 慣例） |

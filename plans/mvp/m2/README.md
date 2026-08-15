# M2:Skill Lab — 執行計畫

- 日期:2026-08-16
- 狀態:進行中
- 前提:M1 程式碼面收斂(工作項對帳見 [../m1/m1-work-items-audit.md](../m1/m1-work-items-audit.md));M1 驗證閘門的使用者測試與 M2 開發並行,正式通過與否以測試結果為準(材料:[../m1/gate-test/](../m1/gate-test/))。

## 範圍(對應 03-work-items)

| 工作群 | 項目 | 里程碑內順序 |
| --- | --- | --- |
| Run 契約與狀態機 | RUN-001~004(Provider-neutral 契約、Capability、run_id 映射、狀態機) | 第一批 |
| Test Case 與執行設定 | TEST-001/002/003/004/008/009/010(005/006/007 後 MVP) | 第一批起 |
| Trace Schema | TRACE-001(事件 schema 先行,收集屬後續批) | 第一批 |
| Run 排程與韌性 | RUN-005~009(排程、取消逾時、冪等清理、重啟恢復、契約測試) | 第二批 **✅ 2026-08-16 完成**(含 Outbox publisher;RUN-004 一併結案) |
| SelfHostedProvider | SBX-001~010(gVisor 基線) | 第二~三批 |
| Trace 收集與 O11y | TRACE-002~008、O11Y-001~003 | 第三批 |
| 內容基準試跑 | CONTENT-007/008(自 M1 移入) | Sandbox 就緒後 |
| 安全驗收 | SEC-002 六項門檻定值(Q18)、SEC-009 逃逸測試——ADR-015 的實作期驗收關卡 | 部署驗證批 |

## 架構鐵律在 M2 的落點(開工前必讀)

1. Run 狀態唯一事實來源=Go 擁有的 Postgres 狀態機(鐵律 5);LangGraph 只是 Job 內編排。
2. 佇列消費者只有 Go Worker(River);Python 由 Go 以內部 HTTP 呼叫,含逾時與取消傳遞(鐵律 7)。
3. 執行平面不碰核心 DB(鐵律 2)——`services/sandbox` 為獨立 Go module(ADR-019 預留),只透過任務契約、短效物件授權與事件互動。
4. 領域狀態變更與對外事件同交易(Transactional Outbox,鐵律 9);清理冪等。
5. `run_id` 平台永久、`provider_run_id` 臨時(鐵律 10;runs 表 0004 已落)。
6. 每 Run 短效 LiteLLM Virtual Key,`max_budget`+`tpm_limit` 雙限(PDM-003 v5)。
7. Test Case 快照不可變(鐵律 4;`test_case_snapshots` 0004 已落)。

## 開發環境限制(誠實記錄)

- 本機 Windows 無法跑 gVisor(runsc 需 Linux)。開發採 SandboxProvider 介面 + DockerProvider(dev 實作);gVisor 配置為生產 provider,其隔離驗證(SEC-009、SBX-010)屬部署期驗收,ADR-015 定案紀錄已明載「未通過不得開放外部使用者」。

## 第二批交付摘要(2026-08-16)

Run 排程與韌性落地於 `services/platform/internal/run`(排程、驅動、清理、supervisor)與 `internal/outbox`(Transactional Outbox publisher),migration `0018_run_scheduling.sql`。

- **Provider 契約 client**:`contracts/openapi/sandbox-provider.yaml` 於 37f1918 凍結,本批只實作 client 端,未改契約。
- **Outbox publisher**:River periodic job(5 秒),MVP 的發布目的地是 process log——先送再標記,at-least-once,`published_at IS NULL` 讓標記本身冪等;換成真實 transport 只需替換一個函式。
- **契約缺口(記錄,不改 spec)**:
  1. `ProviderRun` 未回傳 `workspace_id`,孤兒掃描只能靠 `run_attempt_id` 反查(已加 `GetRunAttemptForReconcile`,是唯一不帶 workspace scope 的查詢,理由寫在 query 註解)。
  2. 契約說 `provider_run_id`「只在單一 Provider 內唯一」,未規範跨重啟不可重用;`run_attempts` 的 `(provider, provider_run_id)` 唯一索引要求的是後者。Provider 重啟後回收 handle 會撞索引——實作 SelfHostedProvider 時須用不可重用的 handle(UUID 級)。
  3. `POST /runs` 的 422 以 `RunError` 回覆、其餘錯誤以 `Error` 回覆,client 需同時解析兩種 body。
- **留給第三批(TRACE-002 收集)的接點**:`RunRequest.trace` 目前送 `{level: "standard"}` 且 `ingestion_url` 留空(沒有收集端就不發事件);填入 URL 即可讓 provider 開始送。事件排序與去重的既有素材是 `outbox_events`(`event_id` 去重)與 `run_status_transitions`(順序事實)。
- **等 SBX-005/006 的接點**:`RunRequest` 的 `object_grants` 與 `model_gateway` 尚未填,`policy_snapshot.egress.allow` 因此維持空清單(default-deny 且無出口),不是佔位 URL。

## 待負責人(承 M1)

- 宣告閘門 D 日;追認 KPI3 判準修正;Q15(匯入 SSRF 歸屬)裁定;anthropic-sa 法務判定。

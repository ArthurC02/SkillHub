# M2:Skill Lab — 執行計畫

- 日期:2026-08-16
- 狀態:**已完結**(工作項對帳見 [m2-work-items-audit.md](m2-work-items-audit.md),基準 commit `512add1`;殘項三類清單見本文件末節)
- 前提:M1 程式碼面收斂(工作項對帳見 [../m1/m1-work-items-audit.md](../m1/m1-work-items-audit.md));M1 驗證閘門的使用者測試與 M2 開發並行,正式通過與否以測試結果為準(材料:[../m1/gate-test/](../m1/gate-test/))。

## 完結摘要(2026-08-16)

M2 範圍 **41 個工作項**,**33 項維持勾選、1 項對帳退回、7 項維持不勾**(全部為誠實記錄的部分完成或部署期項目),**0 項新勾**。18 個 commit(`3bfd8db..512add1`)、5 份 migration(0016～0020)、2 個新 Go 服務邊界(`services/sandbox` 獨立 module、`internal/trace`)。

**M2 里程碑目標(`01` §M2)的達成狀況**:

| `01` 的 M2 目標 | 狀況 |
| --- | --- |
| 完成 Cloud Sandbox、Dataset、Prompt 與 Run Trace | ✅ 後端完成並經 45 個真實 Skill 端到端驗證;**Test Lab 只有上傳與 preflight 兩個畫面**(2026-08-16 補 `/lab/datasets`;建立 Test Case、編輯 Prompt 與驗收條件仍無 UI,見殘項乙-7) |
| 完成權限確認、逾時、取消及清理 | ✅ TEST-008/009、RUN-006/007、SBX-009;73/73 Run 全部 `cleaned` |
| 完成精選 Skill 的範例資料、Prompt、驗收條件與基準試跑 | ✅ CONTENT-008(15/15 符合);**CONTENT-007 差 writing rubric 一項**(見殘項乙-5) |
| **依 Sandbox 實測結果啟用搜尋的 Agent 相容篩選維度** | ~~❌ **未達成**~~ → ✅ **2026-08-16 補齊**(M2 完結後回填):四個設計問題已裁定,migration `0022` 建表、45 筆實測值回填、搜尋與詳情讀真值、`?agent=` 篩選與 `apps/web` 控制項上線。見殘項乙-4 |

**單一最重要的發現**:PDM-005 的 300K/60K token 硬上限被寫進 `policy_snapshot`、**顯示在執行前權限摘要上並要求使用者確認**,而平台沒有任何一行程式會因為超過它而停止一個 Run。它不違反任何一條已勾項的字面允收準則(`02:RUN-003` 與 SBX-006 的清單裡都沒有 token),因此在 18 個 commit 裡一路沒被發現——**它是孤兒殘項**。詳見對帳 §9.1。

## 範圍(對應 03-work-items)

| 工作群 | 項目 | 里程碑內順序 |
| --- | --- | --- |
| Run 契約與狀態機 | RUN-001~004(Provider-neutral 契約、Capability、run_id 映射、狀態機) | 第一批 **✅ 完成**(RUN-004 由第二批一併結案) |
| Test Case 與執行設定 | TEST-001/002/003/004/008/009/010/011(005/006/007 後 MVP) | 第一批起 **六項完成**(TEST-002/008/009 於權限摘要批);**TEST-004 對帳退回後於 2026-08-16 補上顯示面重新勾選**;**TEST-011(預估成本區間)為 2026-08-16 新增並完成** |
| Trace Schema | TRACE-001(事件 schema 先行,收集屬後續批) | 第一批 **✅ 完成**(由第三批勾選,`0019` 補齊四個欄位缺口後才成立) |
| Run 排程與韌性 | RUN-005~009(排程、取消逾時、冪等清理、重啟恢復、契約測試) | 第二批 **✅ 2026-08-16 完成**(含 Outbox publisher;RUN-004 一併結案) |
| SelfHostedProvider | SBX-001~010(gVisor 基線) | 第二~四批 **六項完成**(001/003/004/006/008/009);**002/005/007/010 維持不勾**——門檻待定值＋生產網路與逃逸測試屬部署期 |
| Trace 收集與 O11y | TRACE-002~008、O11Y-001~003 | 第三批 **✅ 2026-08-16 完成**（TRACE-001 一併勾選；TRACE-004 的成本欄位由第四批補上並勾選） |
| 模型閘道出口與短效授權 | SBX-007（dev 網路面）、SBX-008、TRACE-004 成本 | 第四批 **2026-08-16**（SBX-008 完成、TRACE-004 勾選；SBX-007 仍不勾——Proxy 本體屬部署期） |
| 內容基準試跑 | CONTENT-007/008(自 M1 移入) | 第五批 **✅ 2026-08-16**(CONTENT-008 完成、精選 15/15 符合;CONTENT-007 部分完成不勾——writing rubric 缺消費端。**同日補跑＋全量重測**:預算保留根因修復後 9 筆於 `2026.08-2` 重測(§12),負責人裁定後其餘 36 筆亦重測(§13)——33 筆 `transpiled` 全數轉 `native`、成本 −28%、輸入 token −50%,全 45 筆最新量測符合 42/45,相容軸 45 列入庫。見 [content-baseline-report.md](content-baseline-report.md)) |
| 安全驗收 | SEC-002 六項門檻定值(Q18)、SEC-009 逃逸測試——ADR-015 的實作期驗收關卡 | 部署驗證批。**門檻定值與 Q1～Q3 已於 2026-08-16 由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 定案**;SEC-002／SEC-009 仍不勾——**閘門 B 的四項阻擋已於 2026-08-16 全數落地(乙-8)**,剩下的唯一原因是 45 項基線未經 SEC-009 驗證 |

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

## 第三批交付摘要(2026-08-16)

Trace 收集管線與平台可觀測性。migration `0019_trace_ingestion.sql`,新套件 `services/platform/internal/trace` 與 `internal/platform/metrics`,`services/sandbox` 的事件收集,`infra/observability/` 的告警規則。

### 事件流向(為什麼是這樣)

沙箱容器跑在 `--network none` 上,它**不能自己送事件**。所以:workload 在容器內把事件寫成 JSONL(`/out/trace/events.jsonl`,一行一個 JSON) → sandboxd 讀出來 → sandboxd POST 到平台的 ingestion endpoint。執行平面全程只講 HTTP,不碰核心資料庫(鐵律 2)。

sandboxd 讀檔的方式是**在容器內執行 `/bin/cat`**,不是 `docker cp`。這不是偏好:`/out` 是 tmpfs,而 Docker 的 copy API 對照容器的 rootfs 層解析路徑,對任何掛載點下的檔案一律回「找不到」——已對 daemon 實測確認,不是推測。bind mount 會把主機路徑放進沙箱(基線 C-05 明文禁止),走 stdout 會讓 trace 與 workload 自己的輸出混在一起。exec 的代價寫在 `dockerdrv.ReadTrace` 的註解裡:唯讀 rootfs 上的映像檔自帶二進位、絕對路徑、無 shell、無呼叫端參數、與 workload 同一個非特權 uid、讀取有上限。

收集是**邊跑邊送**(2 秒 ticker)而非結束後一次送:NFR-004 要求事件產生後 3 秒內出現在畫面。容器結束後還有一次收尾推送(含 4 次重試),因為那時容器還在(`DELETE` 才會移除它),而那批正是失敗 Run 最需要的尾巴。

### Ingestion 認證(未動契約)

`contracts/openapi/sandbox-provider.yaml` 只給 `TracePolicy.ingestion_url` 一個欄位,沒有 token 欄位,契約本批凍結。因此憑證**嵌在 URL 內**:`{base}/internal/trace/{run_id}.{attempt}.{exp}.{hmac}`,由平台以 `SKILLHUB_TRACE_INGEST_SECRET` 簽發,每個 attempt 一枚、短效、只能對那一個 (run, attempt) 追加事件,讀不到任何東西。workspace 一律由平台從 `run_id` 自己查(鐵律 3),不問 token。

代價講明:URL 對 workload 可見(它是自己的環境變數)且可能進 access log。兩者都被「短效＋單一 attempt 範圍」界住,而且**該 token 本身是遮罩器的已知秘密值**——workload 把自己的 `SKILLHUB_TRACE_URL` 印進事件裡,入庫前會被遮掉。

未設密鑰時不簽發任何 URL,provider 收到空的 ingestion_url 就不產事件——這是「沒設定」的誠實狀態,好過一個誰都能 POST 的端點。

### 冪等、排序、缺失(TRACE-008)

- **去重**:producer 產生的 `event_id` 是唯一冪等鍵,`ON CONFLICT (event_id, occurred_at) DO NOTHING`。重送回報為 `duplicate`,不是錯誤——at-least-once 下重送是正常行為。
- **排序**:串流內用 `seq`(per `(run_id, attempt, emitted_by)`,無洞、從 1 起),跨串流才用 `(occurred_at, emitted_by, seq)` 合併。
- **缺失**:因為 `seq` 無洞是契約保證,「1..max 減去收到的」就**是**遺失集合,不需要啟發式判斷或逾時等待。進階模式逐串流列出缺號,`complete: false` 時 UI 明示可能不完整。
- **遲到**:終態後照收並標 `late`。

### 遮罩(TRACE-005)

入庫**前**遮罩,`0019` 的 `CHECK (masked)` 讓跳過遮罩在資料庫層不可能。走訪 payload 內每一個字串(比 README §6 要求的「只掃 secret_bearing 欄位」更嚴,且少一份會走樣的清單);已知值優先於 pattern;替換為字面量 `[REDACTED]`,不保留長度。

### O11y

Prometheus 文字格式,platform(獨立 listener `METRICS_ADDR`,不掛對外 port)與 sandboxd(與其他路由同一個 bearer)各自曝露。指標清單、標籤規約與告警規則見 [infra/observability/README.md](../../../infra/observability/README.md)。

**未做且明說**:Alertmanager 部署、通知路由、Grafana dashboard 屬部署期;告警門檻值是首發預設,不是實測校準值。

### 契約缺口(記錄,未改 spec)

1. **`TracePolicy` 沒有 token 欄位。** 本批以 URL 內嵌簽章繞過,可行且安全,但代價是憑證會出現在 URL(見上)。若日後要把它移出 URL,需在 sandbox-provider 契約新增 `TracePolicy.ingestion_token`,屬 additive 變更。
2. **`RunResult` 不含 trace 收集的完成度。** 平台無法從 provider 回報得知「這個 attempt 的事件全推完了沒」,只能靠 `seq` 斷號事後判斷。目前夠用(斷號本來就是設計中的偵測手段),但若要在 UI 上區分「還在推」與「推丟了」,需要契約層的收尾訊號。
3. **`Features.event_streaming` 語意未定義。** 契約只給了一個 boolean 沒有說明。本批的解讀是「給我 ingestion_url 我就會推事件」,並依是否配置 sink 誠實宣告。

### 留給後續(EVAL 批)的接點

- **讀取面**:`trace.Service.Advanced` 已回傳依序重建、標明缺失的完整事件流,`General` 已是聚合後的摘要。EVAL-001 的判定要引用證據時直接用這兩者,不必自行查 `trace_events`。
- **`skill_activation` 的 `skipped`**:SDK 訊息流看不到「可用但沒被叫」,本批不臆造。要判斷「Skill 沒被啟用」屬 EVAL-002 的範圍,材料是「Run 掛了哪些 Skill」對照「trace 裡出現了哪些 activation」。
- **成本**:~~`usage.cost_usd` 目前恆為 `null`(閘道未接)~~ → **第四批已接**:harness 讀閘道的 per-key spend 填入,`cost_source: gateway`,實測與閘道 spend log 逐分錢一致(見第四批交付摘要)。EVAL-012 的成本比較有數據可比了。
- **orchestrator 事件流**:控制平面已會在 `failed`／`timed_out` 轉移的同一交易內寫 `error` 事件,seq 由 `NextTraceSeq` 在交易內配號。EVAL 若要加自己的事件(例如評估開始／結束),沿用 `trace.RecordOrchestratorEvent` 即可,不要另開寫入路徑。

## 第四批交付摘要(2026-08-16)

模型閘道出口路徑(SBX-007 的 dev 實作)、短效授權簽發與 Artifact 收集(SBX-008),以及**第一次真實的端到端 Run**——真 Skill 套件、真沙箱、真 Agent SDK、真模型呼叫,成本進 Trace(TRACE-004 結案)。無 migration。

### 網路拓撲(SBX-007)

```
[worker/api]──┐                     skillhub_default
[sandboxd]────┼── postgres ── seaweedfs ── litellm ─┐
              │                                     │ skillhub_egress (internal: true)
              └── (docker daemon) ── sandbox 容器 ───┘
```

- `skillhub_egress` 是 `internal: true` 的 Docker network:上面的容器**沒有對外路由**,而網路上只有 LiteLLM 閘道。沙箱被放進去,閘道就是它唯一到得了的位址——「允許清單只有一項」在開發機上能真正被強制的形式。
- 沙箱**只在 `RunRequest.egress.allow` 含 `model_gateway` 時**才接上該網路;`allow` 為空就維持 `--network none`。方向只有一個:沒有出口路由的節點只宣告 `egress_modes: ["none"]`,排程器不會派需要出口的工作給它(RUN-005),也永遠不會用較弱的模式頂替較強的。
- **物件儲存不在這張網路上**,這是刻意的:dev 的 SeaweedFS 若讓沙箱直接連得到,預簽 URL 就形同虛設(沙箱能讀整個 bucket)。因此物件位元組由 sandboxd 代搬(見下)。
- **仍未完成(SBX-007 不勾)**:生產級 Egress Proxy 本體、域名允許清單、DNS 固定解析、目的地記錄(N-01~N-07)全屬部署期;允許清單管理流程仍是 ADR-015 待決策。本批做的是 dev 節點上的網路面隔離,不是 Proxy。

### Grant 流向(SBX-008)

```
dispatch:  worker ──presign(GET pkg / GET dataset / PUT artifacts)──> RunRequest.object_grants
           worker ──POST /key/generate(alias=attempt id, max_budget, tpm_limit, models)──> RunRequest.model_gateway
沙箱輸入:  sandboxd 用 grant 下載 → docker exec /bin/tee 寫進容器 → 最後寫 ready 標記
沙箱輸出:  sandboxd docker exec /bin/tar 讀出 /out/artifacts → 過濾上限 → PUT 到 artifact grant
cleanup:   worker ──POST /key/delete(key_aliases)──> 撤銷(冪等)
```

- 沙箱**再也拿不到任何預簽 URL**:它只有閘道憑證,而閘道憑證是它唯一該用的。`docker exec` 是被迫的——`docker cp` 對唯讀 rootfs 的容器一律被 daemon 拒絕(實測:`container rootfs is marked read-only`),bind mount 違反基線 C-05。
- **簽發失敗＝不派送**(fail-closed):拿不到套件授權或鑄不出 Virtual Key,Run 直接以平台錯誤結束,不會送出一個註定在沙箱裡失敗的工作。
- **撤銷以 alias 定址**(`skillhub-attempt-<run_attempt_id>`),不存金鑰本身:重啟後也撤得掉,而且金鑰只存在閘道(鐵律 8、SEC-005)。撤銷失敗記為 `cleanup_status='failed'` 並重試,不當作已撤銷。
- **收集交接**:`/out` 是 tmpfs,workload 的行程一結束,核心就把裡面的東西丟掉,而 `docker exec` 需要**執行中**的容器——所以沒有任何「事後讀取」存在。workload 因此寫 `/out/.workload-done` 後**等待**,sandboxd 排完 trace、收完 artifact 才寫 `/out/.collected` 放它走。這正是 PDM-005 說的「合作式停止窗口」。**限制**:崩潰、被殺、逾時的 workload 不會走這條路,只剩 2 秒 ticker 已推出去的部分。
- **一個 attempt 一個封存**:預簽是逐物件的,而平台在 workload 產出前不可能知道檔名,所以一張寫入授權只能授權一個 key。Manifest 逐檔列出名稱、大小、雜湊;位元組在 `run-artifacts/<run_id>/<attempt_id>/artifacts.tar` 裡。逐檔獨立物件需要 POST policy 前綴授權,留給 PACK-001。

### 成本(TRACE-004 結案)

harness 用**自己的 Virtual Key 讀閘道的 `/key/info`** 取本 Run 的 spend——金鑰只讀得到自己,不需要 admin 憑證也不外洩他人。**刻意不採用 SDK 的 `total_cost_usd`**:那是 SDK 用本地價目表對一個由閘道解析的模型名估的,掛上 `cost_source: gateway` 會是假的。

閘道的 spend 是**非同步 flush** 的,所以 harness 先等 2.5 秒再讀,並讀到數字不再變動為止。實測對帳(同一次 Run):

| 來源 | 數字 |
| --- | --- |
| Trace `usage.cost_usd` | `0.00571125` |
| 閘道 `LiteLLM_SpendLogs` 四次呼叫合計 | `0.00036975 + 0.00211590 + 0.00181905 + 0.00140655 = 0.00571125` |

一致。**仍是讀數不是帳本**:最後一次 flush 若落在最後一次輪詢之後就會少算,權威對帳來源是閘道的 per-key spend(ADR-017)。

### 端到端證據(本批驗收核心)

`services/platform/internal/identity/e2e_gateway_integration_test.go`,以 `SKILLHUB_E2E_SANDBOX_URL` 開關(CI 不跑,因為會花錢)。一次通過的實跑:

- 路徑:真 Skill 套件進 SeaweedFS → preflight 摘要 → 確認 → `POST /skills/{id}/runs` → 排程選中 sandboxd → 派送(含 grants 與 Virtual Key)→ 沙箱容器(Node 22 + Agent SDK 0.3.233)接上 `skillhub_egress` → 模型呼叫經 Virtual Key → trace 推回平台 → artifact 收集上傳 → cleanup 撤銷金鑰。
- Trace 六個事件依序入庫(遮罩後):`skill_activation`(run-marker)→ `tool_call`(Skill)→ `tool_call`(Write `/out/artifacts/marker.txt`)→ `agent_output` intermediate/final(`DONE`)→ `usage`(`input_tokens: 49518`、`output_tokens: 226`、`cost_usd`、`cost_source: gateway`)。
- Artifact:`run-artifacts/<run_id>/<attempt_id>/artifacts.tar` 內含 `marker.txt` = `SKILLHUB-E2E-OK`。
- 金鑰撤銷:cleanup 後 `LiteLLM_VerificationToken` 內該 alias 為 0 列;以同法鑄一枚金鑰實測,撤銷前 `/key/info` 回 200,撤銷後 `/key/info` 與 `/v1/messages` 皆回 **401**。
- 花費:單次 Run 約 **$0.006–0.017**(mini 級,四次呼叫);本批含除錯在內的閘道總花費約 **$0.24**。

### 實測推翻的既有假設(重要)

**Agent SDK 0.3.233 的 Skill 載入條件與 PDM-003 Spike(claude-agent-sdk 0.2.137)相反。** 實測:`settingSources: ["project"]` 在 0.3.233 **完全發現不到**專案 skill(`init.skills` 只有內建 plugin);**省略 `settingSources`** 才發現得到。且 0.3.x 把 skill 啟用從 `allowedTools` 移到獨立的 `skills` 選項(`allowedTools` 傳 `'Skill'` 已標為 deprecated)。`run.mjs` 已改為「省略 `settingSources` ＋ `skills: "all"`」,並在檔頭寫明每一條都是**靜默失效**,改動必須重新實測而不是推理。`HOME` 指向每個 Run 專屬的 `/work` tmpfs,所以省略 `settingSources` 不會讓映像裡的 `~/.claude` 滲進來。

### 契約缺口(記錄,未改 spec)

1. **`PackageRef` 沒有 skill 名稱。** Agent SDK 以「一個目錄一個 skill」發現,所以套件必須解到 `<skills>/<name>/`,而契約只給 id 與 hash。目前由沙箱自己讀套件內 `SKILL.md` 的 frontmatter `name`——讀不受信任內容的最低權限位置本來就是沙箱,可接受;若要移出沙箱需 additive 加欄位。
2. **`ObjectGrant` 是逐物件的,沒有前綴授權。** 因此 artifact 只能以單一封存上傳(見上)。逐檔物件需要契約層允許 POST policy 形式的授權。
3. **`RunResult.usage` 仍只有 wall clock。** token 與成本走 Trace,沒有回填到 provider 的 result;PDM-005 5.2a 的「Go worker 累加 input_tokens 當硬上限」因此尚未接上(需要 provider 側回報或平台讀 trace)。

### 留給 CONTENT-007/008 的接點:跑一個 Skill 的最短路徑

前置(各一次):

```bash
# 1) 閘道(.env 需有 OPENAI_API_KEY 與 LITELLM_MASTER_KEY;金鑰只進這個容器)
docker compose --env-file ../../.env -f infra/compose/docker-compose.yml up -d litellm
# 2) Runtime Image
docker build -t skillhub/runtime-agent-sdk:2026.08-2 infra/images/runtime-agent-sdk   # 本批當時是 2026.08-1(無 python3);tag 隨映像內容變更(乙-6)
# 3) sandboxd(掛 docker.sock,沙箱容器會被放到 skillhub_egress)
docker run -d --name sandboxd --network skillhub_default --network-alias sandboxd \
  -v "$PWD:/src" -v /var/run/docker.sock:/var/run/docker.sock -w /src/services/sandbox \
  -e SKILLHUB_SANDBOX_TOKEN=... -e SKILLHUB_SANDBOX_NETWORK=skillhub_egress \
  -e SKILLHUB_SANDBOX_IMAGE=skillhub/runtime-agent-sdk:2026.08-2 \
  golang:1.25 go run ./cmd/sandboxd
```

每次跑 Skill:worker 需要這組環境變數(缺任一項就退回「沒有閘道」的誠實狀態,不會偷偷估算):

| 變數 | 值 | 作用 |
| --- | --- | --- |
| `SKILLHUB_SANDBOX_PROVIDERS` / `SKILLHUB_SANDBOX_TOKEN_<NAME>` | `self_hosted=http://sandboxd:9000` | Provider 註冊表 |
| `OBJSTORE_*` | SeaweedFS(**須有金鑰**,見下) | 預簽 grant |
| `SKILLHUB_MODEL_GATEWAY_URL` | `http://litellm:4000` | 沙箱看得到的閘道位址,同時進 egress allow 清單與權限摘要 |
| `SKILLHUB_MODEL_GATEWAY_KEY` | master key | 只用來鑄／撤金鑰,不做推理 |
| `SKILLHUB_RUN_MODEL` | `gpt-5.4-mini` | 試跑預設層(PDM-003 v5) |
| `SKILLHUB_RUN_MAX_BUDGET_USD` / `SKILLHUB_RUN_TPM_LIMIT` | 預設 `0.50` / `200000` | 每 Run 雙限 |
| `SKILLHUB_TRACE_INGEST_SECRET` / `_URL` | — | 沒設就不收 trace,也就沒有成本數字 |

**api 程序也要其中兩組**(2026-08-16 第五批實測補上,原表只列了 worker):`defaultPolicy()` 在 **API 程序**裡讀 `SKILLHUB_MODEL_GATEWAY_URL`/`_KEY` 組出 `policy_snapshot.egress.allow`——api 沒有它,允許清單就是空的,沙箱被派到 `--network none`,Agent SDK 對閘道空等到 `Request timed out`(實測 188 秒)。同理 api 需要 `SKILLHUB_SANDBOX_PROVIDERS` 與對應 token,否則 Preflight 的 Provider 摘要只寫 `unassigned`。

程式路徑就是既有的:`GET /skills/{id}/runs/preflight` → `POST /skills/{id}/runs/preflight/confirm` → `POST /skills/{id}/runs`(帶 `confirmed_summary_hash`)→ worker 自動驅動 → `GET /runs/{id}/trace`。整段的可執行樣板見 `e2e_gateway_integration_test.go`,照抄即可批量跑。

**兩個會絆倒批量試跑的點**:

1. **Prompt 必須點名 Skill**(PDM-011 實測自主觸發率為 0),而且**要告訴 Agent 把產物寫到 `/out/artifacts/`**——沒有系統提示會自動說這件事,沒寫到那裡就沒有 artifact 被收。Dataset 掛在 `/work/data/<file_name>`。
2. **SeaweedFS 必須帶 S3 金鑰**。匿名 bucket 沒有可簽章的金鑰,`PresignGet` 會直接失敗、Run 以 fail-closed 結束。`infra/compose/seaweedfs-s3.json` 已加,但**既有 stack 需重建 seaweedfs 容器才會生效**。

## 殘項收斂批:平台限制與成本正確性(2026-08-16)

M2 完結後的補件批,處理殘項清單裡「已定值但沒有強制」與「已量到但沒查根因」那幾項。migration `0021`,無契約變更。

### 閘道預算 50 倍誤差:根因是保留而不是計數(乙-3)

LiteLLM 自 v1.84 起對每個在途請求做**樂觀預算保留**:先把該請求的**理論最大成本**加到金鑰的預算計數器上,請求結束再換成實付。理論最大 ＝ 全部 input token 以**未快取**單價 ＋ `max_tokens` 以 output 單價。

對試跑預設層 `gpt-5.4-mini` 而言,output 單價 `4.5e-6`/token,光 `max_tokens=64000` 就保留 **$0.288**——而實際輸出常是數百 token、input 大半是 1/10 價的 cache read。**保留值與整個 $0.50 的 per-Run 預算同量級**,所以兩個重疊的請求就能把計數器頂到上限,之後每個請求都收到 429。`LiteLLM_SpendLogs` 記的是實付,兩邊自然差 50 倍。這也解釋了基準試跑報告 §6.2 的兩個對照實驗為什麼排除不掉它:**單一序列請求永遠不會觸發**,保留在下一個請求之前就釋放了。

本機實驗(1.96.2,同一把 `max_budget=0.5` 金鑰):

| 情境 | 結果 |
| --- | --- |
| 序列 3 次(`max_tokens=64000`) | 全部 200 |
| 併發 4 次 | 第 3、4 次 **429 `Current cost: 0.78845825`** |
| 同一把金鑰全程實付(`/key/info`) | **$0.00096**(相差 821 倍) |

處置是配置修正:`infra/compose/litellm-config.yaml` 加 `general_settings.disable_budget_reservation: true`——LiteLLM 為這個情境提供的開關,其欄位說明明列「phantom BudgetExceededError caused by leaked reservations」。改後複驗:

| 驗證 | 結果 |
| --- | --- |
| 同一組併發 4 次 | **無任何預算 429**(第 4 次撞到 `tpm_limit`,那是另一個煞車,行為正確) |
| 反證強制仍在:`max_budget=1e-06` | 第 2 次起 429,且 `Current cost: 0.00048` **等於該金鑰實付** |

**PDM-003 v5 的 $0.50 不改**:錯的是語意不是數值,修好後 $0.50 就是 $0.50(基準試跑中位數 $0.0566、最大 $0.2367,餘裕 2 倍以上)。代價寫在配置註解裡:改為讀取時強制,已在途的請求會跑完,超額上界是它們的實付金額;沙箱只跑一個 agent、spend flush 為 1 秒、`tpm_limit` 仍在,超額有界且遠小於它取代的誤擋。

**仍未做**:9 個已索引 Skill 的基準補跑(`copyright-creative-work`、`document-format-skills`、`docx`、`excel-delete`、`excel-sort`、`excel-split`、`json-restructure`、`pptx`、`xlsx`,預估 $1.0–1.5)。阻擋它的原因已經沒有了。

### SEC-002 閘門 B 的四項阻擋全數落地(乙-8)

`internal/run/gateb.go`,兩項新檢查都在 `Create` 內、建立 run 之前,超出即 422:

- **靜態掃描等級**:不等威脅模型 Q7。政策其實已經以逐項可判定的形式存在——`skillpkg` 給每個 finding code 指定了 severity,`error` 級就是 `SKILL-002` 定義的阻擋級。閘門 B 重新掃描該版本的套件位元組並套用同一套政策。**不讀 `search_documents.scan`**:那是目錄 UI 的可為 NULL 的警告計數投影,讓閘門讀它等於讀一份決策的快取而不是決策。掃不成(讀不到、解不開、沒有物件儲存)即拒——順帶補上一個既有的洞:先前套件不可讀時 preflight 只顯示 `unavailable`,Run 照跑。
- **Workspace 並行上限 ＝ 2**(PDM-005 §5.2):計非終態 Run,workspace 取自 session(鐵律 3)。在交易內先取 `pg_advisory_xact_lock(workspace)`,否則兩個同時進來的請求會各讀到「目前 1 筆」而都放行。

**SEC-002 仍不勾**:剩下的唯一原因是 45 項基線未經 SEC-009 驗證。

### 權限摘要的預估成本區間(乙-9 / TEST-011)

`PermissionSummary.EstimatedCost`:$0.01 / 常見 $0.06 / $0.30,`basis` 欄把來源(基準試跑 45 筆的閘道實付分布)與「估計值非報價」寫在畫面上。**寫死一組保守區間**而不做動態統計:每個 Skill 的實時百分位要等 EVAL-012 才有足夠樣本,在那之前一個標明來源的估計值好過一個用太少資料算出來、卻長得像統計的數字。

**不進 `summary_hash`**,理由與 User Prompt 不入 hash 同一條:hash 涵蓋的是「這個 Run 能碰什麼」,成本是預測不是權限;拿更大的樣本重新校準,不該把使用者手上每一份確認一起作廢。`TestCostEstimateIsOutsideTheConfirmedHash` 直接對回應位元組重算 sha256 反證。

### TEST-004 的顯示面(乙-7 的一半)

`apps/web` 新增 `/lab/datasets`(`src/pages/DatasetUpload.tsx`):大小限制、支援格式、保存政策、資料使用範圍四項**在檔案選擇控制項之前**渲染,數字全部來自 `GET /test-cases/limits`,UI 內不另寫一份。讀不到規則時**不提供上傳控制項**——規則沒顯示,「上傳前顯示」就沒發生。TEST-004 依允收重新勾選;**TEST-001／002／003 是否同尺退回仍待裁定**(乙-7)。

### SBX-012:in-flight orphan 表與分項計數器

migration `0021_reconciler_orphan_sightings`。每輪掃描 upsert 本輪判定為洩漏的 `(provider, provider_run_id)` 並遞增 `rounds`,輪末刪除本輪沒看到的列——**這個刪除就是「連續」的定義**,重新出現從第 1 輪起算。用資料表不用行程內 map,是因為 Reconciler 是 River 的 leader-only periodic job:leader 會換行程、行程會重啟,而「洩漏活得比 worker 久」正是這個門檻要抓的情況。

sighting 記在 destroy **之前**,而且 destroy 成功不會抹掉它:X-03 問的是「同一筆有沒有撐過兩輪掃描」,撐過了就是撐過了,哪一輪殺掉的不影響這個事實。清掉它的是下一輪看不到它。

另修一個既有行為:`scanProvider` 原本一筆 destroy 失敗就 `return err` 中止整輪,於是第二筆洩漏連被看見都沒有——正好是這個門檻存在的情境下最容易漏的一筆。現在逐筆記錄、掃完再合併錯誤回傳。

三個新指標(`skillhub_orphan_sandbox_persistent{provider}` gauge、`skillhub_gateway_revoke_failed_total`、`skillhub_sandbox_destroy_failed_total{provider}`)。金鑰與沙箱分開計,因為**正確動作相反**:沙箱殺不掉要 drain 節點,金鑰撤不掉 drain 一點用都沒有。`alerts.yml` 的兩條過渡規則已升為正式形式並移除過渡註解,`promtool check rules` 通過(18 條)。

`CleanupBacklogGrowing` 順帶校準:`> 5` → `> 2`。舊值大於封測整池的 4 個 slot,意思是整池沙箱全部洩漏都還低於門檻——一條在容量耗盡前不可能響的告警等於沒有。新值取 4 slot 的 50%,與 ADR-022 X-04 單節點 drain 同一個比例;`for: 15m` 不變,所以正常清理的暫態積壓仍不觸發。**校準義務不變**:池容量改變時要重推。

### 契約缺口(記錄,未改 spec)

`contracts/openapi/public.yaml` 的 `RunPermissionSummary` 尚未宣告 `estimated_cost`。屬 additive(該 schema 沒有 `additionalProperties: false`,現有 client 不受影響),但鐵律 12 要求 schema 先行,**待契約批補上**。

---

# M2 殘項清單(三類)

分類原則:**甲**只能在真實部署環境驗收,本機結構性做不到;**乙**缺的是一個決策不是一段程式,寫程式之前得先有人拍板;**丙**是 M3(EVAL)接手的接點,不是缺口。逐項證據見 [m2-work-items-audit.md](m2-work-items-audit.md)。

## 甲、部署期驗收(**依 ADR-015:未通過不得開放外部使用者提交 Skill 執行**)

本機 Windows 無法跑 gVisor(runsc 需 Linux),巢狀虛擬化亦為 ADR-019 待決策 3。以下四項不是被延後,是**在這台機器上結構性不可能完成**。

| # | 項目 | 內容 | 解除條件 |
| --- | --- | --- | --- |
| 甲-1 | **SEC-009** | 逃逸測試、資源耗盡測試、Runtime 相容性測試、網路外洩測試(DNS tunneling／內網掃描／Metadata Service)、憑證範圍測試、清理失敗測試、設定與供應鏈稽核——覆蓋基線 45 項全部。`02:SEC-009` 明文「M2 的 SelfHostedProvider 驗收必須全數通過」,**M2 結束時未達成**。**可執行的測項清單、通過判準與證據要求已由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分給出**(**10 個測項**、45 項覆蓋核對、全數 pass 且 0 unknown 才放行、證據存 **`plans/mvp/m4/sec-009-acceptance/`**——放 M4 不放 M2,因為往已結案的里程碑目錄持續寫入新證據會讓那份對帳失去「某一時點的帳」的意義) | Linux 節點。**⚠️ ADR-022 修正一個前提:gVisor 的 `systrap` 平台不需巢狀虛擬化**(只有 `--platform=kvm` 才需要),因此多數測項在一般 Linux runner 即可跑,ADR-019 待決策 3 的範圍縮小;**此點須由部署批的第一台節點實測確認** |
| 甲-2 | **SBX-010** | 同上的工作項側。現有的真實容器驗證(非 root、唯讀 rootfs、無主機掛載、pids 上限、逾時強停、清理冪等)**不等於**逃逸測試通過 | 同甲-1 |
| 甲-3 | **SBX-005／007 的生產網路面** | SBX-005 缺 P-02「Sandbox → 核心資料庫連線嘗試被實際阻擋」的**常駐探針**;SBX-007 缺生產級出口強制、允許清單、DNS 固定解析與目的地記錄(N-01～N-07)。dev 已有的是網路面隔離(`internal: true` 網路上只有閘道),**不是強制層**。**⚠️ ADR-022 另指出 dev 的一個既有缺口**:同一張 `skillhub_egress` 上的 sandbox 目前可互相連通,是**不需逃逸**的跨 Run 橫向路徑;生產形態必須關掉(每 Run netns ＋ `--icc=false`),並列為 SEC-009 測項 T5-4。**另一項生產前提**:ADR-022 Q2 強制條件 6——沙箱層允許清單的目的地**不得是控制平面／資料節點位址**,LiteLLM 須移到沙箱面專屬節點(原 Egress Proxy 預算行承接,總額不變;現行 compose 的 `127.0.0.1:4000` 是 dev 形態,生產不可複製),違反時由測項 T5-7 抓 | 生產網路。**Q3 已於 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 定案**(nftables default-deny ＋固定 DNS,不部署 Squid／Envoy;允許清單存 `infra/egress/allowlist.yaml`,含變更、複審與記錄流程),剩下的是實作不是決策 |
| 甲-4 | **SBX-002 的門檻定值** ~~待定~~ **已定值** | 流水線已接上(digest 斷言→build→syft SBOM→grype→門檻)。**I-06 與 I-04 的提案值已由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分採納定案**(可修的 Critical／High 阻擋且**無豁免路徑**、不可修者具名豁免、複審日 ＝ `first_exempted_at`＋90 天;掃描有效期 **30 天**、到期前 7 天告警),`infra/images/README.md` 與 `runtime-image.yml` 的「暫定／PROVISIONAL」字樣**已改為定案(`606ad87`)**。~~**SBX-002 仍不勾的唯一原因**:I-03 的 SBOM 與 I-04 的 `scanned_at` 現落在 90 天 CI artifact 與人工維護的日期。~~ **SBX-011 已於 2026-08-16 完成**:發佈流水線接上 GHCR,SBOM 與掃描結果(含機器可讀 `scanned_at` 與 `fixable_critical_high`)以 attestation 隨 digest 保存,另有每週 `rescan` job 重掃**已發佈的 digest**。**流水線側四項檢查已全部可自動化判定**;**SBX-002 剩下的唯一未勾原因改為**:閘門 A 的節點准入探針要在真實節點上查得到這兩份 attestation,屬部署批。<br>**同日另一項**:映像升 `2026.08-2`(加 python3,見乙-6),重掃後可修的 Critical／High 仍為 0,新增的不可修項已具名豁免 | ~~SBX-011 完成~~ → 部署批的閘門 A 探針 |

## 乙、待負責人決策(**寫程式之前先要有人拍板**)

| # | 項目 | 待決內容 | 阻擋什麼 |
| --- | --- | --- | --- |
| ~~**乙-1**~~ **已解決(2026-08-16)** | ~~**SEC-002 六項門檻值(威脅模型 Q18)**~~ | → **[ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md)**:六項門檻全部定值(P-03 節點 7 天滾動重建;P-04 gVisor 基準 ＝ 上游 N/N−1 且 ≤ 90 天、逃逸類 CVE 24 h 換版或停用、High 7 天;I-04 掃描有效期 30 天;I-06 可修的 Critical／High 阻擋且無豁免路徑、不可修者具名豁免 90 天複審;X-02 每 5 分鐘;X-03 連續 2 輪告警、X-04 單節點 ≥50% slot drain／全池 ≥25%(下限 2 筆)暫停),Q1～Q3 亦已定案(compose-per-VM／執行平面單租戶且同節點多 Run／nftables default-deny ＋固定 DNS,允許清單存 `infra/egress/allowlist.yaml`)。<br>**但 SEC-002 仍不勾**——見甲-1／甲-2(45 項未經驗證)與乙-8(閘門 B 兩項阻擋未落地);**SBX-002 仍不勾**——I-04 的 `scanned_at` 在 container registry(ADR-019 待決策 1)定案前無法隨 image 保存,見甲-4 | ~~SEC-002 勾選、SBX-002 勾選、甲-3／甲-4~~ 剩餘阻擋見右列說明 |
| **乙-2** | ⚠️ **PDM-005 token 硬上限:二選一** | `300_000`／`60_000` 被寫進 `policy_snapshot`、**顯示在執行前權限摘要上要求使用者確認**,而平台與沙箱**都不強制**(`TokenBudget` 零消費端,無累加器)。實際煞車只有 `max_budget`／`tpm_limit`／牆鐘,而 PDM-005 §5.2a-4 明文說過這三個都不是 token 上限。<br>**(a)** 新增實作工作項「依閘道回報 `input_tokens` 累計並於超限時終止 Run」,並把 §5.2a 的輪數換算表(15／7.7／5 輪)回寫 `02:RUN-003`;或 **(b)** 正式承認 MVP 不做,**把 `TokenBudget` 從權限摘要移除**。<br>**現狀(顯示但不強制)是兩者中最壞的一種**——讓使用者確認一個平台不會執行的上限,直接踩 NFR-001「UI 不得誤導」<br>**2026-08-16:規格面(a)已完成**——§5.2a 的輪數換算表(15／7.7／5 輪)與 §5.2 的上限表已回寫 `02:RUN-003`,並寫明「token 上限的強制點只有一個、`max_budget`／`tpm_limit` 不得代理」與「呈現的前提是它真的會被執行」。<br>**2026-08-16 已解決:取 (a),強制已落地**——見新工作項 **`03` SBX-013**(已完成)。<br>**強制點與 PDM-005 §5.2a 的字面指定不同,理由寫在 `02:RUN-003`**:原本指定 Go Worker 依閘道回報的 `input_tokens` 累計,但 `RunResult.usage` 不回傳 token(契約已誠實記載),從 Trace 事後重建又發生在錢花完之後——**唯一看得到每次回應 token 數的是沙箱內的 harness**,強制點因此在那裡。上限值、語意與三層併用的結論全部不變。<br>機制:逐則訊息累計 → 跨越即 `break` 出 SDK 迴圈(不再有下一次呼叫)→ 發 `error`(`code: token_budget_exceeded`)與終態 `usage` → exit code 9;sandboxd 回 `completed`＋`status: failed` 並附可讀訊息,artifact 與 trace 尾巴照常收集。上限值來自該 Run 凍結的 `policy_snapshot`,與權限摘要同源。<br>**failure_class 裁定:不擴充 `0018` 的 CHECK、不新增 migration**——理由見 `03` SBX-013(retry 語意與 `workload_error` 相同;要讓平台分辨它需先在 `contracts/openapi` 加 `RunError.class` 新值,那是契約批範圍;精確原因已在 trace 的 `error.code` 與終態 `usage`)。<br>**誠實界線**:合作式停止,與軟牆鐘同類;跨越上限的那次回應已經付過錢。 | ~~無工作項承接~~ **已由 SBX-013 承接並完成**;PDM-003 v5「三層併用」的第三層現已成立 |
| ~~**乙-3**~~ **已解決(2026-08-16)** | ~~**閘道 $0.50 預算計數 50 倍誤差的處置**~~ | **根因:LiteLLM 的樂觀預算保留(optimistic budget reservation,v1.84 起預設開啟)。** 每一個在途請求會先把**理論最大成本**扣在金鑰的預算計數器上:全部 input token 以**未快取**單價計、加上 `max_tokens` 以 output 單價計。對 `gpt-5.4-mini`(output `4.5e-6`/token)而言,光是 `max_tokens=64000` 就是 **$0.288**,而實際輸出常只有數百 token、input 多半是 1/10 價的 cache read——保留值本身就跟整個 $0.50 預算同量級,兩個請求重疊就把計數器頂到上限,之後每一個請求都被回絕。`LiteLLM_SpendLogs` 記的是實付,所以兩邊差 50 倍。<br>**實驗複現(本機 1.96.2,同一把 `max_budget=0.5` 金鑰)**:序列 3 次全 200;併發 4 次時第 3、4 次回 **429 `Current cost: 0.78845825`**,而該金鑰全程**實付 $0.00096**(821 倍)。錯誤字樣與基準試跑紀錄同型。<br>**處置:配置修正,不動 $0.50。** `infra/compose/litellm-config.yaml` 加 `general_settings.disable_budget_reservation: true`(LiteLLM 官方為此情境提供的開關,其說明明列「phantom BudgetExceededError caused by leaked reservations」)。改後同一實驗:併發 4 次**無任何預算 429**;另以 `max_budget=1e-06` 反證強制仍在——第 2 次起回 429 且 `Current cost: 0.00048` **等於實付**。<br>**代價已寫在配置註解**:改為讀取時強制,已在途的請求仍會跑完,金鑰可超出上限它們的實付金額;沙箱只跑一個 agent、`proxy_batch_write_at` 為 1 秒、`tpm_limit` 仍在,超額有界且遠小於它取代的誤擋。<br>**PDM-003 v5 的 $0.50 維持不變**:誤差不在數值而在語意,修好之後 $0.50 就是 $0.50(基準試跑實測中位數 $0.0566、最大 $0.2367,仍有 2 倍以上餘裕)。<br>**仍未做**:9 個已索引 Skill 的補跑(預估 $1.0–1.5),見下方註 | ~~乙-2 的三個煞車裡最重要的那個本身不可信~~ 金額煞車已可信;**CONTENT-008 已索引層的補跑仍未執行** |
| ~~**乙-4**~~ **已解決(2026-08-16)** | ~~**Agent 相容軸 schema 的四個待決**~~ | **四問已裁定並落地**(migration `0022_agent_compatibility.sql`,決策理由逐條寫在該檔頂端):<br>①**欄位歸屬 ＝ (Skill Version × Runtime Image)**,採「後者才誠實」的既有判斷。代價明示:**換映像即回到未驗證直到重測**——`2026.08-2` 加了 python3(乙-6),所以部署改用它之後這 45 筆結論不再適用,那正是這個鍵存在的意義。<br>②**兩軸判準**:`capability` 取自 trace(有 `skill_activation` 即 `activated`);`runtime` 取自「套件宣告的執行環境 ⊆ 映像提供的環境」,映像提供什麼由 Dockerfile 說了算,宣告什麼由 `seed-skills.json` 的 `deps_runtime` 說了算(策展判斷,不從套件重推)。<br>③**樣本量 ＝ 1 次基準 Run 即可寫入**,逐列帶 `source_run_id` 可追溯。理由:那一列說的是「一次 Run 觀察到什麼」,而不是「宣告 passed」;要等某個門檻才寫,等於讓目錄繼續顯示未驗證而 45 筆實測擱著不用。重測覆寫同一列並帶走 `source_run_id`。<br>④**值域**:`capability` ＝ activated／not_activated／unverified;`runtime` ＝ native／**transpiled**／failed／unverified。`transpiled` 就是第三態——Run 成功但跑的是模型對腳本的改寫。**`not_activated` 不從沉默推定**(SDK 訊息流看不到「可用但沒被叫」,TRACE-002 已記)。<br>**回填 45/45**:`capability` 全數 `activated`;`runtime` 12 `native`(11 無依賴＋1 node)／33 `transpiled`,與 [content-baseline-report.md §8](content-baseline-report.md) 逐筆相符。回填腳本 `tools/content/backfill-agent-compatibility.sql`(可重跑,capability 由 trace 現查,不寫死)。<br>**讀取面**:`internal/catalog` 的 detail 與 search 改讀真值(原本寫死 `unverified`),回應一併帶 `runtime_image` 與 `measured_at`——**不設「目前映像」的部署開關**,因為那個設定填錯會安靜地讓目錄顯示錯的結論;標明來源映像則不可能安靜地錯。<br>**篩選**:`?agent=` 只開 runtime 一軸(capability 全目錄同值,與「來源層級」同一個理由),四態精確比對;`apps/web` 停用控制項四減為三 | ~~**`01` 的 M2 里程碑第 4 項**、`02:DISC-002` 的 Agent 維度~~ 已解除 |
| **乙-5** | **CONTENT-007 的 writing rubric** | `02` §4.7 第 3 條要求 `writing` 類每個精選附一份可編輯 rubric,供 LLM Judge 逐項回傳證據引文。**未做,因為 rubric 沒有消費端**——EVAL-001／002 的 Judge 介面尚未實作。其餘四條允收皆達成 | CONTENT-007 勾選;實質上是 M3 的前置(見丙-1) |
| ~~**乙-6**~~ **已裁定(2026-08-16):加** | ~~**Runtime Image 要不要含 python3**~~ | **加**,映像升 **`2026.08-2`**。理由:沙箱**沒有網路**(SBX-007),使用者不可能自己裝,預裝是唯一途徑;不加則 33 個 Skill 的腳本永遠不會被執行,平台只能是「模型轉譯」語意。<br>**裝什麼 ＝ 45 個目錄 Skill 宣告的依賴集合,一個不多**(pandas 26／openpyxl 17／lxml 12／python-docx 2／numpy・pypdf・pdfplumber・python-pptx・python-dateutil 各 1)——這條規則可以從資料重新推導,「看起來有用」不行。版本 pin 取**通得過 I-06 的最新版**(保守舊版被 grype 抓到可修的 High,而 ADR-022 對可修項無豁免路徑);`pip` 比照 npm 前例裝完即移除。<br>**成本明示**:①映像 806 MB → **1.24 GB**;②豁免清單多了四列(`python3.11` 系列、`libexpat1`、`libsqlite3-0`、`libncursesw6`,皆無上游修復),其中 `libexpat1`／`libsqlite3-0` **會解析工作負載讀進來的資料,攻擊面確實變大**,是接受的風險不是解決的問題;③`pandas` 釘 3.x 而目錄 Skill 多寫於 2.x 年代,**本批未實測兩者差異**。<br>**連帶**:此映像未經基準試跑,所以乙-4 的 45 筆結論**只對 `2026.08-1` 成立**;部署換用 `2026.08-2` 前應重跑 CONTENT-008 基準(預估 $3–4,見基準報告 §5.2),否則目錄該欄會誠實地回到「未驗證」 | ~~乙-4 的值域設計;CONTENT-005 揭露完整度~~ 已解除;**新增待辦**:換映像後的基準重跑 |
| **乙-7** | **`03` §9 的允收是否含 UI** | `apps/web` 有 `RunPreflight`／`RunTrace`,**2026-08-16 新增 `/lab/datasets` 上傳頁**(`DatasetUpload.tsx`),TEST-004 因此依允收重新勾選——它退回的理由是**單一一條可判定的準則**(`02:TEST-002`「上傳前**顯示**大小限制、保存政策及資料使用範圍」),補上顯示面即成立,不需要先有裁定。**待裁定的仍是 TEST-001／002／003**:它們的準則以「使用者可…」起頭,若同尺解讀亦應退回,而那不是一個顯示面就能補的範圍。**(a)** 承認含 UI,一併退回並新增實作工作項;或 **(b)** 明文把 §9 界定在 API＋契約層,UI 由 `DESIGN-007`(未勾)與一個新工作項承接。任一都要三文件同步。**注意 Test Lab 仍沒有建立 Test Case／編輯 Prompt／列出與刪除檔案的介面**,新頁只做上傳這一步 | TEST-001/002/003 的勾選正確性 |
| ~~**乙-8**~~ **已解決(2026-08-16)** | ~~**SEC-002 閘門 B 的兩項未落地阻擋**~~ | 兩項都已在 `internal/run/gateb.go` 落地,`Create` 內、建立 run 之前,超出即 **422**(與既有兩項同一形式)。<br>**①靜態掃描等級**:不等威脅模型 Q7,因為政策其實已經以**可逐項判定**的形式存在於程式裡——`skillpkg` 對每一個 finding code 都指定了 severity,而 `error` 級正是 `SKILL-002` 定義的阻擋級。閘門 B 現在**重新掃描**該版本的套件位元組並套用同一套政策。為什麼不是重覆匯入的檢查:①套件位元組在匯入後可能改變或讀不到;②舊版本可能早於某個今天才是 error 級的 code;③匯入以外的路徑也會產生版本。**刻意不讀 `search_documents.scan`**——那是目錄 UI 用的可為 NULL 的警告計數投影,不是通行證,讓閘門讀它等於讀一份決策的快取而不是決策本身。<br>**fail-closed**:套件讀不到／解不開＝掃描沒做成,一律拒絕(SEC-002「檢查無法執行視同未通過」、DISC-004「不得自行推定為通過」)。這也補上一個既有的洞:先前套件不可讀時 preflight 只顯示 `unavailable`,Run 照跑。<br>**②Workspace 並行上限**:`MaxConcurrentRunsPerWorkspace = 2`(PDM-005 §5.2),計非終態 Run,workspace 一律取自 session(鐵律 3)。在**交易內**檢查並先取 `pg_advisory_xact_lock(workspace)`——否則兩個同時進來的請求會各自讀到「目前 1 筆」而都放行,上限 2 就變成 3。<br>**測試**:`TestRunIsRefusedWhenThePackageCannotBeScanned`／`TestRunIsRefusedWhenTheStaticScanIsBlocking`／`TestWorkspaceConcurrencyLimitBlocksTheThirdRun`(含「另一個 Workspace 不受影響」與「結束一個就放行」)。<br>**SEC-002 仍不勾**:閘門 B 四項阻擋現已全數落地,但 45 項基線尚未經 SEC-009 驗證(甲-1／甲-2) | ~~SEC-002~~ 僅剩 45 項驗證 |
| **乙-9** | **PDM-005 §5.3 未回寫 `02:TEST-005`** | §5.3 指定的權限摘要欄位裡,**「預估成本區間」完全不存在**於 `PermissionSummaryContent`。§5.2a-6 特別強調必須是**區間**不是單值(首次與後續 Run 因 prompt caching 差約 8 倍)。根因是 PDM 指定要回寫 `02` 的內容沒有回寫,所以 `02` 的字面準則已被 TEST-008 滿足,缺的部分不可判定(同型於乙-2 的輪數表)<br>**2026-08-16 已回寫**:`02:TEST-005` 補上 §5.3 的完整欄位清單與「預估成本區間必須是區間」的理由(首次／後續差約 8 倍),並記明該欄位現況為**完全不存在**;不在 TEST-008 字面範圍內,故不退回 TEST-008,改由新增工作項 `03` **TEST-011** 承接。**2026-08-16 已實作並勾選**:`PermissionSummary.EstimatedCost`($0.01／常見 $0.06／$0.30,來源為基準試跑 45 筆的閘道實付分布)＋ `RunPreflight` 的「預估成本(估計值)」列,**刻意在 `summary_hash` 之外**(理由同 User Prompt 不入 hash:成本是預測不是權限,重新校準不該作廢既有確認),`TestCostEstimateIsOutsideTheConfirmedHash` 對回應位元組重算 sha256 反證。**契約缺口(記錄,未改 spec)**:`contracts/openapi/public.yaml` 的 `RunPermissionSummary` 尚未宣告 `estimated_cost`,屬 additive,待契約批補 | `02`／PDM-005 的一致性 |
| **乙-10(承 M1)** | ~~宣告閘門 D 日;追認 KPI3 判準修正;**Q15**(匯入 SSRF 在 MVP 期間的歸屬)裁定;~~**anthropic-sa 法務判定**(未決前 `documents` 已索引有 10→6 的塌陷風險,阻擋 CONTENT-003／004 勾選)<br>**2026-08-16 進度**:①**Q15 已裁定**——擴充 `02:SEC-003` 納入匯入抓取器防護(不另立需求 ID),允收準則已寫入,承接工作項 `03` **INGEST-014**(未實作);②**KPI3 判準修正已追認**(附三項條件,經獨立覆核;見 [content-review-report §12](../m1/content-review-report.md))——關鍵發現是「不修判準就得下架」**套錯規則**,KPI3 屬在地化類,該類無兩輪上限亦無下架終點;③**anthropic-sa 已有分析備忘**([anthropic-sa-license-memo.md](anthropic-sa-license-memo.md),四種平台行為逐項評估＋方案 A/B/C),**終判仍在負責人與法務**;④**D 日仍待負責人宣告** | — | M1 驗證閘門正式結案、CONTENT-003／004 |
| ~~**乙-11**~~ **已裁定(2026-08-16):路徑 ②,已通過** | ~~**`docx` 的裁定(CONTENT-005 44/45)**~~ **結果:CONTENT-005 回到 45/45**,見 [content-review-report.md §13](../m1/content-review-report.md)。升 `enrich-skill/v6` 並新增通用第 4 條「不得把原文分別陳述的兩件事串成一件」——**它不是 v4 第 3 條的加強版**:第 3 條管橫向外推(做得到 A→宣稱做得到 A 的鄰居),而 `docx` 的失敗是縱向串接(做得到 A、也做得到 B→宣稱做得到 A 接 B),那句話的**兩半在原文裡各自成立**,正是這一點讓它讀起來像有依據。A 輪 KPI1 由「24 條 1 條未支持」變「18 條 0 條未支持」(串接句消失)但 KPI5 一致性新失敗,B 輪同路徑重跑(不再動 prompt,KPI5 屬既已認定的非決定性自癒類)**通過**;成本 $0.194／上限 $0.5。未走路徑 ③(下架),`docx` 仍是 golden query D01 的 gold primary。<br>~~原待決內容:~~ | 兩輪重審用盡仍未過 KPI1,**兩輪理由不同**(A 輪:「可保證其他內容保持不變」為原文沒有的一般性保證;B 輪:「DOTX 擷取為 Markdown 並依標題重組」把 `.dotx` 擷取與 `.docx→markdown` 兩個獨立事實接起來,屬 v4 第 3 條「支援一種格式不等於支援其近親」的同型外推)。已排除量測假象(`SKILL.md` 6,868 字元,遠低於 Judge 的 40,000 截斷門檻)。**三條路徑,報告不代為決定**:①再給一次非決定性重跑(超出既定兩輪上限,需授權);②比照 v4 先例為「格式近親外推」加更強通用約束並升 v6;③依字面規則列「建議自目錄下架待修」。**選 ③ 的代價要先看清楚**——`docx` 是 `documents` **已索引**筆(下架不影響 CONTENT-001 每類 4–6 的精選下限),但它是 [`catalog-rebuild-report.md` §6.1](../m1/catalog-rebuild-report.md) golden query **D01 的 gold primary 且為現行 Top-1**,下架會使該題失去判定基準。紀錄見 [content-review-report.md §11.3](../m1/content-review-report.md) | **CONTENT-005 的勾選是否維持**;連帶 CONTENT-003 的檢查 ⑦ |
| ~~**乙-12**~~ **已解決(2026-08-16)** | ~~**兩個平台缺陷,無工作項承接**~~ | **裁定:不重開 INGEST-009,以新工作項 `03` INGEST-015 承接(已完成)。** 理由:INGEST-009 的允收準則在當時完全成立且有證據,兩個缺陷都是 **M2 才出現的新資料形態**誘發的(基準試跑在臨時 Workspace 留下 45 筆 fork 文件;修正輪首次做大批量增強),把一個已結案的工作項改回未完成會讓「某一時點的帳」失去意義,而缺陷本身仍必須被修、被記錄——所以記在新項而不是舊項。兩項修法與誠實界線見 `03` INGEST-015。<br>以下為當時的技術定位,保留原文:<br>**(a) 增強實際只有 30 秒,且逾時仍付錢。** `services/platform/internal/llmclient/client.go:25` 的預設 `http.Client{Timeout: 30 * time.Second}` 比 `services/platform/internal/ingest/enrich.go:32` 的 `enrichTimeout = 75s` **更早到期**,所以那個 75 秒的 ctx 預算從來沒被用到。修正輪 14 次成功增強中有 **3 次因此逾時**(`docx` 是 `/embed` 逾時,`excel-split`／`excel-delete` 是增強逾時),重跑即過——但 **client 端放棄不會取消上游,每一次逾時都已在閘道產生費用**。建議把逾時交給 ctx 控制,不要在 client 上另設一個更短的硬期限。<br>**(b) `ReindexAll` 讓補跑清單無法用時間戳篩選。** `db/queries/search.sql:244-253` 對每一筆存活 skill 無條件 `updated_at = now()`,而 `ListPendingEnrichment` 是「全庫 pending、oldest first」——**時間戳與 `REINDEX_BATCH` 都擋不住** M2 基準試跑在 `content-baseline` 臨時 Workspace 留下的 45 筆 fork 文件,照跑會多花約 45 次旗艦增強呼叫(**約 $2**)。修正輪的處置是手動把那 45 筆暫標 `enriched`、跑完還原(實測還原 45/45 逐欄相同),**這是每次補跑都要重做一次的人工步驟**,不是修好了 | ~~任何未來的增強補跑~~ 兩者皆已修;人工暫標步驟不再需要 |

> **CONTENT-005 揭露缺口已關閉(`afb5767`,2026-08-16 完結後回填)**:[content-baseline-report.md §7.2 #1](content-baseline-report.md) 指出的 11 筆(`data-shape`、`docx`、`excel-delete`、`excel-filter`、`excel-find-duplicates`、`excel-mapping-replace`、`excel-merge`、`excel-sort`、`excel-split`、`excel-validate`、`pdf`)「限制」欄漏 Python 執行依賴,已依 `02` §4.7 `需修改` 流程把 prompt 升至 `enrich-skill/v5`(改的是通用條款,非 hardcode 個案)並重跑增強與重新索引,**11/11 揭露缺口全關**。
>
> ~~**但 CONTENT-005 現為 44/45**~~ → **2026-08-16 回到 45/45**:`docx` 依負責人路徑 ② 裁定升 `enrich-skill/v6`,A＋B 兩輪後通過,線上文字指紋 `4df1c13f96c987781f110b7d2232f9fe`([content-review-report.md §13](../m1/content-review-report.md))。`02` §4.7 的非決定性上限條款照樣履行:判定只對現行入庫文字生效,舊版 v5 文字未留存(投影就地覆寫),§11 與 §13 兩份表格各自對應各自那版文字。**CONTENT-005 勾選維持**,CONTENT-003 檢查 ⑦ 維持 `pass`。

> **已關閉(`ddc3e54`)**:`services/sandbox/README.md:112` 原記載被推翻**前**的 SDK 行為(「`settingSources` 含 `project`」),0.3.233 上照做會得到零個 skill。已改為條件表(`cwd`／`settingSources` **省略**／`skills: "all"`／`allowedTools` 傳 `'Skill'` 已 deprecated),並寫明反轉的實測依據與「SDK 升級必須重新實測、不得推理帶過」。~~**仍無 ADR 記錄此次行為反轉**,版本只釘在 `Dockerfile` 的 `ARG CLAUDE_AGENT_SDK_VERSION=0.3.233`。~~ → **2026-08-16 已關閉**:[ADR-023](../../../adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md) 記錄此次反轉,並定下版本釘選(digest ＋ lockfile,不用語意版本範圍)、升級必須重跑的四項實測(skill 載入條件／閘道相容/401／prompt caching 計費欄位與 token 對帳／`usage` 事件的發出條件)、「靜默失效不得用推理帶過」原則,以及證據落檔位置 `infra/images/runtime-agent-sdk/UPGRADES.md`。

## 丙、移交 M3(EVAL)的接點

**M3 是評估里程碑,以下只列接點與已知限制,不代為開工。**

| # | 接點 | 內容 |
| --- | --- | --- |
| 丙-1 | **讀取面:直接用 `trace.Service`** | `Advanced` 已回傳依序重建、標明缺失(`missing_seq`／`late`／`complete`)的完整事件流,`General` 已是聚合摘要。**EVAL-001 引用證據時直接用這兩者,不要自行查 `trace_events`。** 對應的 rubric 缺口見乙-5 |
| 丙-2 | **寫入面:沿用 `trace.RecordOrchestratorEvent`** | 控制平面已會在 `failed`／`timed_out` 轉移的同交易內寫 `error` 事件,`seq` 由 `NextTraceSeq` 在交易內配號。EVAL 要加自己的事件(評估開始／結束)沿用它,**不要另開寫入路徑** |
| 丙-3 | **成本合計仍是下界,但不再有整筆消失的 Run**(2026-08-16 已修) | ~~`run.mjs` 的 `usage` 事件**只在 SDK `result` 分支內發出**~~ → **已修,見 `03` TRACE-009**。harness 現在逐則訊息累計每次模型回應的 token,run 結束時一律發一筆 run 級 `usage`:有 `result` 以其為準(`token_source: result`),沒有(崩潰、牆鐘、token 上限中止、或 SDK 就是沒給)則發累計值(`token_source: accumulated`);兩條路徑的 `cost_usd` 都照舊向閘道要,`cost_source` 維持 `gateway`。事件 schema 因此升 **1.1**(additive)。實例 `add-iso3166` 那類「Run 成功但成本不存在」的情形不會再發生。<br>**留給 M3 的仍然是下界,但下界的成因換了**:①累計值不含「串流結束時仍在途中的回應」;②`cost_usd` 是對閘道的一次讀取,最後一次 flush 若落在讀取之後就少算。**權威對帳來源仍是閘道 per-key spend(ADR-017)**,EVAL-012 引用 Trace 合計時必須標明它是下界。<br>另一個原有結論不變:`complete: true` 只保證「發出的事件沒遺失」,不保證某類事件存在。UI 誠實——無 usage 事件時顯示「沒有記錄到用量事件。」而非 0 |
| 丙-4 | **`skill_activation` 的 `skipped` 不可觀測** | SDK 訊息流看不到「可用但沒被叫」,M2 不臆造。判斷「Skill 沒被啟用」屬 EVAL-002,材料是「Run 掛了哪些 Skill」對照「trace 裡出現了哪些 activation」 |
| 丙-5 | ⚠️ **`succeeded` ≠ 任務完成** | `run.mjs` 的 `finish("succeeded")` 只代表「agent 這一輪沒有拋錯」,**與任務是否完成無關**。實例 `date-wrangling`:終態 `succeeded`、Skill 有啟用、Trace 完整,而 `/out/artifacts/` 是空的——最終回覆是反問使用者。**UI 若把 succeeded 呈現為「可用」會誤導;判定任務是否達成正是 EVAL-001 的工作** |
| 丙-6 | **可比較的基準已在庫** | 45 個 Skill 各一次完整平台 Run 的結果、Trace(1112 事件全數 `masked`)、Artifact manifest 與 Test Case 快照都可重查(對帳 §2.3 已實查驗證)。EVAL-011／012 的「重新試跑與比較」有現成的第一組對照 |
| 丙-7 | **`RunResult.usage` 只有牆鐘** | token 與成本走 Trace,沒有回填到 provider 的 result。契約已於 `eab9d26` 改為誠實敘述(「a consumer must not treat an absent token count here as zero usage」)。EVAL 需要 provider 側 usage 時要走 additive 契約變更 |

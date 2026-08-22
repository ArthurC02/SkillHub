# M2:Skill Lab — 執行計畫

- 日期:2026-08-16
- 狀態:**已完結**(工作項對帳見 [m2-work-items-audit.md](m2-work-items-audit.md),基準 commit `512add1`;殘項三類清單已移至 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md))
- 前提:M1 程式碼面收斂(工作項對帳見 [../m1/m1-work-items-audit.md](../m1/m1-work-items-audit.md));M1 驗證閘門的使用者測試與 M2 開發並行,正式通過與否以測試結果為準(材料:[../gate-test/](../gate-test/))。

## 完結摘要(2026-08-16)

M2 範圍 **41 個工作項**,**33 項維持勾選、1 項對帳退回、7 項維持不勾**(全部為誠實記錄的部分完成或部署期項目),**0 項新勾**。18 個 commit(`3bfd8db..512add1`)、5 份 migration(0016～0020)、2 個新 Go 服務邊界(`services/sandbox` 獨立 module、`internal/trace`)。

> **2026-08-16 完結後更新(追加不改寫)**:乙-7 的同尺審查已裁定,**TEST-003 退回**(TEST-001／002 維持),詳見 [m2-work-items-audit.md §12](m2-work-items-audit.md)。上段的 33／1 因此成為 **32／2**;退回的理由與 TEST-004 同型(缺使用者面,後端完整),實作由新增的 `03` **TEST-012** 承接。

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
| Test Case 與執行設定 | TEST-001/002/003/004/008/009/010/011(005/006/007 後 MVP) | 第一批起 **六項完成**(TEST-002/008/009 於權限摘要批);**TEST-004 對帳退回後於 2026-08-16 補上顯示面重新勾選**;**TEST-011(預估成本區間)為 2026-08-16 新增並完成**;**TEST-003 於 2026-08-16 同尺審查後退回**(乙-7,缺驗收條件的使用者面),介面由新增的 **TEST-012** 承接 |
| Trace Schema | TRACE-001(事件 schema 先行,收集屬後續批) | 第一批 **✅ 完成**(由第三批勾選,`0019` 補齊四個欄位缺口後才成立) |
| Run 排程與韌性 | RUN-005~009(排程、取消逾時、冪等清理、重啟恢復、契約測試) | 第二批 **✅ 2026-08-16 完成**(含 Outbox publisher;RUN-004 一併結案) |
| SelfHostedProvider | SBX-001~010(gVisor 基線) | 第二~四批 **六項完成**(001/003/004/006/008/009);**002/005/007/010 維持不勾**——門檻待定值＋生產網路與逃逸測試屬部署期 |
| Trace 收集與 O11y | TRACE-002~008、O11Y-001~003 | 第三批 **✅ 2026-08-16 完成**（TRACE-001 一併勾選；TRACE-004 的成本欄位由第四批補上並勾選） |
| 模型閘道出口與短效授權 | SBX-007（dev 網路面）、SBX-008、TRACE-004 成本 | 第四批 **2026-08-16**（SBX-008 完成、TRACE-004 勾選；SBX-007 仍不勾——Proxy 本體屬部署期） |
| 內容基準試跑 | CONTENT-007/008(自 M1 移入) | 第五批 **✅ 2026-08-16**(CONTENT-008 完成、精選 15/15 符合;CONTENT-007 部分完成不勾——writing rubric 缺消費端。**同日補跑＋全量重測**:預算保留根因修復後 9 筆於 `2026.08-2` 重測(§12),負責人裁定後其餘 36 筆亦重測(§13)——33 筆 `transpiled` 全數轉 `native`、成本 −28%、輸入 token −50%,全 45 筆最新量測符合 42/45,相容軸 45 列入庫;**CONTENT-010**(§14)於 deps 修正後的 `2026.08-3` 再全量重測,41 筆完成、4 筆授權受限如實跳過,`ModuleNotFoundError` 歸零、判定零變動,回填 41 列。見 [content-baseline-report.md](content-baseline-report.md)) |
| 安全驗收 | SEC-002 六項門檻定值(Q18)、SEC-009 逃逸測試——ADR-015 的實作期驗收關卡 | 部署驗證批。**門檻定值與 Q1～Q3 已於 2026-08-16 由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 定案**;SEC-002／SEC-009 仍不勾——**閘門 B 的四項阻擋已於 2026-08-16 全數落地(乙-8)**,剩下的唯一原因是 45 項基線未經 SEC-009 驗證 |

## 檔案地圖

| 檔案 | 類型 | 一句話用途 | 狀態 |
| --- | --- | --- | --- |
| [`README.md`](README.md)(本檔) | 計畫 | M2 的計畫、五批交付摘要、契約缺口與本目錄導覽 | **活文件**——殘項解除或補件批仍會回填(追加不改寫) |
| [`m2-work-items-audit.md`](m2-work-items-audit.md) | 審計 | M2 全 41 個工作項的逐項對帳與退回理由,是某一時點的帳 | 凍結 |
| [`content-baseline-report.md`](content-baseline-report.md) | 報告 | CONTENT-007／008 基準試跑:45 個真實 Skill 的端到端 Run、成本、Agent 相容軸量測與三輪重測 | 凍結 |
| [`../governance/anthropic-sa-license-memo.md`](../governance/anthropic-sa-license-memo.md) | 治理 | `anthropics/skills` source-available 條款的平台行為風險分析與方案 A/B/C(**非法律意見**,終判在負責人與法務) | **已移至 `../governance/`**(2026-08-16,ADR-024);凍結 |
| [`../governance/anthropic-sa-inquiry-draft.md`](../governance/anthropic-sa-inquiry-draft.md) | 治理 | 上述備忘的並行建議:上游詢問信草稿,由負責人親自審閱後以自己身分寄出 | **已移至 `../governance/`**(2026-08-16,ADR-024);凍結 |

殘項三類清單不在本目錄,見 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md)(活文件)。

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

Prometheus 文字格式,platform(獨立 listener `METRICS_ADDR`,不掛對外 port)與 sandboxd(與其他路由同一個 bearer)各自曝露。指標清單、標籤規約與告警規則見 [infra/observability/README.md](../../../../infra/observability/README.md)。

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

M2 完結後的補件批,處理殘項清單([`04`](../../04-backlog-and-handoffs.md))裡「已定值但沒有強制」與「已量到但沒查根因」那幾項。migration `0021`,無契約變更。

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

# M2 殘項清單（三類）→ 已移至 [`04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md)

甲（部署期驗收）／乙（待負責人決策）／丙（移交 M3 的接點）三類清單**不再放在本文件**：它會隨裁定與部署持續變動，而本目錄是 M2 完結時的一份帳；兩處各留一份必定漂移，所以只留一處——[`plans/04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md)，那是「目前還缺什麼、誰在等誰」的唯一入口。逐項證據仍在 [m2-work-items-audit.md](m2-work-items-audit.md)。

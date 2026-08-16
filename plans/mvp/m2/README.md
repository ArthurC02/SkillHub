# M2:Skill Lab — 執行計畫

- 日期:2026-08-16
- 狀態:**已完結**(工作項對帳見 [m2-work-items-audit.md](m2-work-items-audit.md),基準 commit `512add1`;殘項三類清單見本文件末節)
- 前提:M1 程式碼面收斂(工作項對帳見 [../m1/m1-work-items-audit.md](../m1/m1-work-items-audit.md));M1 驗證閘門的使用者測試與 M2 開發並行,正式通過與否以測試結果為準(材料:[../m1/gate-test/](../m1/gate-test/))。

## 完結摘要(2026-08-16)

M2 範圍 **41 個工作項**,**33 項維持勾選、1 項對帳退回、7 項維持不勾**(全部為誠實記錄的部分完成或部署期項目),**0 項新勾**。18 個 commit(`3bfd8db..512add1`)、5 份 migration(0016～0020)、2 個新 Go 服務邊界(`services/sandbox` 獨立 module、`internal/trace`)。

**M2 里程碑目標(`01` §M2)的達成狀況**:

| `01` 的 M2 目標 | 狀況 |
| --- | --- |
| 完成 Cloud Sandbox、Dataset、Prompt 與 Run Trace | ✅ 後端完成並經 45 個真實 Skill 端到端驗證;**Test Lab 無 UI**(見殘項乙-7) |
| 完成權限確認、逾時、取消及清理 | ✅ TEST-008/009、RUN-006/007、SBX-009;73/73 Run 全部 `cleaned` |
| 完成精選 Skill 的範例資料、Prompt、驗收條件與基準試跑 | ✅ CONTENT-008(15/15 符合);**CONTENT-007 差 writing rubric 一項**(見殘項乙-5) |
| **依 Sandbox 實測結果啟用搜尋的 Agent 相容篩選維度** | ❌ **未達成**。資料已備妥(45/45 `activated`、33 個「模型轉譯」),但 schema 無欄位可回寫,四個設計問題待決(見殘項乙-4) |

**單一最重要的發現**:PDM-005 的 300K/60K token 硬上限被寫進 `policy_snapshot`、**顯示在執行前權限摘要上並要求使用者確認**,而平台沒有任何一行程式會因為超過它而停止一個 Run。它不違反任何一條已勾項的字面允收準則(`02:RUN-003` 與 SBX-006 的清單裡都沒有 token),因此在 18 個 commit 裡一路沒被發現——**它是孤兒殘項**。詳見對帳 §9.1。

## 範圍(對應 03-work-items)

| 工作群 | 項目 | 里程碑內順序 |
| --- | --- | --- |
| Run 契約與狀態機 | RUN-001~004(Provider-neutral 契約、Capability、run_id 映射、狀態機) | 第一批 **✅ 完成**(RUN-004 由第二批一併結案) |
| Test Case 與執行設定 | TEST-001/002/003/004/008/009/010(005/006/007 後 MVP) | 第一批起 **六項完成**(TEST-002/008/009 於權限摘要批;**TEST-004 對帳退回**——缺上傳前的顯示面) |
| Trace Schema | TRACE-001(事件 schema 先行,收集屬後續批) | 第一批 **✅ 完成**(由第三批勾選,`0019` 補齊四個欄位缺口後才成立) |
| Run 排程與韌性 | RUN-005~009(排程、取消逾時、冪等清理、重啟恢復、契約測試) | 第二批 **✅ 2026-08-16 完成**(含 Outbox publisher;RUN-004 一併結案) |
| SelfHostedProvider | SBX-001~010(gVisor 基線) | 第二~四批 **六項完成**(001/003/004/006/008/009);**002/005/007/010 維持不勾**——門檻待定值＋生產網路與逃逸測試屬部署期 |
| Trace 收集與 O11y | TRACE-002~008、O11Y-001~003 | 第三批 **✅ 2026-08-16 完成**（TRACE-001 一併勾選；TRACE-004 的成本欄位由第四批補上並勾選） |
| 模型閘道出口與短效授權 | SBX-007（dev 網路面）、SBX-008、TRACE-004 成本 | 第四批 **2026-08-16**（SBX-008 完成、TRACE-004 勾選；SBX-007 仍不勾——Proxy 本體屬部署期） |
| 內容基準試跑 | CONTENT-007/008(自 M1 移入) | 第五批 **✅ 2026-08-16**(CONTENT-008 完成、精選 15/15 符合;CONTENT-007 部分完成不勾——writing rubric 缺消費端。見 [content-baseline-report.md](content-baseline-report.md)) |
| 安全驗收 | SEC-002 六項門檻定值(Q18)、SEC-009 逃逸測試——ADR-015 的實作期驗收關卡 | 部署驗證批。**門檻定值與 Q1～Q3 已於 2026-08-16 由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 定案**;SEC-002／SEC-009 仍不勾(45 項未經驗證、閘門 B 兩項阻擋未落地) |

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
docker build -t skillhub/runtime-agent-sdk:2026.08-1 infra/images/runtime-agent-sdk
# 3) sandboxd(掛 docker.sock,沙箱容器會被放到 skillhub_egress)
docker run -d --name sandboxd --network skillhub_default --network-alias sandboxd \
  -v "$PWD:/src" -v /var/run/docker.sock:/var/run/docker.sock -w /src/services/sandbox \
  -e SKILLHUB_SANDBOX_TOKEN=... -e SKILLHUB_SANDBOX_NETWORK=skillhub_egress \
  -e SKILLHUB_SANDBOX_IMAGE=skillhub/runtime-agent-sdk:2026.08-1 \
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
| 甲-4 | **SBX-002 的門檻定值** ~~待定~~ **已定值** | 流水線已接上(digest 斷言→build→syft SBOM→grype→門檻)。**I-06 與 I-04 的提案值已由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分採納定案**(可修的 Critical／High 阻擋且**無豁免路徑**、不可修者具名豁免、複審日 ＝ `first_exempted_at`＋90 天;掃描有效期 **30 天**、到期前 7 天告警),`infra/images/README.md` 與 `runtime-image.yml` 的「暫定／PROVISIONAL」字樣**已改為定案(`606ad87`)**。**SBX-002 仍不勾的唯一原因**:I-03 的 SBOM 與 I-04 的 `scanned_at` 現落在 90 天 CI artifact 與人工維護的日期。**registry 已定案為 GHCR**(ADR-022 補充決策,ADR-019 待決策 1 已回填),所以這已不是結構性阻擋而是一件待做的工作——見新增工作項 **SBX-011**(接 GHCR push ＋ SBOM/掃描 attestation) | SBX-011 完成 |

## 乙、待負責人決策(**寫程式之前先要有人拍板**)

| # | 項目 | 待決內容 | 阻擋什麼 |
| --- | --- | --- | --- |
| ~~**乙-1**~~ **已解決(2026-08-16)** | ~~**SEC-002 六項門檻值(威脅模型 Q18)**~~ | → **[ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md)**:六項門檻全部定值(P-03 節點 7 天滾動重建;P-04 gVisor 基準 ＝ 上游 N/N−1 且 ≤ 90 天、逃逸類 CVE 24 h 換版或停用、High 7 天;I-04 掃描有效期 30 天;I-06 可修的 Critical／High 阻擋且無豁免路徑、不可修者具名豁免 90 天複審;X-02 每 5 分鐘;X-03 連續 2 輪告警、X-04 單節點 ≥50% slot drain／全池 ≥25%(下限 2 筆)暫停),Q1～Q3 亦已定案(compose-per-VM／執行平面單租戶且同節點多 Run／nftables default-deny ＋固定 DNS,允許清單存 `infra/egress/allowlist.yaml`)。<br>**但 SEC-002 仍不勾**——見甲-1／甲-2(45 項未經驗證)與乙-8(閘門 B 兩項阻擋未落地);**SBX-002 仍不勾**——I-04 的 `scanned_at` 在 container registry(ADR-019 待決策 1)定案前無法隨 image 保存,見甲-4 | ~~SEC-002 勾選、SBX-002 勾選、甲-3／甲-4~~ 剩餘阻擋見右列說明 |
| **乙-2** | ⚠️ **PDM-005 token 硬上限:二選一** | `300_000`／`60_000` 被寫進 `policy_snapshot`、**顯示在執行前權限摘要上要求使用者確認**,而平台與沙箱**都不強制**(`TokenBudget` 零消費端,無累加器)。實際煞車只有 `max_budget`／`tpm_limit`／牆鐘,而 PDM-005 §5.2a-4 明文說過這三個都不是 token 上限。<br>**(a)** 新增實作工作項「依閘道回報 `input_tokens` 累計並於超限時終止 Run」,並把 §5.2a 的輪數換算表(15／7.7／5 輪)回寫 `02:RUN-003`;或 **(b)** 正式承認 MVP 不做,**把 `TokenBudget` 從權限摘要移除**。<br>**現狀(顯示但不強制)是兩者中最壞的一種**——讓使用者確認一個平台不會執行的上限,直接踩 NFR-001「UI 不得誤導」 | 無工作項承接;PDM-003 v5 的定案條件之一(「三層併用」)因此未完整成立 |
| **乙-3** | **閘道 $0.50 預算計數 50 倍誤差的處置** | LiteLLM 以「Current cost ≈ 0.50」拒絕請求,而同一把金鑰的 `LiteLLM_SpendLogs` 只有 $0.009–$0.24;兩個對照實驗排除「單次呼叫預估扣款」;上限提到 $2.00 後 7 個受影響精選全數一次通過。**在查清之前 per-Run `max_budget` 不是可信的成本閘門**,PDM-003 v5 的 $0.50 預設需重新檢視。連帶:9 個已索引 Skill 尚無有效基準(補跑預估 $1.0–1.5) | 乙-2 的三個煞車裡最重要的那個本身不可信;CONTENT-008 已索引層的補跑 |
| **乙-4** | **Agent 相容軸 schema 的四個待決** | ①欄位歸屬:Skill Version 層級,還是 (Skill Version × Runtime Image) 層級?**後者才誠實**——33 個 Python 結論只對 `skillhub/runtime-agent-sdk:2026.08-1` 成立。②兩軸判準:`capability` 可直接供給(45/45 有 `skill_activation`),`runtime` 需要「腳本宣告環境 ⊆ 映像提供環境」的判定,而映像的能力清單目前不以資料形式存在。③樣本量:一次 Run 不足以宣告 `passed`,幾次、失敗一次是否降級未定。④值域:需要 `unverified` 以外的第三態表達「腳本不會被執行、由模型轉譯」,它既不是 passed 也不是 failed。<br>**資料已備妥**:`capability = activated` 45/45;`runtime` 為 11 原生／1 node／33 模型轉譯 | **`01` 的 M2 里程碑第 4 項**、`02:DISC-002` 的 Agent 維度 |
| **乙-5** | **CONTENT-007 的 writing rubric** | `02` §4.7 第 3 條要求 `writing` 類每個精選附一份可編輯 rubric,供 LLM Judge 逐項回傳證據引文。**未做,因為 rubric 沒有消費端**——EVAL-001／002 的 Judge 介面尚未實作。其餘四條允收皆達成 | CONTENT-007 勾選;實質上是 M3 的前置(見丙-1) |
| **乙-6** | **Runtime Image 要不要含 python3** | 45 個 Skill 中 33 個 `deps_runtime = python`,映像基底 `node:22-bookworm-slim` 只加 `unzip`。含 Python 讓 33 個 Skill 執行自己的腳本(**也擴大沙箱攻擊面**),不含則平台永遠是「模型轉譯」語意,**目錄必須誠實標示**(＝乙-4)。這是產品決策不是工程細節 | 乙-4 的值域設計;CONTENT-005 揭露完整度 |
| **乙-7** | **`03` §9 的允收是否含 UI** | `apps/web` 有 `RunPreflight`／`RunTrace`,**沒有任何 Test Lab／Dataset 頁面**。TEST-004 已因 `02:TEST-002`「上傳前**顯示**…」退回;TEST-001／002／003 的準則以「使用者可…」起頭,若同尺解讀亦應退回。**(a)** 承認含 UI,一併退回並新增實作工作項;或 **(b)** 明文把 §9 界定在 API＋契約層,UI 由 `DESIGN-007`(未勾)與一個新工作項承接。任一都要三文件同步 | TEST-001/002/003 的勾選正確性 |
| **乙-8** | **SEC-002 閘門 B 的兩項未落地阻擋** | ①「Skill Version 靜態掃描結果為阻擋級」——`internal/run` 內無 severity 判斷,根因是威脅模型 **Q7**(阻擋 vs 警告的具體 Policy)未決,`02:SEC-003` 自陳政策未定案前該條件不可判定。②「超出 Workspace 並行或額度上限」——**PDM-005 §5.2 已定值(每 Workspace 並行 Run 上限=2),強制不存在**;`ConcurrentRunSlots` 是 Provider 側容量不是 Workspace 配額。**②是四項阻擋裡唯一不依賴任何未決策的,可直接實作** | SEC-002 |
| **乙-9** | **PDM-005 §5.3 未回寫 `02:TEST-005`** | §5.3 指定的權限摘要欄位裡,**「預估成本區間」完全不存在**於 `PermissionSummaryContent`。§5.2a-6 特別強調必須是**區間**不是單值(首次與後續 Run 因 prompt caching 差約 8 倍)。根因是 PDM 指定要回寫 `02` 的內容沒有回寫,所以 `02` 的字面準則已被 TEST-008 滿足,缺的部分不可判定(同型於乙-2 的輪數表) | `02`／PDM-005 的一致性 |
| **乙-10(承 M1)** | 宣告閘門 D 日;追認 KPI3 判準修正;**Q15**(匯入 SSRF 在 MVP 期間的歸屬)裁定;**anthropic-sa 法務判定**(未決前 `documents` 已索引有 10→6 的塌陷風險,阻擋 CONTENT-003／004 勾選) | — | M1 驗證閘門正式結案、CONTENT-003／004 |
| **乙-11** | **`docx` 的裁定(CONTENT-005 44/45)** | 兩輪重審用盡仍未過 KPI1,**兩輪理由不同**(A 輪:「可保證其他內容保持不變」為原文沒有的一般性保證;B 輪:「DOTX 擷取為 Markdown 並依標題重組」把 `.dotx` 擷取與 `.docx→markdown` 兩個獨立事實接起來,屬 v4 第 3 條「支援一種格式不等於支援其近親」的同型外推)。已排除量測假象(`SKILL.md` 6,868 字元,遠低於 Judge 的 40,000 截斷門檻)。**三條路徑,報告不代為決定**:①再給一次非決定性重跑(超出既定兩輪上限,需授權);②比照 v4 先例為「格式近親外推」加更強通用約束並升 v6;③依字面規則列「建議自目錄下架待修」。**選 ③ 的代價要先看清楚**——`docx` 是 `documents` **已索引**筆(下架不影響 CONTENT-001 每類 4–6 的精選下限),但它是 [`catalog-rebuild-report.md` §6.1](../m1/catalog-rebuild-report.md) golden query **D01 的 gold primary 且為現行 Top-1**,下架會使該題失去判定基準。紀錄見 [content-review-report.md §11.3](../m1/content-review-report.md) | **CONTENT-005 的勾選是否維持**;連帶 CONTENT-003 的檢查 ⑦ |
| **乙-12** | **兩個平台缺陷,無工作項承接** | 歸 **乙** 不歸丙:兩者都不碰 M3 的評估路徑,且承接它們的 **INGEST-009 已勾選並隨 M1 結案**,所以要先有人裁定「重開 INGEST-009 還是新增工作項」,才談得上修。技術事實已定位,不需再調查:<br>**(a) 增強實際只有 30 秒,且逾時仍付錢。** `services/platform/internal/llmclient/client.go:25` 的預設 `http.Client{Timeout: 30 * time.Second}` 比 `services/platform/internal/ingest/enrich.go:32` 的 `enrichTimeout = 75s` **更早到期**,所以那個 75 秒的 ctx 預算從來沒被用到。修正輪 14 次成功增強中有 **3 次因此逾時**(`docx` 是 `/embed` 逾時,`excel-split`／`excel-delete` 是增強逾時),重跑即過——但 **client 端放棄不會取消上游,每一次逾時都已在閘道產生費用**。建議把逾時交給 ctx 控制,不要在 client 上另設一個更短的硬期限。<br>**(b) `ReindexAll` 讓補跑清單無法用時間戳篩選。** `db/queries/search.sql:244-253` 對每一筆存活 skill 無條件 `updated_at = now()`,而 `ListPendingEnrichment` 是「全庫 pending、oldest first」——**時間戳與 `REINDEX_BATCH` 都擋不住** M2 基準試跑在 `content-baseline` 臨時 Workspace 留下的 45 筆 fork 文件,照跑會多花約 45 次旗艦增強呼叫(**約 $2**)。修正輪的處置是手動把那 45 筆暫標 `enriched`、跑完還原(實測還原 45/45 逐欄相同),**這是每次補跑都要重做一次的人工步驟**,不是修好了 | 任何未來的增強補跑;INGEST-009 的實質正確性 |

> **CONTENT-005 揭露缺口已關閉(`afb5767`,2026-08-16 完結後回填)**:[content-baseline-report.md §7.2 #1](content-baseline-report.md) 指出的 11 筆(`data-shape`、`docx`、`excel-delete`、`excel-filter`、`excel-find-duplicates`、`excel-mapping-replace`、`excel-merge`、`excel-sort`、`excel-split`、`excel-validate`、`pdf`)「限制」欄漏 Python 執行依賴,已依 `02` §4.7 `需修改` 流程把 prompt 升至 `enrich-skill/v5`(改的是通用條款,非 hardcode 個案)並重跑增強與重新索引,**11/11 揭露缺口全關**。
>
> **但 CONTENT-005 現為 44/45**:`docx` 兩輪重審皆未過,且**第二輪的理由與第一輪不同**(格式近親外推,與 Python 揭露無關),依兩輪上限規則記為 `需修改` 留給負責人,未硬修、未就地改寫。`02` §4.7 的**非決定性上限條款已履行**——入庫文字的 md5 指紋逐筆存查([content-review-report.md §11](../m1/content-review-report.md))。**CONTENT-005 的勾選是否維持,取決於 `docx` 的裁定**(見乙-11)。

> **已關閉(`ddc3e54`)**:`services/sandbox/README.md:112` 原記載被推翻**前**的 SDK 行為(「`settingSources` 含 `project`」),0.3.233 上照做會得到零個 skill。已改為條件表(`cwd`／`settingSources` **省略**／`skills: "all"`／`allowedTools` 傳 `'Skill'` 已 deprecated),並寫明反轉的實測依據與「SDK 升級必須重新實測、不得推理帶過」。**仍無 ADR 記錄此次行為反轉**,版本只釘在 `Dockerfile` 的 `ARG CLAUDE_AGENT_SDK_VERSION=0.3.233`。

## 丙、移交 M3(EVAL)的接點

**M3 是評估里程碑,以下只列接點與已知限制,不代為開工。**

| # | 接點 | 內容 |
| --- | --- | --- |
| 丙-1 | **讀取面:直接用 `trace.Service`** | `Advanced` 已回傳依序重建、標明缺失(`missing_seq`／`late`／`complete`)的完整事件流,`General` 已是聚合摘要。**EVAL-001 引用證據時直接用這兩者,不要自行查 `trace_events`。** 對應的 rubric 缺口見乙-5 |
| 丙-2 | **寫入面:沿用 `trace.RecordOrchestratorEvent`** | 控制平面已會在 `failed`／`timed_out` 轉移的同交易內寫 `error` 事件,`seq` 由 `NextTraceSeq` 在交易內配號。EVAL 要加自己的事件(評估開始／結束)沿用它,**不要另開寫入路徑** |
| 丙-3 | ⚠️ **成本合計必然是下界** | `run.mjs` 的 `usage` 事件**只在 SDK `result` 分支內發出**(`run.mjs:354-380`)。串流在沒有 `result` 的情況下結束(崩潰、被牆鐘殺、SDK abort)就**完全不發** `usage`——不是發 0,是不發。實例 `add-iso3166`:Run 成功、13 個事件、無斷號、`complete: true`,而 token 與成本在平台端不存在。<br>**兩個後果**:①`complete: true` 不代表 usage 存在(斷號只看得到「發出後遺失」,看不到「從未發出」);②**EVAL-012 若直接加總 Trace 的 `cost_usd` 會系統性低估**,低估幅度隨失敗率上升(實測:Trace $3.0879 vs 閘道實付 $3.3932)。**權威來源是閘道 per-key spend(ADR-017)**。UI 目前誠實——無 usage 事件時顯示「沒有記錄到用量事件。」而非 0 |
| 丙-4 | **`skill_activation` 的 `skipped` 不可觀測** | SDK 訊息流看不到「可用但沒被叫」,M2 不臆造。判斷「Skill 沒被啟用」屬 EVAL-002,材料是「Run 掛了哪些 Skill」對照「trace 裡出現了哪些 activation」 |
| 丙-5 | ⚠️ **`succeeded` ≠ 任務完成** | `run.mjs` 的 `finish("succeeded")` 只代表「agent 這一輪沒有拋錯」,**與任務是否完成無關**。實例 `date-wrangling`:終態 `succeeded`、Skill 有啟用、Trace 完整,而 `/out/artifacts/` 是空的——最終回覆是反問使用者。**UI 若把 succeeded 呈現為「可用」會誤導;判定任務是否達成正是 EVAL-001 的工作** |
| 丙-6 | **可比較的基準已在庫** | 45 個 Skill 各一次完整平台 Run 的結果、Trace(1112 事件全數 `masked`)、Artifact manifest 與 Test Case 快照都可重查(對帳 §2.3 已實查驗證)。EVAL-011／012 的「重新試跑與比較」有現成的第一組對照 |
| 丙-7 | **`RunResult.usage` 只有牆鐘** | token 與成本走 Trace,沒有回填到 provider 的 result。契約已於 `eab9d26` 改為誠實敘述(「a consumer must not treat an absent token count here as zero usage」)。EVAL 需要 provider 側 usage 時要走 additive 契約變更 |

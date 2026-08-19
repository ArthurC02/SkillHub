# 技術債與設計修復總帳（2026-08-19）

## 1. 目的與判定規則

本文件是 MVP 程式面收斂後的跨層審查與修復總帳。範圍包含產品流程、架構、資料一致性、安全、LLM／Evaluation、Sandbox、Web、契約、CI/CD、供應鏈、測試與維運設計；不把單純「未採最新版本」算成缺陷。

Finding 只有在下列條件都成立時才進入總帳：

1. 有可定位到程式、契約或決策文件的證據；
2. 能說明被破壞的不變量或可觀察影響；
3. 已檢查現有控制，排除重複項與假陽性；
4. 有最小修復與可自動驗證的完成條件。

狀態只有 `open`、`fixed`、`decision`、`deployment` 四種。`fixed` 必須同時有實作、回歸測試與相關檢查通過；外部負責人或真實環境才能完成的項目不得偽裝成 `fixed`。

## 2. 執行順序（阻抗迴路）

1. **S0 安全邊界**：Internal LLM Auth、URL secret、破壞性 DB test guard。
2. **S1 資料與執行正確性**：Run snapshot TOCTOU、Skill/Test Case 關聯、Sandbox 大檔傳輸、Evaluation evidence／截斷／重試。
3. **S2 產品可達性**：登入／登出、Skill 匯入、Run 取消、Evaluation 更新、列表分頁。
4. **S3 可靠性與契約**：HTTP timeout、登入 race、LLM 422／malformed response、model routing、Trace 與 Outbox 競態。
5. **S4 供應鏈與維運**：image lock、workflow privilege/pinning、DB/observability/QA/race lanes、文件漂移。
6. 每批都執行「實作 → 定向測試 → 全域檢查 → 未參與實作的 Reviewer 反向 Review」。有 finding 就回到該批；完整一輪零 finding 才結束。

## 3. Finding ledger

### P0

| ID | 狀態 | 問題與主要證據 | 完成條件 |
| --- | --- | --- | --- |
| LLM-SEC-001 | open | ADR-020 要求 Go→Python 靜態 Bearer；`llm-internal.yaml` 卻是 `security: []`，Go client 不送、FastAPI 不驗。 | OpenAPI、Go、Python、環境範本與部署一致 default-deny；錯／缺 token 在觸發 gateway 前回 401。 |

### P1

| ID | 狀態 | 問題與主要證據 | 完成條件 |
| --- | --- | --- | --- |
| GOV-RETENTION-001 | decision | `packaging.DefaultRetention` 在環境值缺失／錯誤時套用尚未追認的 90 天。 | 未核定時 fail-closed；有效政策才可建立下載產物，或由 PDM 正式追認後以測試鎖定。 |
| DEPLOY-IAC-001 | deployment | ADR-022 要求的 sandbox node Compose/cloud-init/render 實體尚不存在。 | 以最小 IaC 建 node，未填 pinned IP 時無放行規則，完成 ADR-022 真實驗收。 |
| FE-AUTH-001 | open | Web 無 `/auth/github/login` 或 `/auth/logout` 入口。 | 匿名可登入、登入者可登出且清 cache；瀏覽器測試涵蓋完整生命週期。 |
| FE-IMPORT-001 | open | 後端有 URL/upload import，Web 無 route 或 adapter。 | `/workspace/import` 支援 URL／multipart、findings、dedupe 與成功導頁。 |
| FE-CANCEL-001 | open | 後端有 Run cancel，Web 無 mutation/control。 | 可取消狀態顯示確認操作；race 409 正確重讀。 |
| RUN-PERMISSION-001 | open | permission hash 驗證在 transaction 外，snapshot 不鎖 Test Case；Dataset 可在兩者間變更。 | 驗 hash、鎖 parent、snapshot 同交易；所有影響 permission 的 mutation 使用同一鎖。 |
| INGEST-SECRET-001 | open | URL userinfo/query/fragment 可進 DB、log、HTTP error 與 download manifest。 | credential URL fail-closed；持久化與輸出只用 canonical/redacted provenance。 |
| LLM-EVAL-001 | open | 非 undetermined verdict 可在 evidence 驗證後零證據仍保留 passed/failed。 | 零有效證據一律降為 undetermined 並重算 overall。 |
| LLM-EVAL-002 | open | Trace excerpt 超過 2,000 字被截斷，但 truncation/complete 未反映。 | 任一截斷會標記不完整，不能形成 pass。 |
| LLM-EVAL-003 | decision | Python 允許 runtime/tool/dataset 建議，Go/DB 要求 file path/content 而靜默丟棄。 | 收窄成可套用 skill 建議，或完整支援不可套用建議；不得靜默 drop。 |
| LLM-EVAL-004 | open | Python `_clip` 完整 replacement/path，截斷內容仍可能被核准套用。 | 超限 proposal 整筆拒絕為 malformed，不改寫模型輸出。 |
| LLM-EVAL-005 | open | Evaluation job retry 會建立新 evaluation 並再次付費呼叫 Judge。 | retry 固定 attempt identity；Judge 已成功而持久化失敗時最多一次付費呼叫。 |
| LLM-RES-001 | open | 匿名 search 無 query 長度上限與 rate/cost quota，可反覆觸發 embedding/match reason。 | API/Go/Python 同一上限；匿名節流，拒絕時不呼叫 LLM。 |
| TEST-DB-DESTRUCTIVE-001 | open | integration harness 對任意 `SKILLHUB_TEST_DATABASE_URL` 執行 `DROP SCHEMA public CASCADE`。 | 隨機 schema 或嚴格 test DSN guard；非 test target 在 Exec 前拒絕。 |
| SBX-TRANSFER-001 | open | `/bin/tee` 回聲大檔，driver 只讀 1 MiB 且不檢查 exec running，可能 hang／部分寫入。 | 改無 stdout 寫入或完整 concurrent drain；大於 1 MiB exact-byte 測試通過。 |
| SBX-FETCH-LIMIT-001 | open | `LimitReader(64MiB)` 對 64MiB+1 靜默當成功。 | 讀 limit+1 並回錯；不得把 URL secret 帶入錯誤。 |

### P2

| ID | 狀態 | 問題與主要證據 | 完成條件 |
| --- | --- | --- | --- |
| MODEL-CONFIG-001 | decision | Match Reason 決策為 Luna；程式預設 `gpt-4o-mini`，LiteLLM allowlist 兩者皆無。 | PDM/ADR、app default、gateway allowlist 一致；靜態 mapping test。 |
| CONTRACT-LLM-422-001 | open | OpenAPI 422 `Error.detail` 是 string；FastAPI 預設是 array。 | 每條 route 的 422 runtime response 通過 schema validation。 |
| RUNTIME-PYTHON-001 | decision | PDM／驗收目標 3.12，image 實際 3.11.2。 | 選定一個版本並同步 image、文件與真實 gVisor 證據。 |
| FE-EVAL-POLL-001 | open | Evaluation 404/pending 無 polling/event invalidation。 | 404→pending→terminal 自動更新，terminal 停止 timer。 |
| FE-STATE-IDENTITY-001 | open | 同 route A→B 復用前一資源 local state，可能把 A 表單寫入 B。 | resource identity 改變時安全 reset；mutation target/value regression。 |
| FE-TRACE-SCALE-001 | open | Trace 每 3 秒全量 DB/傳輸/DOM，terminal 後仍輪詢。 | active-only、增量 cursor/limit 或 summary snapshot；大事件量測試。 |
| FE-PAGINATION-001 | open | Run 只取前 50、Test Case 前 100，UI 無下一頁。 | cursor/has_more 與 Load more；51/101 邊界無遺漏重複。 |
| FE-GENERATED-ADAPTER-001 | open | generated TS client 未被 Web adapter 使用，transport 仍以 unchecked cast mirror。 | 逐 endpoint DTO→view model 遷移並有 contract fixture；不整檔替換 UI model。 |
| RUN-TESTCASE-001 | open | 同 Workspace 可用 Skill A Version 搭配 Skill B Test Case。 | preflight/confirm/create 都拒絕不相屬 Test Case。 |
| IDENTITY-RACE-001 | open | 同 GitHub identity 首次登入併發會 unique conflict/500。 | 兩個並發 callback converge 到同一 user/workspace/identity，皆取得 session。 |
| HTTP-TIMEOUT-001 | open | GitHub OAuth、URL import/probe 使用無 absolute timeout 的 default client。 | bounded client；hang server 在預算內失敗且無 goroutine 累積。 |
| LLM-EVAL-006 | open | Suggest prompt 可印第 9+ evidence，但 allowlist digest 只留前 8。 | prompt 與 allowlist 使用完全相同 evidence 集合。 |
| LLM-EVAL-007 | decision | Suggest usage/cost 由 Python 回傳但 Go 丟棄；帳務落點未定。 | 決定並落地 usage/cost 的唯一事實來源與 contract test。 |
| LLM-RES-002 | open | LiteLLM empty choices/data、缺 embedding、維度錯誤會落成未約定 500。 | 所有 malformed gateway envelope 正規化成不洩漏的 502。 |
| SUPPLY-PREDICATE-001 | open | runtime image 的 scan predicate PR 不跑，push image 後才生成。 | PR/push 都先生成與 gate，再允許 push/attest。 |
| TEST-DB-SQL-001 | open | `db/tests/immutability_test.sql` 未接 Task/CI。 | `test:db` 納入 full test/CI，破壞約束會紅。 |
| SUPPLY-RUNTIME-LOCK-001 | open | runtime image Python/Node transitive dependency live resolve。 | repo-owned lock/constraints；乾淨 cache 建置 dependency tree 一致。 |
| SBX-IMAGE-DIGEST-001 | open | runsc production 仍接受 image tag，README default 也已落後。 | runsc 強制 `@sha256:`；config test 與文件同步。 |
| CI-PRIVILEGE-001 | open | runtime image PR review job 取得 packages/id-token/attestation write。 | review/publish 分 job；PR 僅 contents read。 |
| SUPPLY-IMAGE-VERSION-001 | open | runtime image 內容變更不要求 `IMAGE_VERSION` bump。 | CI 檢查非文件變更必須同步 version。 |

### P3

| ID | 狀態 | 問題與主要證據 | 完成條件 |
| --- | --- | --- | --- |
| YAGNI-API-STUB-001 | open | Python generated stub 是 runtime dependency，但產品碼不使用。 | 降為 test dependency或由 runtime boundary 真正使用；保留有效 conformance test。 |
| FE-A11Y-FOCUS-001 | open | 取消刪除確認後焦點不回 trigger。 | Ask→Cancel 後 activeElement 回原按鈕。 |
| RUN-DB-STATE-001 | open | DB transition 只驗 from，不驗合法 pair。 | DB guard 拒絕非法 non-terminal transition。 |
| OUTBOX-CLAIM-001 | open | 多 publisher 可同時取到未發佈事件。 | claim/`SKIP LOCKED`，保留 crash-before-mark 的 at-least-once。 |
| TRACE-SEQ-001 | open | `max(seq)+1` 無鎖也無 unique；transaction 本身不能序列化。 | unique＋retry或 per-stream counter/lock；並發寫入唯一連續。 |
| TEST-LOCAL-SIGNAL-001 | open | `task test` 自稱 all suites，但沒 DB URL 時核心 integration tests skip。 | unit/integration 分名；full test 對缺 DB fail-closed。 |
| TEST-QA002-001 | open | mutated import corpus 預設 skip 且無 CI lane。 | 最小 pinned fixture 進 PR lane，完整 corpus可排程。 |
| CI-TASKFILE-001 | open | CI 只用自製 scanner，未解析 Taskfile syntax/graph。 | pinned Task 執行 `--list`/`--summary`；壞 YAML/deps 會紅。 |
| O11Y-PROMTOOL-001 | open | Prometheus rules 只有歷史手動驗證，無持續 promtool gate。 | pinned promtool 檢查納入 CI。 |
| SUPPLY-SCANNER-DIGEST-001 | open | Syft/Grype 用 mutable tag，且 Syft 掛 Docker socket。 | scanner image 使用 digest。 |
| SUPPLY-CI-INSTALL-001 | open | workflow 中裸 `pip install`／`npx --yes` 繞過 lock。 | validators 進 locked env/container；CI lint 禁止未鎖安裝。 |
| TEST-RACE-001 | open | 無 Go race/fuzz lane；本機 Windows 又因 CGO 未啟用不能跑 race。 | Linux scheduled/manual race lane；先覆蓋 platform/sandbox。 |
| SUPPLY-ACTIONS-SHA-001 | open | GitHub Actions 只 pin major tag。 | pin commit SHA並用自動更新工具維護。 |
| DEV-COMPOSE-BIND-001 | open | Postgres/SeaweedFS dev ports 綁所有介面且用靜態 dev credential。 | 明確 bind `127.0.0.1`。 |
| SBX-DOC-DRIFT-001 | open | Sandbox README 仍寫舊 image 版本與 pipeline 未接。 | 文件與 `2026.08-3`／現行 pipeline 一致。 |

## 4. 已排除與不重複項

- Docker `GO-2026-4887`／`GO-2026-4883` 是 Engine daemon AuthZ/plugin 路徑；本 repo 只用 Docker client container lifecycle/exec API，沒有 vulnerable server/plugin call path，不列可利用漏洞。
- Sandbox contract 目前雖手寫，但 schema、handler 與 provider types 實際一致；「未 codegen」本身不是缺陷。
- Ogen 只掛 `/healthz` 是刻意保留 per-route AuthZ，不是漏掛。
- FastAPI top-level request 未 `extra=forbid` 與 OpenAPI 現況一致；nested dataset boundary 已 forbid。
- `source.url` 在 UI 原樣作 href，但正常寫入路徑只接受 allowlisted HTTPS；列 defense-in-depth，不列漏洞。
- Query key 未含 workspace 在目前 full-page OAuth/session lifecycle 下未證實跨 Workspace 洩漏；新增 SPA account switching 時再納入。
- `llmclient` 本身不設 timeout不是問題；其既有 caller 多有 deadline。`HTTP-TIMEOUT-001` 僅指確實沒有 deadline 的 public outbound paths。
- Moby scanner module/init trace、未採最新版 dependency、已明文且未到期的 CVE 風險接受，不單獨算 finding。
- Local Runner、remote MCP、Kubernetes、service mesh 不在 MVP；不以「尚未做」當技術債。

## 5. Ponytail／過度設計結論

- `yagni:` Python generated stub 不應作 runtime dependency；保留測試期 contract model 即可。`apps/llm/pyproject.toml`
- `delete:` Web 未使用的 generated API factory 在真正遷移第一個 endpoint 前沒有產品價值；若本輪不立即遷移就刪除 factory，保留 generated package。`apps/web/src/api/generated.ts`
- 其餘分離平面（Platform、LLM、Sandbox）、Transactional Outbox、immutable versions 與 generated ownership皆由既有信任／一致性需求正當化，未找到可安全刪除的大型架構層。

估計可直接淨減：`-10~20` 行、`-1` runtime dependency；主要修復會增加測試與邊界檢查，目標不是追求負 LOC。

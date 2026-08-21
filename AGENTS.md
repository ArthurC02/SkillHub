# Skill Hub — Coding Agent 導覽

## 這是什麼專案

Skill Hub 是 Agent Skill 的搜尋引擎與試驗室：個人創作者以自然語言描述任務 → 探索候選 Skill → 用自己的 Prompt 與測試資料在隔離 Sandbox 試跑 → 依 Trace 與評估報告改善 → 下載符合 Agent Skills 規格的可攜套件。

## 目前狀態（2026-08-18）

**M4 打包與封測的程式面已收斂；M0～M3 亦已收斂。MVP 尚未完成——剩下的是部署期與負責人動作，不是程式。** 對帳見 [docs/plans/mvp/m4/audit.md](docs/plans/mvp/m4/audit.md)（49 項：**11 勾選、38 誠實不勾、零項退回**；**2026-08-18 稍晚的後端批後為 14 勾選、35 不勾**，見該檔 §7 補記；**同日的 UI 批再加 `DESIGN-012` 與 `WS-004` 兩項，為 16 勾選、33 不勾**，見 §8 補記）、[m3/audit.md](docs/plans/mvp/m3/audit.md)（16 項）與 [m2/m2-work-items-audit.md](docs/plans/mvp/m2/m2-work-items-audit.md)（41 項）。

- **M4 交出去的是一條完整的打包與下載路徑**：不可變版本 → 三個目標的 zip → 對**交出去的位元組**重跑匯入路徑再驗一次 → 四道鎖（人工 hold／不可散布／授權未知／驗證未過）任一即拒 → 短效授權下載 → 下載紀錄與稽核事件同交易。封測面另有准入閘門、配額強制點、四個漏斗事件、回饋端點與 P1 停派送開關。**勾選的 11 項與不勾的 38 項各自的理由逐條寫在 `03` 的行內**。
- **「程式面收斂」不等於「MVP 完成」**：`03` §18 的 `RELEASE-001`～`010` **十項全部誠實不勾**。共同的三個阻擋是**甲類四項未到期**、**六項 PDM 未追認**、**M1 閘門 D 日未宣告**。逐項的「誰做什麼驗什麼」見 [docs/plans/mvp/m4/release-checklist.md](docs/plans/mvp/m4/release-checklist.md)。
- **M1 驗證閘門仍未正式通過**：材料已備妥（[docs/plans/mvp/gate-test/](docs/plans/mvp/gate-test/)），**D 日仍待負責人宣告**。**M4 與前三個里程碑不同——封測不能與閘門並行**（三個理由見 [m4/README.md §5.3](docs/plans/mvp/m4/README.md)），打包批可以。同意書草稿已補（[gate-test/consent-and-data-policy.md](docs/plans/mvp/gate-test/consent-and-data-policy.md)，**待法務確認**），它是閘門與封測共用的前置。
- **ADR-020～029 已入列**（身分／Session、License 溯源、Sandbox 部署拓撲與安全定值、Agent SDK 版本釘選、頂層目錄分「跑的」與「讀的」；Run 終態與 Evaluation 判定分離、重評 append-only 與 LLM Judge 四條防線；**M4 的三份**：Download Artifact 的雙雜湊與「MVP 明文不簽章」、封測准入與配額強制點、產品分析事件的邊界）。**M4 沒有新增 ADR-030 之後的任何一份**——三份在文件批就寫完了。
- **殘項三類清單**（甲＝部署期驗收、乙＝待負責人決策、丙＝實作或資料工作）見 [docs/plans/mvp/04-backlog-and-handoffs.md](docs/plans/mvp/04-backlog-and-handoffs.md)（**活文件**，隨時更新）——**開工前先看那份，它記的是「已定值但沒有強制」與「已量到但沒查根因」那一類洞**。目前**甲 4 項、乙 6 項、丙 17 項**（2026-08-18 M4 對帳批後：乙新增「同意書待法務確認」，丙新增十二項並結案丙-9／丙-10）。**十七項丙類沒有一項在等決策**，逐項補法見 [m4/release-checklist.md §1.9](docs/plans/mvp/m4/release-checklist.md)。
- **「移交 M4」六條接點已逐項裁定**：四條關閉（`PACK-002` 重用驗證路徑、衍生關係的溯源方向、可攜 Test Case 的兩半、評估產物由白名單排除），兩條轉殘項（乙-13 的 G7／G8、乙-14 的甲類到期）。**MVP 之後的接點另起一節**（`04` §移交下一階段）。

| 目錄 | 內容 | 入口 |
| --- | --- | --- |
| `docs/plans/mvp/` | 產品基準：目標、規格允收準則（需求 ID）、工作清單、[殘項與移交](docs/plans/mvp/04-backlog-and-handoffs.md)（活文件）；`m0/`～`m4/` 為各里程碑凍結產出；`content/`／`governance/`／`gate-test/` 為跨里程碑仍在被引用的主題目錄（ADR-031） | [docs/plans/mvp/README.md](docs/plans/mvp/README.md) |
| `docs/adr/` | 36 份架構決策紀錄（ADR-000～035；014 已由 018 取代，024 已由 031 取代，019 §1 現由 031 修訂） | [docs/adr/README.md](docs/adr/README.md)（含索引與架構總圖） |
| `docs/spikes/` | **已刪除，只留墓碑**：M0 驗證用 spike code，結論已沉澱到 m0 報告／ADR-013／ADR-023／`UPGRADES.md`／`tools/goldenset/` | [docs/spikes/README.md](docs/spikes/README.md)（含還原指令與結論落點對照） |

Monorepo 的 CI/CD 基線見 **ADR-019（Proposed）**，頂層收納現由 **ADR-031（Accepted）** 按產物角色定義；它取代 ADR-024 的 `apps/`／`services/` 雙軌。結構性偏離需先更新 ADR。

| 頂層目錄 | 收納語意 |
| --- | --- |
| `apps/` | 可獨立啟動、建置或部署的產品程式：`web`、`platform`、`llm`、`sandbox` |
| `packages/` | 供其他程式 import 的 library；generated 子目錄禁止手改 |
| `contracts/` | 跨程序／跨語言介面的唯一來源 |
| `db/` | Migration、query、sqlc config 與 DB test 等持久化來源 |
| `infra/` | 部署、runtime image、網路、節點與 observability 設定 |
| `tools/` | 開發、CI、資料維護、維運命令及其緊密 fixture |
| `docs/` | 不被產品程式 import 或執行的敘事與歷史文件 |

`.github/`、`.devcontainer/` 與 repo 根層工具入口保留在平台預期位置。`services/` 已停用；不是所有含程式碼的內容都進 `apps/`，判準是能否作為產品程式獨立啟動／建置／部署。

## 已定案的技術棧速覽

| 層 | 選擇 | 依據 |
| --- | --- | --- |
| 前端 | React + TS（Vite、TanStack Router/Query），SPA 起步 | ADR-016 |
| 平台後端 | Go：chi/echo 薄層、pgx + sqlc、River（Postgres 佇列） | ADR-016、014 |
| LLM 工作負載 | Python：FastAPI + LangGraph（uv 管理），內部服務 | ADR-016 |
| 模型供應商 | OpenAI API（試跑預設 mini 級；Embedding `text-embedding-3-small`），一律經 LiteLLM 閘道 | PDM-003、ADR-017 |
| 資料 | PostgreSQL 中心（交易、FTS + pgvector、佇列、Trace 分割表）＋受管 S3 相容物件儲存；核心元件容器化自架（E1） | ADR-018 |
| 搜尋 | 混合檢索（向量腿承載跨語言召回，FTS＋RRF 為召回覆蓋）＋索引時 LLM 增強（摘要與任務範例句為必要項） | ADR-013 |
| Agent Runtime | Claude Agent SDK **0.3.233**，以 digest ＋ lockfile 釘選；升級必須重跑四項實測，靜默失效不得以推理帶過 | ADR-023 |
| 身分與 Session | GitHub OAuth ＋ Postgres Session（`DEV_LOGIN` 為離線 provider） | ADR-020 |
| Sandbox 隔離 | gVisor 基線（`systrap` 平台，不需巢狀虛擬化），獨立 VM 池，沙箱層 nftables default-deny ＋固定 DNS（不部署 L7 Proxy） | ADR-015、005、022 |
| Runtime Image | 自建映像發佈至 **GHCR**，SBOM 與漏洞掃描以 attestation 隨 digest 保存；過不了門檻的映像到不了 registry | ADR-022、`03` SBX-011 |
| 模型出口 | LiteLLM Proxy（唯一模型閘道，每 Run 短效 Virtual Key） | ADR-017 |
| LLM 觀測 | Langfuse Cloud（工程調優專用，非事實來源） | ADR-017 |
| 契約 | OpenAPI-first，Go 為 spec 來源，codegen 產 TS/Python stub | ADR-016 |

範圍注意：**Local Runner 與遠端 MCP 已移出 MVP 首發**（架構決策保留於 ADR-006 與相關規格，實作依需求訊號啟動）。

## 實作鐵律（違反任何一條 = 架構回歸）

1. 不受信任的 Skill、Script、資料不得在 Web/API 程序內執行；匯入與掃描階段不得執行套件內 Script。（ADR-001、007）
2. 執行平面不得直接存取核心資料庫；只透過任務契約、短效物件授權與事件互動。（ADR-001）
3. 所有使用者資料查詢預設要求 Workspace Scope；不信任 UI 傳入的 `workspace_id`。（ADR-011）
4. Skill Version、Test Case 快照、歷史 Run 不可變；採用改善建議＝建立新版本，不原地覆寫。（ADR-003）
5. Run 狀態的唯一事實來源是 Go 擁有的 Postgres 狀態機；LangGraph 只是單次 Job 內的程序內編排，其 checkpoint 是暫存草稿。（ADR-008、016）
6. Python 是能力提供者：收結構化請求、回結構化結果；政策、授權、狀態轉移、重試決策全在 Go，業務規則不進 Python。（ADR-016）
7. 佇列消費者只有 Go Worker；Python 不消費佇列，由 Go 以內部 HTTP 呼叫（含逾時與取消傳遞）。（ADR-016）
8. 所有模型呼叫走 LiteLLM 閘道，不得直連供應商；供應商金鑰只存在閘道。（ADR-017）
9. 領域狀態變更與對外事件同交易（Transactional Outbox）；Consumer 必須冪等；`destroy`/清理可安全重複。（ADR-008、004）
10. 平台 `run_id` 是永久識別；Provider 臨時 ID 不得當主鍵或永久 URL。（ADR-004）
11. Secrets 不得出現在套件、Log、Trace 明文或分析事件；顯示前完成遮罩。（NFR-002、TRACE-001）
12. 跨語言介面先寫 OpenAPI schema 再實作；CI 以 codegen 檢查 drift。（ADR-016）

## 文件維護規則

- 三份 MVP 文件（目標／規格／工作清單）改範圍時必須同步；規格新功能先補需求 ID 與允收準則。
- 工作項目 `- [ ]` → `- [x]` 只在完全符合允收準則時；部分完成保持未勾。
- ADR 是決策歷史：推翻舊決策＝新增 ADR 並把舊的標 `Superseded`，不刪除、不原地改寫決策內容。
- 新 ADR 從 **ADR-036** 起編；選型類決策採 ADR-016 格式（含「評估選項」比較），邊界類可用精簡格式。
- ADR 的待決策被後續 ADR 回答時，回填 `→ [ADR-xxx](...)` 引用（現有文件已有此慣例）。
- 新 ADR 記得更新 [docs/adr/README.md](docs/adr/README.md) 的決策索引。
- **檔案放哪裡**：活文件放 `docs/plans/mvp/` 根層（編號 `01~`）；里程碑的歷史產出放 `mX/`，里程碑完結即凍結。一份文件如果會被下一個里程碑繼續改，它就不屬於 `mX/`。
- **里程碑目錄固定骨架**：`README.md`（計畫＋狀態＋檔案地圖）、`audit.md`、報告用 `report-*` 前綴；目錄內檔名不重複 `mX` 前綴（路徑已經說了）。**M3 起適用，既有檔名不回溯改。**

## 慣例

- 文件語言：繁體中文（保留 Run、Workspace、Provider 等英文術語不硬翻）。
- 多人／多 agent 共用同一工作樹平行作業時：只以明確 pathspec stage 自己的檔案、push 前 `git pull --rebase`；**禁止 `git stash`**（stash 會連他人未提交與未追蹤的工作一起收走，本專案已三度因此出事）；暫存產物放 scratchpad，不放 repo 根目錄。
- 程式碼、識別字、commit message：英文。
- 里程碑：M0 基線 → M1 Explorer（結尾有驗證閘門，不通過不進 M2）→ M2 Lab → M3 評估 → M4 打包與封測。
- 需求 ID 前綴：DISC／SKILL／WS／TEST／RUN／SBX／TRACE／EVAL／PACK／NFR／PDM／SEC 等，見 `docs/plans/mvp/02`、`03`。

## 開發自動化（Agent 開工先讀）

本節是人類與 Coding Agent 的共同入口；詳細決策見 [ADR-030](docs/adr/ADR-030-portable-developer-automation-and-contract-code-generation.md)，完整操作與排錯見 [開發自動化手冊](docs/development/automation.md)。不要把只在某一台電腦成立的 native command 當成 repo 的標準流程。

1. **先診斷再修改**：進入 repo 後先看 `task --list`，再跑 `task doctor`。新電腦尚未安裝 Task 時，用 `go -C tools/devctl run . doctor`；版本來源是各語言原生檔與 `tools/toolchain.yaml`，不是 Agent 記憶。
2. **初始化不覆寫秘密**：`task env:init` 只在 `.env` 不存在時由 `.env.example` 建立；不得把真實 key 寫入 `.env.example`、Log、Trace 或回覆。`task bootstrap` 只下載各語言依賴。
3. **預設不花錢**：`task dev`／`task dev:core` 只啟動 Postgres 與 SeaweedFS，不需要模型金鑰；`task dev:model` 會先 fail-closed 檢查模型秘密，且後續操作可能產生費用，不得由唯讀 SubAgent 自行啟動。
4. **共享工作樹、單一 Writer**：目前不要求 SubAgent 使用 worktree。SubAgent 預設唯讀；同一時間只能有一個 Writer。寫入 SubAgent 必須有精確 path allowlist，不得自行執行 repo-wide formatter、package install、Compose down、Git 寫入或 lockfile 更新。
5. **保護他人變更**：除既有的禁止 `git stash` 外，也禁止對未知修改執行 `git reset`、`git clean` 或 `git checkout -- <path>`；看到不屬於自己的 delta 就保留並回報。只以明確 pathspec stage 本批檔案。
6. **高衝突區由主 Agent 序列化**：`contracts/`、`db/migrations/`、`db/queries/`、generated 目錄、`go.sum`／`package-lock.json`／`uv.lock`、`Taskfile.yml` 與 `.github/workflows/` 不交給多個寫入 Agent 平行處理。
7. **平台限制要誠實**：Dev Container 是跨電腦的建議路徑，但不取代 SEC-009 的真實 Linux／gVisor 部署驗收。Doctor 的版本不符是環境診斷，不得靠跳過檢查偽裝成通過。
8. **generated files 禁止手改**：修改 `db/migrations/**`、`db/queries/**` 或 `db/sqlc.yaml` 後，由主 Agent序列化執行 `task gen:sql`；修改 `contracts/openapi/public.yaml`／`llm-internal.yaml` 後執行 `task gen:openapi`。提交前一律跑 `task gen:check`。`apps/platform/internal/platform/db/gen/**`、`apps/platform/internal/api/gen/**`、`packages/api-client-ts/src/generated/**`、`packages/api-stub-py/src/skillhub_api_stub/generated/**` 的衝突要在來源解決後重生，不得手動合併。`task gen` 有 repo-local lock、暫存輸出與原子替換；SubAgent 不自行執行。
9. **transport type 不是產品政策**：Web 既有 `apps/web/src/api/types.ts` 是 UI view model，依 endpoint 逐一用 adapter 遷移，不得因 generated client 存在就整檔替換；Python generated models 只描述內部 HTTP payload，授權、政策、重試與狀態轉移仍在 Go（鐵律 6）。生成器與映像版本只從 `tools/toolchain.yaml` 讀。
10. **Go generated router 不擁有 AuthZ**：Phase 4 只把 ogen server 放在 `router.go` 的精確 `GET /healthz` pattern 後。其他 route 仍逐條由 `router.go` 套 `RequireSession`／`RequireOperator`／`OptionalSession`；不得直接 mount 完整 generated server、不得讓 `UnimplementedHandler` 的其他 operation 對外可達。每移一條 endpoint 都要保留原 middleware 語意並加 route 測試。
11. **Platform 的 Bounded Context 治理（ADR-032，2026-08-20 起生效）**：`apps/platform/internal/` 每個套件屬於且僅屬於一個 context；新增套件必須先在 ADR-032 §1 對照表登記。跨 context 的新 import 必須**同一個 commit** 改 ADR-032 附錄 A 與 `apps/platform/.golangci.yml` 的 depguard 規則（CI 以 depguard＋`devctl automation-check` 兩道強制）；領域 Service 一律由 `apiserver.NewApp` 注入，禁止方法內現場建構其他 context 的 Service。速查對照（事實來源是 ADR-032 §1）：

    | Context | 套件 | 需求 ID |
    | --- | --- | --- |
    | Identity & Workspace | `identity` | WS、SEC |
    | Catalog & Discovery | `catalog` | DISC |
    | Skill Registry & Versioning | `registry`、`skillpkg` | SKILL |
    | Trust & Supply Chain | `ingest`（含版本寫入的唯一驗證路徑） | SKILL、SEC |
    | Test Lab | `testlab` | TEST |
    | Run Orchestration | `run` | RUN、SBX |
    | Evaluation & Improvement | `eval` | EVAL |
    | Packaging & Distribution | `packaging` | PACK |
    | Run Trace | `trace` | TRACE |
    | Policy & Usage | `policy`（quota 與 retention 規則）、`analytics`（漏斗量測） | PDM、NFR |
    | Generic（無領域規則） | `audit`、`outbox`、`objreconcile`、`llmclient`、`skillpkg`、`platform/*`、`apiserver`、`api/gen` | — |

12. **Query ownership（ADR-033，2026-08-20 起生效）**：sqlc 把全部 query 生成到同一個 package，depguard 看不到。每條 query 的 owner context 宣告在 `db/query-owners.yaml`，由 `devctl automation-check` 強制：**跨 context 的 write query 呼叫會 FAIL**（read 只宣告不擋）。新增或刪除 `db/queries/*.sql` 的 query，同一批要改 `db/query-owners.yaml`，漏了 CI 會 FAIL。該檔的 `allow:` 是**存量漂移清單，不是擴充點**——新的跨 context 寫入要改程式，不准往下面加行。

## 快速判斷「我該看哪份文件」

| 你要做的事 | 先看 |
| --- | --- |
| 理解產品範圍與里程碑 | `docs/plans/mvp/01-goals-and-plan.md` |
| 查某功能的允收準則 | `docs/plans/mvp/02-specifications-and-acceptance-criteria.md`（按需求 ID） |
| 找下一個工作項目 | `docs/plans/mvp/03-work-items.md`（章節已標里程碑） |
| 理解系統邊界與平面 | ADR-001、002 |
| 資料模型與儲存 | ADR-003、018 |
| Monorepo 結構與 CI/CD | ADR-019、031 |
| Run 生命週期與 Provider 契約 | ADR-004、008 |
| 安全與信任 | ADR-005、007、015；部署拓撲與安全門檻定值見 ADR-022 |
| Query 屬於哪個 context、能不能直接讀寫 | ADR-033（write）＋ADR-035（read、context 對照表對帳）＋`db/query-owners.yaml` |
| 目前還缺什麼、誰在等誰 | `docs/plans/mvp/04-backlog-and-handoffs.md`（殘項三類清單＋跨里程碑待辦） |
| 封測上線前要做什麼、誰做 | `docs/plans/mvp/m4/release-checklist.md`（程式面尚缺／部署期／負責人動作三段） |
| 受測者同意書與資料保存政策 | `docs/plans/mvp/gate-test/consent-and-data-policy.md`（**草稿，待法務確認**；閘門與封測共用） |
| 語言分工與跨語言守則 | ADR-016 |
| 模型呼叫與成本 | ADR-017 |
| 評估判定、重評與 Judge 邊界 | ADR-025、026 |
| 打包下載的雜湊、簽章與可散布性 | ADR-027（＋ ADR-012、021） |
| 封測准入、免費額度強制點 | ADR-028（值由 PDM-010／PDM-009 給） |
| 漏斗量測、分析事件與 audit 的分野 | ADR-029（＋ ADR-009） |
| 目前所有未決議題 | 各 ADR 的「待決策」章節＋ `docs/plans/mvp/03` 第 1 節 |

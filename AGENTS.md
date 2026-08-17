# Skill Hub — Coding Agent 導覽

## 這是什麼專案

Skill Hub 是 Agent Skill 的搜尋引擎與試驗室：個人創作者以自然語言描述任務 → 探索候選 Skill → 用自己的 Prompt 與測試資料在隔離 Sandbox 試跑 → 依 Trace 與評估報告改善 → 下載符合 Agent Skills 規格的可攜套件。

## 目前狀態（2026-08-17）

**M3 評估與改善完結，程式面已收斂；M0／M1／M2 亦已收斂。** 對帳見 [docs/plans/mvp/m3/audit.md](docs/plans/mvp/m3/audit.md)（16 項：13 勾選、1 退回後同日補回、2 已勾覆核後維持）與 [docs/plans/mvp/m2/m2-work-items-audit.md](docs/plans/mvp/m2/m2-work-items-audit.md)（41 項：33 維持勾選、1 退回後補回、7 誠實不勾）。

- **M3 對帳唯一退回的 `EVAL-011` 已於 2026-08-17 補上並勾選**：採納建議建出新版本後，`AppliedResult` 直接把三個 id 連到執行前權限確認畫面（`04` 丙-11 已結案，見 [m3/audit.md §2.1](docs/plans/mvp/m3/audit.md)）。**preflight 頁仍然沒有版本選單**，那屬 `DESIGN-007`。
- **M1 驗證閘門仍未正式通過**：使用者測試材料已備妥（[docs/plans/mvp/gate-test/](docs/plans/mvp/gate-test/)），**D 日仍待負責人宣告**；閘門與 M2／M3 並行至今，通過與否以測試結果為準。
- **ADR-020～029 已入列**（身分／Session、License 溯源、Sandbox 部署拓撲與安全定值、Agent SDK 版本釘選、頂層目錄分「跑的」與「讀的」；Run 終態與 Evaluation 判定分離、重評 append-only 與 LLM Judge 四條防線；**M4 的三份**：Download Artifact 的雙雜湊與「MVP 明文不簽章」、封測准入與配額強制點、產品分析事件的邊界）。
- **殘項三類清單**（甲＝部署期驗收、乙＝待負責人決策、丙＝移交下一里程碑的接點）見 [docs/plans/mvp/04-backlog-and-handoffs.md](docs/plans/mvp/04-backlog-and-handoffs.md)（**活文件**，隨時更新）——**開工前先看那份，它記的是「已定值但沒有強制」與「已量到但沒查根因」那一類洞**。目前**甲 4 項、乙 5 項、丙 6 項**（2026-08-17 M4 文件批後：乙新增「甲類四項是否在封測前到期」與「PDM-009 封測提案待追認」，丙新增「preflight 沒有版本選擇器」）；原移交 M3 的丙類七項已全數結案。
- 下一個里程碑是 **M4 打包與封測**；M4 的接點（`PACK-002` 可重用 `EVAL-010` 的驗證路徑、衍生關係的溯源方向、評估產物不得進套件、封測與甲類的關係）已在 `04` 的「移交 M4」一節逐項寫明。**M4 的計畫與三份設計文件已產出**（[docs/plans/mvp/m4/](docs/plans/mvp/m4/)），**程式面未開工**；文件批已於 2026-08-17 補齊三份 ADR、`02`／`03` 同步與 PDM-009 提案，**第 1 批剩餘的前置決策只有 PDM-008 的追認**。

| 目錄 | 內容 | 入口 |
| --- | --- | --- |
| `docs/plans/mvp/` | 產品基準：目標、規格允收準則（需求 ID）、工作清單；`m0/`／`m1/`／`m2/` 為各里程碑凍結產出；`content/`／`governance/`／`gate-test/` 為跨里程碑仍在被引用的主題目錄（ADR-024） | [docs/plans/mvp/README.md](docs/plans/mvp/README.md) |
| `docs/adr/` | 30 份架構決策紀錄（ADR-000～029；014 已 Superseded by 018，019 §1 已 Amended by 024） | [docs/adr/README.md](docs/adr/README.md)（含索引與架構總圖） |
| `docs/spikes/` | **已刪除，只留墓碑**：M0 驗證用 spike code，結論已沉澱到 m0 報告／ADR-013／ADR-023／`UPGRADES.md`／`tools/goldenset/` | [docs/spikes/README.md](docs/spikes/README.md)（含還原指令與結論落點對照） |

Monorepo 目錄結構與 CI/CD 已提案於 **ADR-019（Proposed）**，其第 1 節已由 **ADR-024** 修訂為「跑的」（`apps/`、`services/`、`contracts/`、`db/`、`infra/`、`tools/`）與「讀的」（`docs/`）兩類——鋪程式碼依其結構進行，結構性偏離需先更新 ADR。

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
- 新 ADR 從 **ADR-030** 起編；選型類決策採 ADR-016 格式（含「評估選項」比較），邊界類可用精簡格式。
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

## 快速判斷「我該看哪份文件」

| 你要做的事 | 先看 |
| --- | --- |
| 理解產品範圍與里程碑 | `docs/plans/mvp/01-goals-and-plan.md` |
| 查某功能的允收準則 | `docs/plans/mvp/02-specifications-and-acceptance-criteria.md`（按需求 ID） |
| 找下一個工作項目 | `docs/plans/mvp/03-work-items.md`（章節已標里程碑） |
| 理解系統邊界與平面 | ADR-001、002 |
| 資料模型與儲存 | ADR-003、018 |
| Monorepo 結構與 CI/CD | ADR-019 |
| Run 生命週期與 Provider 契約 | ADR-004、008 |
| 安全與信任 | ADR-005、007、015；部署拓撲與安全門檻定值見 ADR-022 |
| 目前還缺什麼、誰在等誰 | `docs/plans/mvp/04-backlog-and-handoffs.md`（殘項三類清單＋跨里程碑待辦） |
| 語言分工與跨語言守則 | ADR-016 |
| 模型呼叫與成本 | ADR-017 |
| 評估判定、重評與 Judge 邊界 | ADR-025、026 |
| 打包下載的雜湊、簽章與可散布性 | ADR-027（＋ ADR-012、021） |
| 封測准入、免費額度強制點 | ADR-028（值由 PDM-010／PDM-009 給） |
| 漏斗量測、分析事件與 audit 的分野 | ADR-029（＋ ADR-009） |
| 目前所有未決議題 | 各 ADR 的「待決策」章節＋ `docs/plans/mvp/03` 第 1 節 |

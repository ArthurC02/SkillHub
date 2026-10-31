# Coding Agent 分層與派工

這份文件說明 Agent 如何取得規則與如何被派工。它補充根 [`AGENTS.md`](../../AGENTS.md)；區域卡不得放寬根共同安全規則，遇衝突必須停下回報，權威規格決定允收內容。官方對 `AGENTS.md` 的啟動載入說明見 [Agent configuration](https://learn.chatgpt.com/docs/agent-configuration/agents-md)；本文件把官方啟動載入與派工介面的工具契約分開記錄。

## 技術棧

| 層 | 選擇 | 依據 |
| --- | --- | --- |
| 前端 | React + TS（Vite、TanStack Router/Query） | [系統情境、平面與部署路徑](../adr/README.md#系統情境平面與部署路徑) |
| 平台後端 | Go：薄 HTTP 層、pgx + sqlc、River | [系統情境、平面與部署路徑](../adr/README.md#系統情境平面與部署路徑)、[資料所有權與核心基礎設施](../adr/README.md#資料所有權與核心基礎設施) |
| LLM 工作負載 | Python FastAPI（uv），內部服務 | [系統情境、平面與部署路徑](../adr/README.md#系統情境平面與部署路徑) |
| 模型供應商 | OpenAI（試跑預設 mini 級；Embedding `text-embedding-3-small`），一律經 LiteLLM，每 Run 短效 Virtual Key | [模型閘道與可觀測性](../adr/README.md#模型閘道與可觀測性) |
| 資料 | PostgreSQL 中心 ＋ S3 相容物件儲存；核心元件容器化自架 | [資料所有權與核心基礎設施](../adr/README.md#資料所有權與核心基礎設施) |
| 搜尋 | 混合檢索（向量腿 ＋ FTS 腿 `UNION` 擴充候選，不做 RRF）＋ 索引時 LLM 增強 | [意圖搜尋](../adr/README.md#意圖搜尋) |
| Agent Runtime | Claude Agent SDK，事實來源是 image digest；版本字串釘在 `infra/images/runtime-agent-sdk/Dockerfile` 的 `ARG`，**不在 `tools/toolchain.yaml`**；升級必重跑四項實測 | [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全) |
| 身分 | GitHub OAuth ＋ Postgres Session（`DEV_LOGIN` 為離線 provider） | [身分、Workspace、准入與額度](../adr/README.md#身分workspace准入與額度) |
| Sandbox | gVisor `systrap`，獨立 VM 池，nftables default-deny、沙箱沒有 DNS，不部署 L7 Proxy | [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全) |
| Runtime Image | 自建映像發佈至 GHCR，SBOM 與掃描以 attestation 隨 digest 保存 | [Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全) |
| LLM 觀測 | **不外接第三方服務**（[`05` R-24](../plans/05-pending-rulings.md) 取 (b)）：花多少看 `cost_events`、發生了什麼看 Trace 分割表、品質有沒有退步看 `tools/eval-regression`。**Langfuse 不做**——回呼那半邊要在閘道之外多存一把金鑰，撞鐵律 11 | [模型閘道與可觀測性](../adr/README.md#模型閘道與可觀測性) |
| 互動創作 | Python LangGraph 分階段編排、Go／Postgres 會話快照與事件；已接線，曝光與品質驗收仍待核准，見[開發與驗證](interactive-creation.md) | [互動創作](../adr/README.md#互動創作) |
| 契約 | OpenAPI-first；Go 側 models-only，handler 手寫並逐條對齊 | [系統情境、平面與部署路徑](../adr/README.md#系統情境平面與部署路徑)、[開發自動化與依賴治理](../adr/README.md#開發自動化與依賴治理) |

**Local Runner 與遠端 MCP 已移出 MVP 首發**（Local Runner 的決策見[系統情境、平面與部署路徑](../adr/README.md#系統情境平面與部署路徑)；遠端 MCP 的範圍在 [`01` §8](../plans/01-goals-and-plan.md)）。

## 三層指示

- **根共同規則**：根 `AGENTS.md` 的鐵律、共享工作樹、付費／生成限制、⛔ 曝光邊界與開發自動化紅線，適用整個 repo。
- **區域卡**：目標檔案所在的父目錄 `AGENTS.md`（以及需要時的 `CLAUDE.md`／`.claude/rules/`），說明該區域先讀什麼、允許改什麼與哪個閘門會攔截。
- **權威規格**：需求與允收讀 `docs/plans/`，架構決策讀 `docs/adr/`，設計規範讀 `docs/design/`；程式契約則讀對應 `contracts/` 或程式原生來源。

任何代理讀或改目標前，沿著 **repo root → 目標父目錄** 明確讀完各層 `AGENTS.md`，並依路由要求讀權威規格；跨區工作要讀每個涉及區域的指示。不要假設工具會自動載入，也不要把規則只存在 brief 裡。

## 派工流程

主代理與子代理都適用：

1. 先確認工作樹中的未知 delta 並保留。
2. brief 必須寫清楚任務、精確 path allowlist、必讀指示、驗證命令／允收條件與模型等級；子代理模型從能完成任務的最低等級起，禁止派旗艦級；推論深度用 effort 調，並與模型一起寫明。
3. 共享工作樹只有一個 Writer；未列入 allowlist 的檔案不可改。子代理不自行 Git 寫入、安裝、生成、formatter 或啟停共享 Compose。
4. 變更後按 brief 驗證；若 brief 與程式碼或權威規格不一致，停下來回報，以程式碼／權威規格為準，不自行推進。

官方啟動載入是從 root 到目前 cwd 的層級指示；若存在 `AGENTS.override.md`，必須先讀它並明確檢查與 `AGENTS.md` 的差異／衝突，不得悄悄略過根安全限制。若派工工具沒有 `cwd` 參數，代理共享目前工作目錄；其他介面應以其工具契約確認，不從任務路徑推定啟動目錄。不要承諾工具會按檔案自動載入指示，必須在 brief 中列出讀取路徑並要求回報。

指示來源變更後，停止依舊版指示的新派工；由唯一 Writer 同步來源並重新讀取，再重新派送。已啟動的代理不會自動重載變更。

## 跨工具任務流程契約

以下是跨工具都能重做的流程，不是某個 runtime 的命令語法。先分類任務，再依工具能力用原生派工組裝同一流程；沒有對應 runtime 時不可直接 `node .claude/workflows/*.js`，也不可把腳本存在當成已執行或已獨立驗證。`.claude/workflows/` 是 Claude workflow 的一種實作；它不會由 `tools/devctl/agent_sync.go` 生成，且不替其他工具提供 runner 保證。

### 直接問答／小任務

輸入是問題、目標檔案或一個小而明確的查證。主代理先判斷範圍，讀 root → 目標父目錄指示與必要的權威來源，再回答證據、假設與限制。若需小型明確修改，可由主代理依 allowlist 直接完成並驗證；較大或需協作的修改才建立下列寫入 brief，不把回答授權推廣成寫入授權。

### `ux-text-audit`

輸入是明確的頁面群組、scope 與 exclusions（可指定檔案；未指定時仍須核對腳本的現況清單與排除理由），以及設計規範。排除的檔案不等於已覆蓋。階段依序為：

1. **Read**：每組一名唯讀讀者，逐檔讀完，列出所有可見文字、rune 計數、分類、D 區塊、Tip／icon／title 候選與未確定項。
2. **Refute**：至少三個唯讀角度重新開檔，攻擊 C／D 邊界、Tip 六條件與計數；反駁結果須逐項保留。
3. **Critic**：列出未讀檔案、未覆蓋頁面及只能由 fixture／伺服器字串補足的內容。

證據是讀者結構化結果、反駁結果與 critic 缺口；它是稽核報告，不是修改授權，完成不得只以 gate 判定。停止條件是所有輸入群組都有結果，未讀／不確定／被反駁項已明列；缺任何一項就回報未完成。

### `error-path-audit`

輸入是一條功能線的 Go handler、契約、頁面、API 與測試路徑（可多條，但每條獨立 brief）。階段依序為：Read（每條一名唯讀讀者，逐檔走完）、Refute（逐 finding 重新開檔，預設反駁）、Critic（未覆蓋 route／檔案與 shared component 重複）。輸出須包含每條失敗／拒絕／缺席路徑、file:line、七種形狀或其他違規規則、讀取範圍與 unsure。

反駁新增的 `missed` 不得直接當確認缺陷，必須由獨立唯讀複核重新查證後才可入帳。證據是完整讀取清單、findings、逐項 verdict、獨立複核與 critic；缺結果或與程式／契約矛盾即停止回報。此流程唯讀，不自動修補。

### 批次修補

輸入是互斥 path allowlist 的寫入 briefs、每項測試命令與 gate 成功行。主代理序列派送：單一 Writer → 唯讀 Verify → 突變證明（還原修正、確認有斷言的紅、恢復且 diff 為空；純文案、純註解與沒有可判定斷言的整理免做並記下理由）。全部完成後主線只跑一次整合 gate；gate 綠不能單獨宣稱完成，要逐項核對每份 brief 的結果。

所有流程的模型依 `automation.md` 分級：從能完成工作的最低級開始，快速文件整理可用快速執行；需要安全判斷或對抗性複核才升級深度推論，禁止派旗艦級。流程不得依賴本機記憶；規則提升須核對 repo 現況，不搬入歷史、個人偏好或 commit／push 授權。

# Coding Agent 分層與派工

這份文件說明 Agent 如何取得規則與如何被派工。它補充根 [`AGENTS.md`](../../AGENTS.md)；區域卡不得放寬根共同安全規則，遇衝突必須停下回報，權威規格決定允收內容。官方對 `AGENTS.md` 的啟動載入說明見 [Agent configuration](https://learn.chatgpt.com/docs/agent-configuration/agents-md)；本文件把官方啟動載入與派工介面的工具契約分開記錄。

## 技術棧

| 層 | 選擇 | 依據 |
| --- | --- | --- |
| 前端 | React + TS（Vite、TanStack Router/Query） | ADR-016 |
| 平台後端 | Go：薄 HTTP 層、pgx + sqlc、River | ADR-016、018 |
| LLM 工作負載 | Python FastAPI（uv），內部服務 | ADR-016 |
| 模型供應商 | OpenAI（試跑預設 mini 級；Embedding `text-embedding-3-small`），一律經 LiteLLM，每 Run 短效 Virtual Key | ADR-017 |
| 資料 | PostgreSQL 中心 ＋ S3 相容物件儲存；核心元件容器化自架 | ADR-018 |
| 搜尋 | 混合檢索（向量腿 ＋ FTS 腿 `UNION` 擴充候選，不做 RRF）＋ 索引時 LLM 增強 | ADR-013 |
| Agent Runtime | Claude Agent SDK，事實來源是 image digest；版本字串釘在 `infra/images/runtime-agent-sdk/Dockerfile` 的 `ARG`，**不在 `tools/toolchain.yaml`**；升級必重跑四項實測 | ADR-023 |
| 身分 | GitHub OAuth ＋ Postgres Session（`DEV_LOGIN` 為離線 provider） | ADR-020 |
| Sandbox | gVisor `systrap`，獨立 VM 池，nftables default-deny ＋固定 DNS，不部署 L7 Proxy | ADR-015、005、022 |
| Runtime Image | 自建映像發佈至 GHCR，SBOM 與掃描以 attestation 隨 digest 保存 | ADR-022 |
| LLM 觀測 | Langfuse Cloud（工程調優用，非事實來源；**MVP 未實作**，見 [`05` R-24](../plans/05-pending-rulings.md)） | ADR-017 |
| 互動創作 | Python LangGraph 分階段編排、Go／Postgres 會話快照與事件；已接線，曝光與品質驗收仍待核准，見[開發與驗證](interactive-creation.md) | ADR-067 |
| 契約 | OpenAPI-first；Go 側 models-only，handler 手寫並逐條對齊 | ADR-016、030 |

**Local Runner 與遠端 MCP 已移出 MVP 首發**（決策保留於 ADR-006）。

## 三層指示

- **根共同規則**：根 `AGENTS.md` 的鐵律、共享工作樹、付費／生成限制、凍結邊界與開發自動化紅線，適用整個 repo。
- **區域卡**：目標檔案所在的父目錄 `AGENTS.md`（以及需要時的 `CLAUDE.md`／`.claude/rules/`），說明該區域先讀什麼、允許改什麼與哪個閘門會攔截。
- **權威規格**：需求與允收讀 `docs/plans/`，架構決策讀 `docs/adr/`，設計規範讀 `docs/design/`；程式契約則讀對應 `contracts/` 或程式原生來源。

任何代理讀或改目標前，沿著 **repo root → 目標父目錄** 明確讀完各層 `AGENTS.md`，並依路由要求讀權威規格；跨區工作要讀每個涉及區域的指示。不要假設工具會自動載入，也不要把規則只存在 brief 裡。

## 派工流程

主代理與子代理都適用：

1. 開始前回報已讀檔案與關鍵限制，先確認工作樹中的未知 delta 並保留。
2. brief 必須寫清楚任務、精確 path allowlist、必讀指示、驗證命令／允收條件與模型等級；子代理模型從能完成任務的最低等級起，禁止派旗艦級。
3. 共享工作樹只有一個 Writer；未列入 allowlist 的檔案不可改。子代理不自行 Git 寫入、安裝、生成、formatter 或啟停共享 Compose。
4. 變更後按 brief 驗證；若 brief 與程式碼或權威規格不一致，停下來回報，以程式碼／權威規格為準，不自行推進。

官方啟動載入是從 root 到目前 cwd 的層級指示；若存在 `AGENTS.override.md`，必須先讀它並明確檢查與 `AGENTS.md` 的差異／衝突，不得悄悄略過根安全限制。本次驗證的 `spawn_agent` 沒有 `cwd` 參數，且代理共享目前工作目錄；其他介面應以其工具契約確認，不從任務路徑推定啟動目錄。不要承諾工具會按檔案自動載入指示，必須在 brief 中列出讀取路徑並要求回報。

指示來源變更後，停止依舊版指示的新派工；由唯一 Writer 同步來源並重新讀取，再重新派送。已啟動的代理不會自動重載變更。

## 跨工具任務流程契約

以下是跨工具都能重做的流程，不是某個 runtime 的命令語法。先分類任務，再依工具能力用原生派工組裝同一流程；沒有對應 runtime 時不可直接 `node .claude/workflows/*.js`，也不可把腳本存在當成已執行或已獨立驗證。`.claude/workflows/` 是 Claude workflow 的一種實作；它不會由 `tools/devctl/agent_sync.go` 生成，且不替其他工具提供 runner 保證。

### 直接問答／小任務

輸入是問題、目標檔案或一個小而明確的查證。主代理先判斷範圍，讀 root → 目標父目錄指示與必要的權威來源，再回答證據、假設與限制。若需小型明確修改，可由主代理依 allowlist 直接完成並驗證；較大或需協作的修改才建立下列寫入 brief，不把回答授權推廣成寫入授權。

### `ux-text-audit`

輸入是本次明確的頁面群組、scope 與 exclusions（可指定檔案；未指定時仍須核對腳本的現況清單與本次排除理由），以及設計規範。歷史上曾排除的檔案不等於本次已覆蓋。階段依序為：

1. **Read**：每組一名唯讀讀者，逐檔讀完，列出所有可見文字、rune 計數、分類、D 區塊、Tip／icon／title 候選與未確定項。
2. **Refute**：至少三個唯讀角度重新開檔，攻擊 C／D 邊界、Tip 六條件與計數；反駁結果須逐項保留。
3. **Critic**：列出未讀檔案、未覆蓋頁面及只能由 fixture／伺服器字串補足的內容。

證據是讀者結構化結果、反駁結果與 critic 缺口；它是稽核報告，不是修改授權，完成不得只以 gate 判定。停止條件是所有輸入群組都有結果，未讀／不確定／被反駁項已明列；缺任何一項就回報未完成。

### `error-path-audit`

輸入是一條功能線的 Go handler、契約、頁面、API 與測試路徑（可多條，但每條獨立 brief）。階段依序為：Read（每條一名唯讀讀者，逐檔走完）、Refute（逐 finding 重新開檔，預設反駁）、Critic（未覆蓋 route／檔案與 shared component 重複）。輸出須包含每條失敗／拒絕／缺席路徑、file:line、七種形狀或其他違規規則、讀取範圍與 unsure。

反駁新增的 `missed` 不得直接當確認缺陷，必須由獨立唯讀複核重新查證後才可入帳。證據是完整讀取清單、findings、逐項 verdict、獨立複核與 critic；缺結果或與程式／契約矛盾即停止回報。此流程唯讀，不自動修補。

### `parallel-page-edit`

輸入是互斥 path allowlist 的寫入 briefs、共同 context、每項測試命令與 gate 成功行。預設流程是主代理序列派送：單一 Writer（包含任何突變操作）→ 唯讀 Verify → Mutation（還原修正、確認有 assertion 的紅、完整恢復 diff）→ 主代理檢查每項狀態。Mutation 僅適用於 bug fix 或其他有可失敗斷言的行為修正；純文案、純註解與本來沒有可判定斷言的文件整理免做，並在證據中記下理由。唯讀工作可平行，但同一目標的驗證須等寫入穩定；Writer、生成、formatter、package manager 與高衝突區一律序列化。

原生 `parallel-page-edit.js` 的多 brief 扇出安全尚未跨工具驗證，不作預設，也不宣稱單 brief 已安全；只有先確認該工具的單 brief 呼叫契約與共享工作樹隔離後才能採用，否則用上述序列組裝，不新增 runner。即使確認可呼叫，原腳本 gate 只檢查 Verify 結果，不檢查 Mutation 是否變紅或已恢復；主代理仍須逐項驗收，不能以 gate 代替。若工具沒有獨立 Writer／Verify／Mutation 能力，回報限制，不能假裝具備獨立驗證。批次契約須等全部 briefs 完成 Writer／Verify／適用的 Mutation 後，由主線只跑一次適用整合 gate；缺失或未恢復不能當完成。完成判定須逐項核對 writer／verify／mutation、mutation 已恢復、out-of-scope 與缺失結果，再看 gate 的 exit code 和成功行；gate 綠不能單獨宣稱任務完成。

所有流程的模型依 `automation.md` 分級：從能完成工作的最低級開始，快速文件整理可用快速執行；需要安全判斷或對抗性複核才升級深度推論，禁止派旗艦級。流程不得依賴本機記憶；規則提升須核對 repo 現況，不搬入歷史、個人偏好或 commit／push 授權。

## 無記憶情境驗收題

每次換工具、換代理或重開工作都可重新出題；下列是題目與應留下的證據，**不預填通過／失敗**：

| 情境題 | 預期證據 |
| --- | --- |
| 只有一個小型唯讀問題，代理應改檔嗎？ | 先判為直接問答，列出 root → 目標父目錄的讀取清單、結論、假設與限制；沒有 diff。 |
| `ux-text-audit` 少了一組檔案或某段文字只能從 fixture 得知，能宣稱完成嗎？ | Read 結果、逐項 Refute 結果、Critic 的未讀／不可計數清單；缺任一項即停下回報。 |
| `error-path-audit` 的 refuter 回報一個 `missed`，可直接寫入帳本嗎？ | 原 finding、refuter 結果、獨立唯讀複核的 file:line 與 verdict；沒有複核不得算確認缺陷。 |
| 兩個 page briefs 同時要寫共享工作樹，能直接扇出嗎？ | 共享工作樹無論 allowlist 是否交集都只准單一 Writer；交集另須重分責任或處理依賴，再依序 Writer → Verify → Mutation，不宣稱 native workflow 安全。 |
| 寫入後 gate 綠了，能宣稱全部完成嗎？ | 每項 writer／verify／mutation 狀態、mutation 已恢復且 diff 相符、缺失與 out-of-scope 結果，加上 gate exit code 與成功行；任一缺失即未完成。 |
| brief 與程式碼或契約互相矛盾，應怎麼做？ | 指出衝突的檔案／行號與權威來源，停止該流程並回報；不得自行推進或以 CI 靜態檢查代替行為驗證。 |

# ADR-016：語言與框架 — TS 前端、Go 平台、Python LLM 服務

- 狀態：Accepted
- 日期：2026-08-13
- 決策者：產品負責人、架構規劃

## 背景

MVP 進入實作前需選定語言與框架。決策驅動因素：

1. 單人開發，每多一種語言就多一套工具鏈與心智切換成本。
2. 產品域（MCP、Agent Skills、Claude API）的一流 SDK 集中在 TypeScript 與 Python。
3. 執行平面（containerd、gVisor、Kubernetes）與未來 Local Runner（跨平台單一執行檔）是 Go 的原生領域。
4. UI 有真實複雜度（SSE 進度、Trace 檢視、版本 diff），前端必然是 React／TS。
5. ADR-002 模組邊界需可機器檢查；ADR-008 要求 Run 狀態單一事實來源；ADR-014 為 Postgres 中心。

## 評估選項

### 選項 A：TypeScript 全端

- 優點：單語言、契約（Zod）前後端共用。
- 缺點：與團隊既有 Go／Python 能力不符；執行平面與 Local Runner 生態逆風。

### 選項 B：Python 全後端 + TS 前端

- 優點：LLM 生態最強、語言數少。
- 缺點：控制平面的狀態機、佇列與未來 infra 整合非 Python 強項。

### 選項 C：Go 平台 + Python LLM 服務 + TS 前端（採用）

- 優點：每種語言都用在其原生領域，且與 ADR-001 的平面劃分自然對齊。
- 缺點：三套工具鏈；跨語言契約需要紀律；LLM 工作流與平台狀態機有重疊風險（以下守則處理）。

## 決策

> **更正（2026-08-22 查證，2026-08-30 複驗並補上依賴現況）**：下面兩張表與跨語言守則第 1 條都把
> **LangGraph** 寫成已採用的選型，**它從未被採用過**。決策表本身不改寫——它是決策當下的紀錄——
> 但讀它的人要知道實際落地的是什麼。
>
> **2026-08-30 逐項複驗**：`apps/llm/pyproject.toml` 的直接依賴只有 **`fastapi`／`uvicorn`／`openai`／`pydantic`** 四項，
> `uv.lock` 裡 `langgraph`／`langchain` 皆 **0 筆**；七個端點（`/embed`、`/match-reasons`、`/suggest-criteria`、
> `/v1/enrich-skill`、`/judge-run`、`/suggest-improvements`、`/v1/generate-skill`）**各一次閘道呼叫，共 7 次，
> 沒有任何迴圈包著呼叫**；client 以 `max_retries=0` 建構，重試留在 Go（守則 3）。
> `evaluate.py` 檔頭與 `contracts/openapi/llm-internal.yaml` 都逐字寫著 `no LangGraph`。
>
> **2026-08-22 那份更正當時把依賴記成 `fastapi`／`uvicorn`／`litellm`／`openai`，其中 `litellm` 之後被移除**
> （它被宣告卻未被 import，而它的進入點會依模型名前綴自選供應商 handler，等於繞過鐵律 8 的一條路；
> 理由寫在 `app.py`），`pydantic` 則由 transitive 升為直接依賴。
>
> **不引入的裁定不在本 ADR，而在 [m3/evaluation-design.md §2.3](../plans/mvp/m3/evaluation-design.md)**：
> 本 ADR 提到 LangGraph 是**選型層級的授權，不是每個端點都得用它的義務**，
> 而「組 prompt → 呼叫 → 驗 schema」是單次呼叫。若改善建議生成日後真的需要多步，
> 那時再引入並把理由寫進該批的交付摘要。
>
> **守則第 1 條要照樣讀**：它用 LangGraph 的 checkpoint 當例子說明「Python 側的程序內狀態不得成為第二個持久化工作流層」。
> **那條規則不依賴 LangGraph 存在**——今天 Python 側根本沒有任何跨請求狀態，所以它是被滿足到底，不是被繞過。

| 平面 | 語言 | 範圍 |
| --- | --- | --- |
| 體驗平面 | TypeScript（React） | Web UI、進度串流、Trace 檢視 |
| 控制＋執行平面 | Go | API、ADR-002 全部領域模組、Run 狀態機、佇列 Worker、Sandbox Worker、（後 MVP）Local Runner |
| LLM 工作負載 | Python（LangGraph） | 索引時增強、查詢改寫、LLM Judge、改善建議生成 |

### 框架與函式庫

| 層 | 選擇 | 說明 |
| --- | --- | --- |
| 前端 | Vite + React + TanStack Router/Query | SPA 起步；公開頁面若需 SEO 再評估 SSR |
| Go HTTP | chi 或 echo（薄層） | 標準庫優先，不用重框架 |
| Go 資料存取 | pgx + sqlc | SQL-first，符合 ADR-014 的 Postgres 中心 |
| Go 佇列 | River | Postgres 佇列，與 Outbox 同庫同交易（回應 ADR-014 待決策） |
| 模組邊界檢查 | Go internal package + 依賴 lint（go-arch-lint 類） | 回應 ADR-002 待決策 |
| Python 服務 | FastAPI + LangGraph + Anthropic SDK，uv 管理 | 內部服務，不對外 |
| 契約 | OpenAPI-first | Go 為 spec 來源，codegen 產 TS client 與 Python server/client stub |

## 跨語言邊界守則

1. **狀態單一擁有者**：Run 與工作流的持久化狀態只存在於 Go 擁有的 Postgres 狀態機（ADR-008）。LangGraph 是單次工作「程序內」的編排庫；其 checkpoint 視為暫存草稿，僅限單一 Job 範圍，不得成為第二個持久化工作流層。
2. **Python 是能力提供者，不是規則擁有者**：Python 服務接受結構化請求、回傳結構化結果（含證據與信心）。政策、授權、狀態轉移、重試決策全在 Go。業務規則不得寫入 Python。
3. **Python 不消費佇列**：佇列消費者只有 Go Worker。Go Worker 呼叫 Python 內部 HTTP API（含逾時、取消傳遞），LLM 工作的排程、重試與冪等由 Go 依 ADR-008 管理。
4. **契約先行**：三語言間的每個介面先寫 OpenAPI schema 再實作；CI 以 codegen 檢查 drift。Run Contract（ADR-004）的 schema 是 Go 側的單一定義。
5. **Sandbox 內的 Agent Runtime 不在此範圍**：Skill 試跑時在 Sandbox 內執行的 Agent（如 Claude Agent SDK）屬工作負載，語言由 Runtime Image 決定（ADR-005），與平台語言選型無關。

## 影響

### 正面

- 每種語言用在生態最強處；Sandbox Worker 與 Local Runner 不需跨語言橋接。
- LLM 邏輯集中在 Python，模型、Prompt 與評估策略可獨立迭代與測試。
- 三平面各自可獨立部署與擴展，符合 ADR-010 的拆分接縫。

### 成本與限制

- 三套工具鏈、測試與 CI 管線；契約 drift 是主要風險，靠 OpenAPI codegen 與契約測試（NFR-006）壓制。
- Python 服務是 Run 關鍵路徑的一環，需納入 O11y 與逾時預算（NFR-004）。
- 開發環境需同時起三個 runtime；以 docker compose 一鍵化。

## 待決策

- Go↔Python 內部通訊維持 REST 或改 gRPC（流量與串流需求確認後）。
- 公開 Skill 頁面的 SEO 需求與 SSR 時點。
- LangGraph checkpoint 在長時評估中的暫存策略（單 Job 內重試恢復）。 **→ 前提消失（2026-08-22 查證）：LangGraph 從未被採用。** `apps/llm/pyproject.toml` 的依賴只有 `fastapi`／`uvicorn`／`litellm`／`openai`（**2026-08-30 訂正：`litellm` 已移除、`pydantic` 已升為直接依賴，現為四項**），`evaluate.py` 檔頭逐字寫「no tools, no LangGraph」，兩個 endpoint 都是單次閘道呼叫（**2026-08-30 複驗：全服務七個端點、七次呼叫、零迴圈**）。**所以這條待決策不存在受詞。** AGENTS.md 的技術棧表與鐵律 5 曾以它為條文，已同批更正。

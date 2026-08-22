# ADR-023：Agent SDK 的版本釘選與行為重驗政策

- 狀態：Accepted
- 日期：2026-08-16
- 決策者：產品負責人（概括授權）、架構規劃
- 相關：[ADR-015](./ADR-015-sandbox-isolation-technology.md)（Runtime Image 屬 Sandbox 供應鏈）、[ADR-017](./ADR-017-model-gateway-and-llm-observability.md)（模型出口）、[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（Image 掃描與發佈門檻）、[PDM-003 Spike](../plans/mvp/m0/pdm-003-litellm-spike-report.md)

## 背景

Agent SDK 是沙箱內唯一執行 Skill 的元件，平台對「Skill 有沒有被載入、模型呼叫有沒有走閘道、用量與成本怎麼計」的全部認知都由它產生。

2026-08-16 的第四批實測**推翻了 PDM-003 Spike 的一項結論**：Spike 在 `claude-agent-sdk` 0.2.137 上量到「`settingSources: ["project"]` 才發現得到專案 skill」，而映像釘的 0.3.233 上**完全相反**——傳 `["project"]` 發現到零個 skill，**省略**該選項才發現得到；同時 skill 啟用從 `allowedTools`（傳 `'Skill'` 已 deprecated）移到獨立的 `skills` 選項。

這件事的性質不是「SDK 有 breaking change」，而是：

1. **它靜默失效。** 載入零個 skill 的 Run 不報錯、不警告，Agent 照樣回一段看起來合理的話，Run 照樣 `succeeded`。第四批之前沒有任何自動化能發現它。
2. **它推翻的是一份被引用過的實測結論。** Spike 報告不是猜的，它是量出來的——量測結果本身有版本壽命，而引用它的人不會知道壽命已到。
3. **它沒有留下決策紀錄。** 反轉後的正確做法寫進了 `run.mjs` 檔頭與 `services/sandbox/README.md`，版本則只釘在 `Dockerfile` 的 `ARG CLAUDE_AGENT_SDK_VERSION=0.3.233`。[m2 對帳 §9.7](../plans/mvp/m2/m2-work-items-audit.md) 因此把「無 ADR 記錄此次行為反轉」列為未關閉的一半。本 ADR 關閉它。

## 決策

### 1. 版本只釘 digest 與 lockfile，不釘語意版本範圍

- Runtime Image 的基底以 `FROM <image>@sha256:<digest>` 釘選（I-02，CI 已有 grep 斷言）。
- SDK 以**精確版本**釘在 `ARG CLAUDE_AGENT_SDK_VERSION`，安裝走 lockfile（`npm ci`）；`^`／`~`／`latest` 一律不得出現在映像建置路徑上。
- **最終事實來源是 image digest，不是任何一個檔案裡的版本字串。** `sandboxd` 的同名預設值只是開發便利，兩者不一致時以映像為準；發佈至 GHCR 後（`03` SBX-011），digest 是唯一該被引用的識別。
- 「升級」的定義是**任何會改變該 digest 的變更**：SDK 版本、基底映像、映像內任何套件。三者都適用下一節。

### 2. 升級必須重跑的實測清單

以下四項在升級 PR 合併前必須**在釘定的新 digest 上實跑**，並附輸出證據。缺一項即不得合併。

| # | 測項 | 斷言 | 為什麼是它 |
| --- | --- | --- | --- |
| 1 | **Skill 載入條件** | 掛一個已知 Skill 的 Run，`init` 訊息的 skills 清單含該專案 skill，且 Trace 出現對應的 `skill_activation` 事件 | 這是已經反轉過一次的那一條。`cwd`／`settingSources` 省略／`skills: "all"`／工具清單，四者缺一則零個 skill |
| 2 | **閘道相容** | 模型呼叫全數經 LiteLLM（`ANTHROPIC_BASE_URL`＋短效 Virtual Key），閘道 spend log 有對應紀錄；金鑰撤銷後 `/v1/messages` 回 **401** | 鐵律 8。SDK 換了 HTTP 客戶端或改用新路由時，「直連供應商」是不會有人通知你的失敗 |
| 3 | **Prompt caching 計費欄位** | 記錄該版本在 `/v1/messages` 路由上是否輸出 `cache_read_input_tokens`／`cache_creation_input_tokens`；`usage` 事件的 `input_tokens`／`output_tokens` 與閘道 per-key spend 對帳一致 | PDM-005 §5.2a-7 記載這兩個欄位目前是**缺欄不是 0**；token 上限的強制點依賴 `input_tokens`（`02:RUN-003`），欄位語意變動會直接改變強制行為 |
| 4 | **`usage` 事件的發出條件** | 確認 `usage` 是否仍只在 SDK 的 `result` 分支發出（崩潰／被殺／abort 時不發） | 這是 EVAL-012 成本合計系統性低估的來源（[`04-backlog-and-handoffs`](../plans/04-backlog-and-handoffs.md) 丙-3）。行為若改變，下游的補償邏輯要跟著改 |

可執行形式已存在：`services/platform/internal/identity/e2e_gateway_integration_test.go`（以 `SKILLHUB_E2E_SANDBOX_URL` 開關，會花錢故 CI 不跑）。單次 Run 成本約 $0.006–0.017，這是升級的**已知固定成本**，不是可省的一項。

### 3. 靜默失效不得用推理帶過

上表每一項失敗時都不會拋錯，只會讓某個行為消失，而 Run 仍然成功。因此：

- **升級 PR 必須附實測輸出，不得以 SDK changelog、release notes 或「這個選項的語意應該沒變」代替。** 這條規則的成立依據就是 0.3.233 那次：changelog 讀起來完全合理，而行為是反的。
- 文件與程式碼註解裡任何一句「SDK 會如何如何」，都必須指得出是哪一次實測、在哪個版本上量的。**沒有版本的實測結論等於沒有結論。**
- CI 不能取代這件事（需要真模型呼叫與真沙箱），所以它是人工關卡；補償方式是把證據落檔，而不是假裝有自動化。

### 4. 證據放哪

- **落檔位置**：`infra/images/runtime-agent-sdk/UPGRADES.md`，append-only 表格，每次升級一列——日期、舊 → 新版本與 digest、四項測項各自的結果與關鍵輸出摘要、對應的 Run id 或 CI 連結、以及**本次是否推翻了任何既有的文件敘述**（若有，同 PR 修正 `run.mjs` 檔頭與 `services/sandbox/README.md`）。該檔隨第一次升級建立，不預先放空表。
- **原始輸出**留 CI artifact 或本機 Run 的 Trace，於表格內附連結；Trace 本身已是可重查的證據（`trace_events` 依 `run_id` 可查）。
- **不另建目錄**：這不是里程碑證據（`SEC-009` 那類存 `plans/mvp/m4/...`），而是隨映像生命週期存在的紀錄，放在映像旁邊才找得到。

## 影響

### 正面

- 「SDK 行為只能實測不能推理」有了持久落點，不再只存在於兩個檔案的註解裡——註解會隨檔案重寫消失，ADR 不會。
- 升級成本變成明碼標價的一次 e2e Run，而不是一次不知道何時會爆的靜默回歸。
- 與 ADR-022 的 Image 發佈門檻（SBOM、掃描、attestation）互補：那邊管**供應鏈風險**，這邊管**行為回歸**，兩者都不覆蓋對方。

### 成本與限制

- 每次升級都要花錢跑一次真實 Run，且需要人看輸出——這是刻意的，沒有更便宜的等價物。
- 四項清單只涵蓋**已知會靜默失效的面**；SDK 可能以其他方式靜默改變行為，清單須隨每次發現追加（追加即改本 ADR 或另立 ADR，不在別處私自維護第二份）。
- 升級延遲是明示的代價：若安全性修補要求快速升版，實測仍不可略過，只能提高優先序。

## 待決策

- 是否為 SDK 升級開一條有預算的 CI 路徑（例如每月一次的排程 e2e），使升級不完全依賴人工觸發；需先有 `PDM-010` 的成本歸屬。
- npm 供應鏈的涵蓋範圍：`03` SBX-011 的 SBOM／attestation 是否涵蓋 `node_modules`，以及 SDK 的傳遞依賴是否納入 I-06 的漏洞門檻。
- 上游若提供可程式化的能力宣告（例如查詢「目前生效的 skill 來源」），可否用它把測項 1 從 e2e 降級為便宜的煙霧測試。

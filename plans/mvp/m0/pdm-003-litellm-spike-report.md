# PDM-003：LiteLLM 閘道相容性 Spike 報告

- 日期：2026-08-14
- 對應：[pdm-proposals.md §3（PDM-003）](./pdm-proposals.md)「必須先做的前置 Spike」第 1 項、[ADR-017 模型閘道與 LLM 可觀測性](../../../adr/ADR-017-model-gateway-and-llm-observability.md)
- Spike 程式碼：[`spikes/pdm-003-litellm-gateway/`](../../../spikes/pdm-003-litellm-gateway/)
- 狀態：**已執行，7/7 測項通過。結論為「協定層可行，PDM-003 的閘道假設成立」**，但範圍有明確界定（見第 3 節）與三個必須處理的相容性坑（見第 6 節）。
- **2026-08-14 追加**：前置 Spike 第 2 項（Agent SDK 的 Skill 載入路徑）已補測，**6/6 PASS，見第 10 節**。載入機制成立，但**提案假設的 `<workdir>/skills/<name>/` 路徑被證偽**，正確路徑為 `<workdir>/.claude/skills/<name>/`。
- **2026-08-14 再追加（第 11 節）**：模型供應商已定案為 **OpenAI API（經 LiteLLM 閘道，ADR-017 架構不變）**，原「需 Anthropic 憑證」的補測項改於正式後端完成。三項結果：(i) **Skill 自主觸發率 0/9**，旗艦與 mini 級同為 0，不可作為試跑成功判準；(ii) **`skills` 白名單的行為性過濾成立**，對 15 個內建 CLI Skill 同樣有效；(iii) **prompt caching 不會增加 300K token 上限能買的輪數**（省的是錢不是 token），300K 實測夠 **15 輪（無工具）／7.7 輪（每輪 1 次工具呼叫）**，且 `/v1/messages` 路由**不透傳 cache 用量欄位**（計費正確、可觀測性受損）。

---

## 1. 目的與範圍

PDM-003 §3 把「LiteLLM Proxy 的 Anthropic 相容端點在 Claude Agent SDK ＋ tool use ＋ streaming 路徑下行為正確」列為**定案前置**，並寫明「若不相容，退路是選項 A（Claude Code CLI）或在閘道前加一層薄轉譯」。同時 ADR-017 的待決策「Sandbox 內 Agent Runtime 注入 Virtual Key 的具體機制」也依賴同一組驗證。

本 Spike 驗證四件事：

1. LiteLLM 的 `/v1/messages`（Anthropic 相容端點）回應結構是否完整。
2. streaming 事件序列是否合法。
3. tool use 在串流與非串流下的轉譯是否正確（特別是 `input_json_delta` 能否組回完整 arguments）。
4. 每 Run 一把短效 Virtual Key 的**模型範圍限制與預算強制**是否真的生效，且能以 `ANTHROPIC_BASE_URL` ／ `ANTHROPIC_AUTH_TOKEN` 環境變數注入方式使用（＝ PDM-003 §3 建議的注入機制、威脅模型 Q11）。

額外加測（PDM-003 §3 前置 Spike 第 2 項的一半）：`claude-agent-sdk` 能否以環境變數指向閘道完成一次含 tool 的完整對話。

---

## 2. 方法

| 項目 | 作法 |
| --- | --- |
| 閘道 | LiteLLM Proxy **1.96.2**（pip 與官方 image 兩種部署都跑過，版本相同） |
| 官方 image | `ghcr.io/berriai/litellm:main-stable`，digest `sha256:154e23bb…3c63` |
| 模型別名 | `sonnet-test`、`haiku-test` → `openai/gpt-4o-mini` |
| 客戶端 | `anthropic` Python SDK **0.121.0**，以 `ANTHROPIC_BASE_URL` ／ `ANTHROPIC_AUTH_TOKEN` 環境變數建構（不傳任何建構參數，模擬 Sandbox 注入） |
| Agent Runtime | `claude-agent-sdk` **0.2.137**（Python 版），工具以 in-process SDK MCP server 提供 |
| Virtual Key 儲存 | 拋棄式 `postgres:17-alpine` 容器（LiteLLM `/key/generate` 強制需要資料庫） |
| Master key | 每次執行以 `secrets.token_urlsafe(24)` 隨機產生，不寫入任何交付檔案 |
| 其他版本 | `openai` 2.54.0、`fastapi` 0.139.2、`prisma` 0.15.0、Python 3.14.0 |

安全處理：API 金鑰只從 repo 根目錄 `.env` 讀取（該檔已 gitignore），不進入 `results.txt`、`config.yaml`、`test_gateway.py` 或本報告。交付前以字面比對掃描過三個檔案，均無金鑰殘留。

---

## 3. 範圍界定（必讀）

> **本環境沒有 Anthropic API key，`.env` 只有 `OPENAI_API_KEY`。**

因此本 Spike 驗證的是**協定層**：客戶端以 Anthropic Messages 格式呼叫 LiteLLM，由 LiteLLM 轉譯到 OpenAI 後端模型。

| 已驗（OpenAI 後端） | 待補測（Anthropic 後端） |
| --- | --- |
| LiteLLM 的 Anthropic 相容層是否接受標準 Anthropic 請求 | Anthropic 原生模型經閘道的端到端行為 |
| Anthropic → OpenAI 的 tool use 雙向轉譯 | `thinking` 區塊的**透傳**（PDM-003 §3 明確點名的觀察項，本 Spike **未能驗證**，見 §6.1） |
| SSE 事件型別與順序是否符合 Anthropic 規格 | Anthropic 特有的 `cache_creation_input_tokens` ／ prompt caching 欄位是否正確回報 |
| Virtual Key 的模型範圍與預算強制 | Anthropic 供應商實際計價下的預算精準度 |
| Agent SDK 以環境變數指向閘道能否跑完含 tool 的迴圈 | Agent SDK 在 Anthropic 後端下的 extended thinking 行為 |

**結論的可轉移性**：Virtual Key 流程（測項 E）與環境變數注入機制完全與後端無關，可直接採信。測項 A–D 驗證的是 LiteLLM 相容層本身，Anthropic 後端時這一層做的事**更少**（近乎直通），因此風險方向是「OpenAI 後端通過 ⇒ Anthropic 後端更可能通過」，唯一例外是 `thinking`（§6.1）。

---

## 4. 逐項結果

全部測項見 [`spikes/pdm-003-litellm-gateway/results.txt`](../../../spikes/pdm-003-litellm-gateway/results.txt)。

| # | 測項 | 結果 | 觀測值 |
| --- | --- | --- | --- |
| — | Proxy 啟動與 `/health/liveliness` | **PASS** | — |
| A | 非串流基本訊息 | **PASS** | `type=message`、`role=assistant`、1 個 `text` block、`stop_reason=end_turn`、`usage` 為 13 in／3 out（非零） |
| B | streaming 事件序列 | **PASS** | 25 個事件；`message_start` → `content_block_start` → `content_block_delta` → `content_block_stop` → `message_delta` → `message_stop`，順序斷言通過；重組文字非空 |
| C | tool use（非串流）往返 | **PASS** | `stop_reason=tool_use`、`tool_use` block 帶合法 `id`、`input={'location':'Taipei','unit':'celsius'}`；回傳 `tool_result` 後 `stop_reason=end_turn` 且最終答案含工具回傳值 |
| D | tool use ＋ streaming | **PASS** | 11 個 `input_json_delta`，手動串接後 `json.loads` 成功得 `{'location':'Kyoto','unit':'celsius'}`，且**與 SDK 自行組裝的結果逐欄位相等**；`stop_reason=tool_use` |
| E | Virtual Key：範圍＋預算 | **PASS** | 建立成功；允許模型可用；**allow-list 外的模型被拒（403）**；**預算耗盡後被拒（429）** |
| F | `claude-agent-sdk` 經 `ANTHROPIC_BASE_URL` | **PASS**（附條件，見 §6.1） | 2 turns，完成 `ToolUseBlock` → `ToolResultBlock` → `TextBlock` 完整迴圈，`is_error=False` |

### 4.1 測項 D 的斷言為何重要

PDM-003 §3 風險表把「LiteLLM 的 Anthropic 相容端點與 Agent SDK 不完全相容」列為「PDM-003 整個方案失效」的風險。tool use ＋ streaming 是這條路徑上最脆弱的一段：Anthropic 的 `input_json_delta` 是**逐段 JSON 字串**，OpenAI 的 `tool_calls` delta 是**逐段 arguments 字串**，兩者切分邊界不同。本測項不只斷言「有 delta 事件」，而是斷言**串接後可 `json.loads`** 且**與 SDK 組裝結果相等**——這才是「轉譯正確」而非「轉譯有輸出」。

### 4.2 測項 E 的細節

Virtual Key 以 LiteLLM 管理 API `/key/generate` 建立，帶三個 PDM-003 ／ ADR-017 要求的屬性：

```
models:     ["sonnet-test"]        # 模型範圍限制
max_budget: 0.000002               # 預算上限（刻意設極小以便觀測）
duration:   "20m"                  # TTL，對齊 PDM-003 建議的 20 分鐘
metadata:   {"run_id": "..."}      # Run 級成本歸因（ADR-017）
```

三個行為都實測到：allow-list 外模型回 **403**、預算耗盡回 **429**。兩者是**不同狀態碼**，直接支撐 PDM-005 風險表「Token 預算耗盡的失敗與模型錯誤難以區分」的緩解措施——`budget_exhausted` 可映射為獨立診斷碼（NFR-003），不需要靠解析錯誤訊息字串。

**但預算強制是非同步的**：LiteLLM 先扣後檢，spend 需要 flush 到資料庫後下一次請求才會被擋。本 Spike 已設 `proxy_batch_write_at: 1`（每秒 flush），測試仍需重試迴圈才觀測到拒絕。**這代表預算是「軟上限」——超支幅度約等於 flush 間隔內能發出的請求量**，不是硬性截斷。見 §6.3。

---

## 5. 環境相容性紀錄（Python 3.14）

PDM-003 §3 要求誠實記錄安裝相容性。`pip install "litellm[proxy]" openai anthropic` 在 **Python 3.14.0 上可以安裝成功**（無需 `--pre` 或降版），但**開箱即用的 proxy 無法啟動**，需要三次修正：

| 症狀 | 原因 | 修正 |
| --- | --- | --- |
| `python -m litellm` → `No module named litellm.__main__` | litellm 只提供 console script，不是可執行套件 | 改用 `.venv/Scripts/litellm.exe` |
| `ImportError: cannot import name 'get_flat_dependant' from 'fastapi.dependencies.utils'` | **litellm 1.96.2 未對 FastAPI 設上界**，pip 解析到 0.141.1，該符號已被移除 | 釘 `fastapi<0.140`（實測 0.139.2 可用） |
| `ModuleNotFoundError: No module named 'prisma'` → 再來 `Unable to find Prisma binaries` | `litellm[proxy]` extra **不含 `prisma`**；補裝後 `prisma generate` 又被 npx 拉到 Prisma CLI 7.9.1，該版本拒絕 litellm 的 schema（`datasource.url` 已不支援），且 `PRISMA_VERSION=5.17.0` 未被採納 | **放棄 pip 部署走資料庫路徑**，改用官方 image |

**結論**：pip 安裝的 proxy 在本平台上**只能跑無資料庫模式**（測項 A–D 已用它跑過並全數通過）；任何需要 Virtual Key 的功能（測項 E，也就是 ADR-017 的核心機制）必須用官方 container image。

> **對部署的影響**：這其實與 ADR-017「LiteLLM Proxy 作為獨立部署單元」的既定決策一致——正式環境本來就該用官方 image，不該 pip 安裝。此處記錄的價值在於：**任何開發者的本機環境若想跑帶 Virtual Key 的閘道，必須用 container，不能靠 `pip install`。** 這應寫進未來的開發環境文件。

---

## 6. 發現的相容性坑

### 6.1 `thinking` 區塊在 `/v1/messages` 上不受 `drop_params` 管轄（最重要）

**現象**：Claude Agent SDK（Claude Code harness）預設會送出 Anthropic 的 `thinking` 區塊。LiteLLM 將其轉譯為 OpenAI 的 `reasoning_effort`，而 `gpt-4o-mini` 不支援該參數，回 400：

```
Unsupported parameter: 'reasoning.effort' is not supported with this model.
```

**已隔離的根因**（同一組 config、同一個 proxy 程序）：

| 路徑 | 送出參數 | 結果 |
| --- | --- | --- |
| `/v1/chat/completions` | `reasoning_effort: "low"` | **200 OK**（`drop_params` 生效，參數被丟棄） |
| `/v1/messages` | `thinking: {type: enabled, …}` | **400**（`drop_params` 與 `additional_drop_params` **均未生效**） |

也就是說，這**不是模型能力問題，是 LiteLLM 1.96.2 的 Anthropic 相容路由沒有套用參數丟棄邏輯**。設定 `additional_drop_params: ["reasoning_effort", "thinking"]` 於該路由無效（config 中保留該行並加註，作為此發現的紀錄，不是修正）。

**Spike 採用的變通**：客戶端設 `MAX_THINKING_TOKENS=0`。設定後測項 F 立即通過完整 tool 迴圈。

**對 PDM-003 的意義**：

- **不阻擋定案**。此坑只在**後端不是 Anthropic 模型**時觸發。PDM-003 表定的試跑模型全是 Anthropic（`claude-sonnet-5` ／ `claude-opus-5`），`thinking` 會原生透傳，不需轉譯。
- **但它直接命中 PDM-003 §3 點名的觀察項，而本 Spike 無法驗證**。「`thinking` 區塊透傳」屬第 3 節表格右欄的待補測項，且已知該路由的參數處理有缺陷——**補測時應把 `thinking` 列為第一優先驗證項，不能因為「Anthropic 後端是直通」就跳過**。
- **它讓 ADR-017「模型抽換與容錯（fallback、重試、路由）設定在閘道層」出現一個限制**：若把非 Anthropic 模型設為 fallback，Agent SDK 的請求會在 fallback 觸發時 400。ADR-017 §邊界守則 4 說「閘道故障視為 Provider 級故障」，但這裡失敗的是**成功切換後的第一個請求**。跨供應商 fallback 需要實測，不可假設可用。

### 6.2 Claude Agent SDK 的 harness 開銷極大（對 PDM-005 有直接影響）

測項 F 一次「一輪工具呼叫 ＋ 一句回答」的最小對話，實測 **`input_tokens` = 50,139**，`output_tokens` = 38。

這是 Claude Code harness 的系統提示與內建工具定義的固定成本。對照 PDM-005 §5.2 的每 Run 上限 **300K input ／ 60K output**：

- 若每輪都重送完整 harness context 而無 prompt caching，**約 6 輪就會耗盡 input 預算**。
- 亦即 PDM-005 的 300K input 預算對 Agent SDK 而言是**約 5–6 個 agent turn**，不是「很寬鬆」。
- 本次測量在本機環境下進行，`setting_sources=[]` 已停用專案設定，但 harness 仍載入了完整內建工具清單（Task／Bash／Edit／Glob／Grep／WebFetch／Skill 等 20+ 項）。**Sandbox 內的 Runtime Image 若能裁減工具集，這個數字會顯著下降**——這是一個可行的成本槓桿，值得在 SBX-002 設計時評估。

> ⚠️ **此數字未經 prompt caching 驗證。** Anthropic 後端的 prompt caching 會讓重複的 harness context 以 cache read 計價（約 1/10）。**補測 Anthropic 後端時應一併量測 `cache_read_input_tokens`，再回頭校正 PDM-005 的 300K 上限是否合理。** 在此之前，300K input 上限應視為**未經驗證的數值**。

### 6.3 Virtual Key 預算是軟上限，不是硬性截斷

如 §4.2 所述，LiteLLM 的 spend 記帳是非同步的：請求先執行、事後累計，超出預算的判定發生在**下一次**請求。超支幅度取決於 `proxy_batch_write_at` 間隔內的請求併發量。

**對 PDM-003 §3 殘餘風險評估的修正**：該節寫「爆炸半徑上限＝一次 Run 的預算，約 $1.80」。**嚴格說應為「約 $1.80 加上 flush 間隔內可發出的請求量」**。在 Sandbox 內腳本可高併發呼叫的前提下，這個尾巴不是零。緩解方向是同時設 `rpm_limit` ／ `tpm_limit`（LiteLLM 支援，且該類限制是即時的，不依賴 spend flush），而不是只靠 `max_budget`。**建議定案時把「Virtual Key 必須同時帶 `max_budget` 與 `tpm_limit`」寫進 PDM-003。**

### 6.4 次要：`ANTHROPIC_AUTH_TOKEN` 的優先順位已實測確認

PDM-003 §3 配套第 2 項要求「Runtime Image 必須確保 `ANTHROPIC_API_KEY` 未設」。測試中觀測到 Claude Code 主動印出警告：

> `claude.ai connectors are disabled because ANTHROPIC_API_KEY or another auth source is set and takes precedence…`

證實憑證解析確實有優先順位機制，且 `ANTHROPIC_AUTH_TOKEN` 被視為一個 auth source。**PDM-003 該項配套的必要性已被證實，維持原樣即可。**

---

## 7. 對 PDM-003 定案的建議

### 7.1 前置 Spike 第 1 項：**建議判定為通過**

PDM-003 §3 的原文條件是「驗證 LiteLLM Proxy 的 Anthropic 相容端點在『Claude Agent SDK ＋ tool use ＋ streaming』路徑下行為正確」。本 Spike 在協定層上全數通過，且額外驗證了 Virtual Key 機制。**PDM-003 §3 風險表第一列「LiteLLM 的 Anthropic 相容端點與 Agent SDK 不完全相容 → PDM-003 整個方案失效」可以降級**：退路（Claude Code CLI 或薄轉譯層）不需要啟動。

**但通過附帶一個未解項**：`thinking` 透傳未驗證，且已知該路由的參數處理有缺陷（§6.1）。建議定案文字寫成「協定層已驗證通過；`thinking` 透傳於取得 Anthropic key 後補測，補測未過則重新評估退路」，而不是無條件通過。

### 7.2 建議併同定案的三項調整

1. **Virtual Key 必須同時帶 `max_budget` 與 `tpm_limit`**（§6.3）。只有 `max_budget` 時預算是軟上限，與 PDM-003 §3 殘餘風險段落宣稱的「爆炸半徑上限」不符。
2. **PDM-005 的 300K input 上限標記為「未經驗證」**（§6.2）。實測 harness 固定開銷約 50K input／turn，該上限只夠約 5–6 輪；是否足夠需在補測 prompt caching 後才能判定。
3. **ADR-017「模型抽換與 fallback 設定在閘道層」加註限制**：跨供應商 fallback 對 Agent SDK 路徑未經驗證，且 §6.1 顯示 Anthropic→OpenAI 方向有已知缺陷。此為 ADR 的既有決策，**本報告不修改 ADR**，僅提出需要新增 ADR 或在補測後回填的事項供負責人裁量。

### 7.3 尚未回答的前置項

PDM-003 §3 列的三個前置 Spike 中：

| 前置項 | 狀態 |
| --- | --- |
| 1. LiteLLM 相容性 | **本報告：協定層通過**（Anthropic 後端待補測） |
| 2. Skill 載入路徑（`<workdir>/skills/<skill-name>/`） | **已於 2026-08-14 追加實測，6/6 PASS——但提案假設的路徑被證偽，見 §10** |
| 3. 真 Embedding 跨語言召回 | 不在本報告範圍（屬 ADR-013／PDM-011） |

> **注意（2026-08-14 更新）**：PDM-003 §3 寫「前兩項未通過前，PDM-003 不應定案」。第 2 項已於 §10 補測完成，**三個前置項至此全部解除**。但第 2 項的結論**要求同時修正 PDM-003 與 PDM-008 的路徑寫法**（§10.6）——定案時必須連同修正一起採納，不能沿用提案原文的 `<workdir>/skills/<skill-name>/`。

---

## 8. 一句話結論

LiteLLM Proxy 的 Anthropic 相容端點在 tool use ＋ streaming ＋ 每 Run 短效 Virtual Key（含模型範圍與預算強制）路徑下協定層行為正確，`ANTHROPIC_BASE_URL` ／ `ANTHROPIC_AUTH_TOKEN` 注入機制實測可用，**PDM-003 的閘道假設成立、退路不需啟動**；但 `thinking` 透傳與 prompt caching 下的真實 token 成本仍待 Anthropic 憑證補測；Skill 載入路徑前置項已於 §10 補測完成（6/6 PASS），但**實測路徑是 `<workdir>/.claude/skills/<name>/`，不是提案假設的 `<workdir>/skills/<name>/`**。

---

## 9. 重現方式

見 [`spikes/pdm-003-litellm-gateway/README.md`](../../../spikes/pdm-003-litellm-gateway/README.md)。

---

## 10. 前置項 2：Skill 載入路徑實測（2026-08-14 追加）

- Spike 程式碼：[`spikes/pdm-003-litellm-gateway/test_skill_loading.py`](../../../spikes/pdm-003-litellm-gateway/test_skill_loading.py)
- 結果檔：[`results-skill-loading.txt`](../../../spikes/pdm-003-litellm-gateway/results-skill-loading.txt)
- 環境：`claude-agent-sdk` **0.2.137**（Python）、Python 3.14.0，模型呼叫沿用第 2 節的 LiteLLM 官方 image（`sonnet-test` → `openai/gpt-4o-mini`）
- 結果：**6/6 測項 PASS**

### 10.1 要驗什麼

[pdm-proposals.md](./pdm-proposals.md) §3 把 Skill 載入路徑寫為 `<workdir>/skills/<skill-name>/`，§7（PDM-008）的 `claude-agent-sdk` Profile 直接沿用同一句並自行標註「此路徑待 PDM-003 前置 Spike 驗證項 2 確認」。該假設在提案中**無出處**，且 §3 風險表已寫明「若不成立，PDM-003 的載入設計與 PDM-008 的 Profile 同時失效」。

### 10.2 方法

建立一個拋棄式工作目錄，**同時**放入兩種候選佈局，讓每個測項都是同一棵目錄樹上的 A／B 對照：

```text
<wd>/.claude/skills/spike-dot-claude/SKILL.md      # 官方文件的佈局
<wd>/skills/spike-flat-workdir/SKILL.md            # 提案假設的佈局
```

兩份 `SKILL.md` 內容相同（`name`／`description` 齊備，指示「回覆必須以標記字串 `SKILLHUB-MARKER-7Q4Z` 開頭」）。

**判定依據刻意選在模型之外**：SDK 在串流開頭送出 `subtype=init` 的 system message，其 `skills` 陣列即為該 Session 實際發現的 Skill 清單。測項 1–5 只讀這個 init 事件就中止（`aclose()`），**完全不呼叫模型**，因此結論與後端模型無關，可直接轉移到 Anthropic 後端。這與 §3 的可轉移性論證是同一個道理，但更強：這裡連一次推論都不需要。

### 10.3 逐項結果

| # | 測項 | 結果 | 觀測值 |
| --- | --- | --- | --- |
| 1 | `<workdir>/.claude/skills/` 被發現、`<workdir>/skills/` 不被發現 | **PASS** | `setting_sources=["project"]` 下 `init.skills` 含 `spike-dot-claude`，不含 `spike-flat-workdir` |
| 2 | 提案假設的 `<workdir>/skills/` 在任何設定下都不被發現 | **PASS** | `["project"]` 與 `["user","project"]` 兩種設定皆未出現 `spike-flat-workdir` |
| 3 | `setting_sources=[]` 會關閉發現 | **PASS** | 專案 Skill 從 `init.skills` 消失（**這正是 §4 測項 F 的設定，也是為何該測項完全沒碰到 Skill 機制**） |
| 4 | 省略 `setting_sources` 仍會載入 | **PASS** | 未傳該參數時專案 Skill 可見（總數 22，多出的是本機使用者層 `~/.claude/skills/`） |
| 5 | `Skill` 工具存在於 Session 工具清單 | **PASS** | `init.tools` 含 `Skill`；`skills=["spike-dot-claude"]` 時該 Skill 仍列於 `init.skills` |
| 6 | `Skill` 工具能解析並把 `SKILL.md` 內容送進對話 | **PASS** | 工具呼叫 `Skill(skill="spike-dot-claude")`，回傳含 base directory 與全文，標記字串出現在對話中 |

### 10.4 正確的路徑慣例與啟用條件（本節即前置項 2 的答案）

| 項目 | 實測結論 |
| --- | --- |
| **路徑** | `<workdir>/.claude/skills/<skill-name>/SKILL.md`。官方文件另述 `.claude/skills/` 可位於 `cwd` 的**任一上層目錄直到 repo root**，以及使用者層 `~/.claude/skills/` |
| **`<workdir>/skills/<skill-name>/`** | **不被載入。提案的假設不成立。** |
| **啟用條件 1：`cwd`** | `ClaudeAgentOptions.cwd` 必須指向含 `.claude/skills/` 的目錄或其子目錄 |
| **啟用條件 2：`setting_sources`** | **省略即可**（預設載入 user＋project）。但**一旦顯式指定就必須含 `"project"`**，否則專案 Skill 全部消失。Sandbox 內建議顯式寫 `setting_sources=["project"]`——排除 `"user"` 可避免映像或掛載中的 `~/.claude/` 意外污染 Run |
| **啟用條件 3：`skills`** | 省略時「已發現的 Skill 預設啟用、`Skill` 工具可用」；`"all"` 全開、`list[str]` 白名單、`[]` 全關 |
| **啟用條件 4：工具權限** | 若傳了顯式的工具清單，必須包含 `"Skill"`（實測 `init.tools` 確有此工具） |
| **不需要** | 不需要任何程式化註冊 API——官方文件明言 Skill **只能**是檔案系統產物 |

### 10.5 兩層區分：載入機制 vs 模型行為

本 Spike 只需證載入機制，兩者實測結果不同，必須分開讀：

- **載入機制（測項 1–6，已證）**：與後端模型無關。測項 6 以「明確要求使用 `Skill` 工具」的 Prompt 驗證，工具成功解析 Skill 名稱並把 `SKILL.md` 全文送回對話——**發現、解析、內容注入三段全通**。
- **模型行為（記錄項 b，非判定項）**：只給「請幫我產出一份季度庫存報告」這種**符合 description 但未點名 Skill** 的任務時，`gpt-4o-mini` **完全沒有呼叫 `Skill` 工具**（`tools=[]`），直接自行作答，標記字串未出現。這是自主觸發的模型能力差異，不是載入缺陷；生產模型為 `claude-sonnet-5`，其 Skill 自主觸發是 Claude Code 的原生行為。**但這一點在取得 Anthropic 憑證後應與 `thinking` 一併補測**——若 M1 要用「Skill 是否被觸發」作為試跑結果的一部分，自主觸發率是產品訊號，不能假設為 100%。

另記錄一項 SDK 觀察（記錄項 a，非判定項）：`skills=[]` 時 `init.skills` **內容不變**，該陣列反映的是「發現」而非「過濾」。因此 `init.skills` 可作為載入路徑的判定依據，**不可**用來驗證 `skills` 過濾是否生效；`skills` 過濾的實際效果本 Spike 未行為性驗證。

### 10.6 對 PDM-003 與 PDM-008 的結論

**PDM-003 前置項 2：PASS。三個前置項至此全部解除，PDM-003 的定案阻塞消失**（第 1 項的 `thinking` 補測條件不變，見 §7.1）。

但**通過的是機制，不是提案寫的那條路徑**。定案時必須併同修正兩處（本報告不修改 `pdm-proposals.md`，僅回報）：

| 位置 | 提案原文 | 應改為 |
| --- | --- | --- |
| pdm-proposals.md §3（PDM-003 建議段） | 「Skill 載入路徑：`<workdir>/skills/<skill-name>/`，Run 開始前由 Sandbox Worker 以短效物件授權下載展開（SBX-008）」 | 「Skill 載入路徑：`<workdir>/.claude/skills/<skill-name>/`；`query()` 須以 `cwd=<workdir>` 呼叫，`setting_sources` 省略或顯式含 `"project"`」 |
| pdm-proposals.md §7（PDM-008 表格第 3 列，`claude-agent-sdk` Profile 的「安裝位置」） | 「Agent 工作目錄 `skills/<name>/`」＋「此路徑待 PDM-003 前置 Spike 驗證項 2 確認」 | 「Agent 工作目錄 `.claude/skills/<name>/`」，並移除待確認註記；隨附的 `query()` options 片段必須示範 `cwd` 與 `setting_sources`——**這兩個選項是安裝步驟的一部分，只給路徑不足以讓使用者的 Skill 被載入** |

三個連帶影響：

1. **SBX-008（套件展開）的目標路徑改變**：展開點是 `<workdir>/.claude/skills/<name>/`，不是 `<workdir>/skills/`。`.claude` 是隱藏目錄，展開與清理邏輯需一併涵蓋。
2. **PDM-008 的 `claude-code` 與 `claude-agent-sdk` 兩個 Profile 的安裝路徑其實相同**（都是 `.claude/skills/<name>/`，差別只在使用者層 vs 專案／工作目錄層與隨附的驗證方式）。這降低了維護成本，但也意味著兩個 Profile 的差異必須在文案上講清楚，否則使用者會以為是重複選項。
3. **Runtime Image 會夾帶 CLI 內建 Skill**：測項 3 中即使 `setting_sources=[]`，`init.skills` 仍列出 15 個非本機專案的 Skill，且 `init.plugins` 為空——它們是 Claude Code CLI 自帶的內建 Skill，**不受 `setting_sources` 管轄**。對 Sandbox 的兩個後果：(i) 這些 Skill 的 metadata 是 §6.2 那 50K harness 固定開銷的一部分，裁減工具集時應一併評估；(ii) 試跑「使用者的 Skill」時，模型可能改用內建 Skill，構成試跑結果的干擾源——`skills=["<被測 Skill 名>"]` 白名單是自然的緩解，但其過濾效果**尚未經行為性驗證**（見 §10.5 記錄項 a），SBX-002 設計時應實測。

---

## 11. 補測：模型自主觸發 Skill、`skills` 白名單行為性過濾、Prompt Caching（2026-08-14 追加）

- Spike 程式碼：[`spikes/pdm-003-litellm-gateway/test_supplemental.py`](../../../spikes/pdm-003-litellm-gateway/test_supplemental.py)
- 結果檔：[`results-supplemental.txt`](../../../spikes/pdm-003-litellm-gateway/results-supplemental.txt)
- 環境：`claude-agent-sdk` 0.2.137、Python 3.14.0、LiteLLM 1.96.2 官方 image、Postgres 17
- 總計 189 次模型呼叫，實際計費 **$0.93**

### 11.1 為何現在可以測

§6.2、§7.3、§10.5 把三個項目標記為「待取得 Anthropic 憑證後補測」。**負責人已定案模型供應商採 OpenAI API（經 LiteLLM 閘道，ADR-017 架構不變）**，因此這些項目不再需要 Anthropic 憑證——直接在正式後端上測，結論比原先「用 `gpt-4o-mini` 代打」更有效力：**測的就是生產模型本身**。

原本標註「Anthropic 後端會原生透傳／原生支援」的推論段落（§6.1、§10.5）在新前提下**不再適用**，因為後端就是 OpenAI。本節的實測值取代那些推論。

### 11.2 模型別名配置

2026-08 當前 OpenAI 模型系列與牌價（每 1M token，input／output）：GPT-5.6 家族 2026-07-09 GA（Sol $5／$30、Terra $2／$12、Luna $0.20／$1.20）；GPT-5.5 $5／$30；GPT-5.4 家族 Mini $0.75／$4.50、Nano $0.20／$1.25。快取命中的 input 一律以 **1/10 牌價**計。

本次補測經 LiteLLM 別名配置兩個檔位（`config.yaml`）：

| 別名 | 後端模型 | 牌價 input／output／cached-input |
| --- | --- | --- |
| `flagship-test` | `openai/gpt-5.6-sol`（當前主力旗艦） | $5／$30／$0.50 |
| `mini-test` | `openai/gpt-5.4-mini`（mini 級） | $0.75／$4.50／$0.075 |

LiteLLM 1.96.2 的價目表已內建此兩型號，成本記帳正確；`/v1/messages` 路由實測會轉譯到 OpenAI 的 **Responses API**（`api_base` 為 `https://api.openai.com/v1/responses`）。

---

### 11.3 A-1：模型自主觸發 Skill 的實測觸發率

**方法**：工作目錄放一個 `description` 明確涵蓋任務的 Skill（`spike-report`，描述「產出 Acme 倉庫季度 widget 庫存報告」），Prompt 只給符合該描述的任務、**完全不點名 Skill**，觀察模型是否自行呼叫 `Skill` 工具。每個模型 3 次。SKILL.md 內含標記字串，可判定內容是否真的進了對話。

| 模型 | 自主觸發率（本次記錄） | 跨 3 輪獨立完整回合累計 |
| --- | --- | --- |
| `flagship-test`（gpt-5.6-sol） | **0 / 3** | **0 / 9** |
| `mini-test`（gpt-5.4-mini） | **0 / 3** | **0 / 9** |
| （前次 §10.5 記錄項 b，gpt-4o-mini） | 0 / 1 | — |

**對照組（必要，用來排除機制故障）**：同一個工作目錄、同兩個模型，改用「請用 `Skill` 工具呼叫 `spike-report`」的明確 Prompt：

| 對照測項 | 結果 |
| --- | --- |
| A1c `flagship-test` 明確點名 | **PASS**：`tools=['Skill']`、`skill="spike-report"`、標記字串出現 |
| A1c `mini-test` 明確點名 | **PASS**：同上 |

**結論**：載入機制完好（與 §10 一致），**0 觸發率純粹是模型自主行為**。且**換更強的模型並不會改善**——旗艦與 mini 同為 0。

**旗艦模型的失敗模式反而更差**：`gpt-5.6-sol` 三次中有兩次不呼叫 `Skill` 工具，改用 `Glob`／`Read` 自己去翻檔案系統找 `SKILL.md`（另有一次因此耗盡 6 turn 上限而中止）；`gpt-5.4-mini` 則是直接自行作答、不動任何工具。對 Sandbox 試跑而言，旗艦的行為額外消耗 turn 與 token，且引入檔案系統探索這個非預期路徑。

> ⚠️ **這是本次補測最重要的產品訊號**：`pdm-proposals.md` 待補測清單裡「模型自主觸發 Skill 的能力（若 M1 要把『Skill 是否被觸發』當作試跑結果的一部分）」的答案是 **不能**。在 OpenAI 後端上，「Skill 被自主觸發」的基準率實測為 0，**不能作為試跑成功與否的預設判準**。

---

### 11.4 A-2：`skills` 白名單的行為性過濾（§10.5 記錄項 a 的答案）

§10.5 只證明 `init.skills` 反映「發現」而非「過濾」，因此白名單是否真的擋得住**未經驗證**。本次改以行為層驗證：設白名單排除某 Skill 後，**點名要求模型使用該 Skill**，看它是否真的呼叫得到。

| 測項 | 設定 | 結果 | 觀測值 |
| --- | --- | --- | --- |
| A2 排除**專案** Skill | `skills=["spike-decoy"]`，Prompt 點名 `spike-report` | **PASS** | 模型確實嘗試呼叫，harness 回 `<tool_use_error>Skill spike-report is not in this session's skills allowlist</tool_use_error>`；標記字串未洩漏到對話 |
| A3 排除**內建** Skill | `skills=["spike-report"]`，Prompt 點名內建 `code-review` | **PASS** | 同樣被擋：`<tool_use_error>Skill code-review is not in this session's skills allowlist</tool_use_error>` |
| A0 內建 Skill 清單 | `setting_sources=["project"]`、`skills="all"` | — | 專案 2 個 ＋ **內建 15 個**：`deep-research`／`design-sync`／`dataviz`／`update-config`／`verify`／`debug`／`code-review`／`simplify`／`batch`／`fewer-permission-prompts`／`doctor`／`loop`／`claude-api`／`run`／`run-skill-generator` |
| A0 `init.skills` 是否受白名單影響 | `skills=["spike-report"]` | — | **不變**（與 §10.5 記錄項 a 一致，再次確認） |

**三點結論**：

1. **`skills` 白名單的過濾是真的，且對內建 Skill 同樣有效**——§10.6 連帶影響第 3 點提出的緩解方向**成立**，可以寫進 SBX-002 設計。
2. **攔截點在 SDK harness，不在模型**：錯誤字串由 harness 產生，模型只是收到一個 tool error。因此這個結論**與後端模型無關**，換模型不需重測。
3. **但白名單消除的是「被執行」，不是「被看到」**：兩個測項中模型都**先嘗試了呼叫**，代表被排除的 Skill 仍出現在模型可見的清單裡。所以白名單**不會**降低 harness 的 token 開銷，也不會完全消除注意力干擾——只是保證干擾 Skill 的內容不會進入對話。

---

### 11.5 B：Prompt Caching 與 300K input 上限的校正

#### 11.5.1 OpenAI 目前的快取機制（查證）

- **自動快取**，無需宣告：prompt 前綴 **≥ 1024 token** 即自動快取，以前綴比對。
- **2026-05-29 起預設保留 24 小時**（GPT-5.5 及更新的型號只有 24h 模式，無短效模式）。
- **cached input 以 1/10 牌價計**（GPT-5 世代；早期型號為 50% 折扣）。
- 用量欄位為 `usage.prompt_tokens_details.cached_tokens`。

#### 11.5.2 LiteLLM 是否透傳 cached token 用量欄位

以同一段約 27K token 的前綴，各打兩次（第一次冷、第二次熱；前綴帶 per-run nonce 以確保第一次必為 miss）：

| 路由 | 冷 | 熱 | 判定 |
| --- | --- | --- | --- |
| `/v1/chat/completions`（OpenAI 原生，對照組） | `prompt=27,042`、`cached_tokens=0` | `prompt=27,042`、**`cached_tokens=26,880`**（99.4%） | 正常透傳 |
| **`/v1/messages`（Agent SDK 實際走的路由）** | `usage={'input_tokens': 27041, 'output_tokens': 5}` | `usage={'input_tokens': 27041, 'output_tokens': 5}` | **不透傳** |

> ⚠️ **實測結論：LiteLLM 1.96.2 在 `/v1/messages` 路由上完全不輸出 cache 相關用量欄位。** 回應的 `usage` 只有 `input_tokens` 與 `output_tokens`，`cache_read_input_tokens` 與 `cache_creation_input_tokens` **兩個欄位根本不存在**（不是 0，是缺欄）。連帶地，Agent SDK 的 `ResultMessage.usage.cache_read_input_tokens` 恆為 **0**，`model_usage[...].cacheReadInputTokens` 亦為 0。這與 LiteLLM 上游已知的 Anthropic 格式 usage 正規化缺陷一致。

**但這不是計費問題。** 追查 LiteLLM 自己的 `/spend/logs` 記錄，同一組冷／熱請求：

| 路由 | 冷請求計費 | 熱請求計費 | 折扣 |
| --- | --- | --- | --- |
| `/v1/chat/completions` | $0.020300 | $0.002155 | 9.4× |
| **`/v1/messages`** | $0.020306 | **$0.002161** | **9.4×** |

**LiteLLM 內部的成本計算有正確套用快取折扣，兩個路由一致。** 壞掉的只有兩處：(i) 回給客戶端的 `usage` 缺欄位；(ii) spend log 的 `cache_read_input_tokens` 欄位為 `null`。

> **對 Virtual Key 預算計費準確性的結論（修正原先的疑慮）**：**預算金額是準的**，`max_budget` 不會因為快取而錯扣。**受損的是可觀測性**：平台拿不到 cache 命中率，因此 (a) Trace 無法呈現快取效益、(b) 無法從 usage 欄位反推「這次 Run 的 input 有多少是重複的 harness 前綴」、(c) 若 ADR-017 的 Langfuse 成本歸因依賴 cache 欄位，該欄位在此路由上不可用。

#### 11.5.3 多輪對話的實際 input 消耗曲線

**量測基準改用閘道自身的計費記錄**，而非 SDK 回報值：實測 `ResultMessage.usage.input_tokens` 在含工具呼叫的輪次上不穩定（有時回報最後一次 API 呼叫、有時回報加總），而閘道記錄才是 Virtual Key 預算真正扣減的依據。

**(a) 5 輪真實 Agent SDK 對話（`mini-test`，純問答不呼叫工具）**

| 輪 | 閘道記錄 input tokens | 實際計費 |
| --- | --- | --- |
| 1 | 19,219 | $0.001726 |
| 2 | 19,259 | $0.001756 |
| 3 | 19,298 | $0.001754 |
| 4 | 19,331 | $0.001792 |
| 5 | 19,367 | $0.001815 |
| **合計** | **96,883** | **$0.0092** |

未快取應為 $0.0727 → **實效折扣 87%**。曲線幾乎持平：每輪只增加約 40 token（＝上一輪的一問一答），**harness 固定前綴約 19.2K 才是主體**，且該前綴每輪都完整命中快取。

另跑一次 7 輪對話交叉驗證，SDK 端累計 `inputTokens = 135,803`，平均約 19.4K／輪，與上表一致。

> 判讀注意：結果檔中 B3 的 `t7:in=0` 是該次記錄裡 SDK 少送了一則 `ResultMessage` 所致（同一測項的另外兩次執行皆為 7 筆完整、平均 19.34K／輪）。因此 B3 該列的 `avg_per_turn=16561` 與 `turns_until_300K=18.1` **不可採用**；本節的輪數結論一律以 B4／B5 的閘道計費記錄為準。這正是改用閘道記錄作為量測基準的原因。

> 附帶的成本結構觀察：因為快取保留 24 小時且 harness 前綴跨 Run 完全相同，**第二次以後的 Run 會直接命中前一次 Run 留下的快取**。上表連第 1 輪都已是折扣價，正是這個效應。這對免費額度成本模型是有利的，但也代表「首次 Run」與「後續 Run」的單位成本差約 8 倍，成本模型不應只用單一均值。

**(b) 有無工具呼叫的差距（決定性因素）**

| 情境 | 閘道記錄的 API 呼叫 | 該輪 input 合計 |
| --- | --- | --- |
| 純對話，不呼叫工具 | 2 次：405 ＋ 19,215 | **19,620** |
| 含 **1 次** `Skill` 工具呼叫 | 3 次：431 ＋ 19,242 ＋ 19,415 | **39,088**（**1.99×**） |

原因很直接：每一次工具結果回填都要把整個 harness 前綴重送一次。**每多一次工具呼叫，該輪 input 就多約 19.4K。**

（附帶觀察：每輪都有一次約 400–430 token 的小型輔助呼叫，屬 harness 固定行為。）

#### 11.5.4 「300K input 上限實際夠幾 turn」

| 每輪型態 | 每輪 input | **300K 可跑輪數** |
| --- | --- | --- |
| 純對話，0 次工具呼叫 | ~19.6K | **約 15 輪** |
| 每輪 1 次工具呼叫 | ~39.1K | **約 7.7 輪** |
| 每輪 2 次工具呼叫 | ~58.5K（外推） | **約 5 輪** |

§6.2 原本量到的 50,139／輪落在「1–2 次工具呼叫」之間，因此原估的 **5–6 輪對「工具密集的輪次」是對的**，但對純對話輪**低估了約 2.5 倍**。

> ⚠️ **最關鍵的校正：prompt caching 完全不會增加 300K token 上限能買的輪數。**
> 快取省的是**錢**，不是 **token 數**——命中快取的 token 一樣計入 `input_tokens`。§6.2 的推測（「屆時實際可用輪數會顯著上升」）**不成立**，實測 87% 的成本折扣對應的輪數增益是 **0**。
>
> **真正被快取改變的是「300K token 上限」與「$1.80 預算上限」之間的關係。** PDM-005 §5.2 把 token 上限寫成由 LiteLLM Virtual Key **預算（金額）**強制。在 87% 折扣下，同一筆金額預算能買到的 token 數變成約 **7–8 倍**：以 `$1.80` 為 `max_budget`，實際要跑到約 **2.4M** input token 才會被擋，而不是 300K。**兩者已嚴重脫鉤，金額預算目前無法代理 token 上限。**

---

### 11.6 本節對定案的建議

#### (1) 對 PDM-005 300K input 上限的校正建議數字

| 項目 | 建議 |
| --- | --- |
| **300K input 上限本身** | **維持 300K，並解除「未經驗證」標記**——已實測。但必須改寫為明確的 **token 上限**，且移除「待 prompt caching 補測後校正」的註記（本節即為該補測，結論是不需上調） |
| **應同時寫入允收準則的換算** | 300K input ≈ **純對話 15 輪**／**每輪 1 次工具呼叫 7.7 輪**／**每輪 2 次工具呼叫 5 輪**。單一輪數沒有意義，必須帶「每輪工具呼叫次數」這個參數 |
| **harness 固定開銷** | 由 §6.2 的「約 50K／輪」修正為 **約 19.4K／次 API 呼叫**；一輪的 input ≈ 19.4K ×（1 ＋ 該輪工具呼叫次數）。§6.2 的 50K 是「一輪含工具呼叫」的合計值，不是單次呼叫的固定開銷——**這兩個數字在 `pdm-proposals.md` §3(c) 與 §5.2 被當成同一件事引用，應分開** |
| **強制機制** | **不能只用 `max_budget` 代理 token 上限**（脫鉤約 7–8 倍）。建議 Virtual Key 同時帶 `max_budget`（依**快取後**實價編列，非牌價反推）＋ `tpm_limit`（§6.3 已建議）；**token 級的 300K 上限改由 Go Worker 依閘道回報的 `input_tokens` 累計強制**——該欄位在 `/v1/messages` 上實測可用（缺的只有 cache 欄位） |
| **成本結構翻轉（提醒，不在本節裁定）** | 快取後 input 幾乎免費，單 Run 成本變成 **output 主導**。以 `gpt-5.6-sol` 計，300K cached input ≈ $0.20、60K output ＝ $1.80。`pdm-proposals.md` §5.2／§8.2 現行以 `claude-sonnet-5` 牌價、input:output ≈ 1:1 編列的成本模型，在供應商改為 OpenAI 後需整體重算——**這超出本節範圍，屬 PDM-005／成本試算的工作** |

#### (2) 試跑預設模型的建議

**建議預設採 mini 級（`gpt-5.4-mini`）。** 理由是實測而非成本偏好：

1. **自主觸發率上旗艦沒有任何優勢**：`gpt-5.6-sol` 與 `gpt-5.4-mini` 同為 **0/9**。原本「用更強的模型換自主觸發率」的假設**被證偽**，因此升級旗艦買不到這個能力。
2. **明確點名時兩者都 PASS**：試跑要驗的「Skill 內容能否正確驅動模型」在 mini 級上完全成立。
3. **旗艦的失敗模式對 Sandbox 更不利**：`gpt-5.6-sol` 會改用 `Glob`／`Read` 探索檔案系統（三次中兩次），額外消耗 turn 與 input token，其中一次耗盡 turn 上限而中止。
4. **成本差 6.7 倍**（input $0.75 vs $5、output $4.50 vs $30）。以 11.5.4 的每輪用量，同一組 300K／60K 上限在 mini 級的成本約為旗艦的 1/6.7。

**附帶的產品層要求（不是模型選擇，但同屬此結論）**：既然自主觸發基準率為 0，**試跑的預設 Prompt 必須明確指示呼叫被測 Skill**，或把「明確點名」設為預設試跑模式。**「Skill 是否被自主觸發」不可作為試跑成功判準**；若要作為產品訊號呈現，必須另行標示為「探索性指標」並註明基準率為 0。

#### (3) `skills` 白名單能否作為內建 Skill 干擾的有效緩解

**可以，建議採用，並作為 SBX-002 的預設設定。** 實測支撐：

- 被排除的 Skill（**專案的與內建的都試過**）在模型嘗試呼叫時由 harness 直接回 `tool_use_error`，內容不會進入對話。§10.6 連帶影響第 3 點提出的緩解方向**成立**。
- 攔截點在 SDK harness 而非模型，**與後端模型無關**，換模型不需重測。
- 建議寫法：`skills=["<被測 Skill 名>"]`（只放被測 Skill），搭配 §10.4 的 `cwd` 與 `setting_sources=["project"]`。

**三點限制必須一併寫入**：

1. **`init.skills` 不反映白名單**（再次確認）。驗證白名單是否生效**只能用行為測試**，不能讀 `init.skills`。
2. **白名單擋的是執行、不是曝光**：被排除的 Skill 仍在模型可見清單中（實測模型會先嘗試呼叫才被擋），因此白名單**不會降低 harness 約 19.4K 的固定開銷**，也不能完全排除注意力干擾。若要削減 token 成本，仍需 §6.2 提到的「裁減 Runtime Image 工具集」那條獨立路徑。
3. **會多一次無效的工具往返**：模型嘗試呼叫被擋的 Skill 時，該輪會多一次 API 呼叫（＝多約 19.4K input）。在 300K 上限下這不可忽略，Trace 應把 `tool_use_error` 的白名單拒絕獨立標示，避免使用者誤判為 Skill 本身有問題。

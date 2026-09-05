# 互動創作 vs 單次生成的量測（harness 就位，2026-09-06；**2026-09-06 跑了五次——a～c 不含 Run、d／e 含 Run 階段——見 [report.md](report.md)**）

02:GEN-012 的證據條與 05 R-45 的量測門檻要的是一份分布：同 15 個任務，一次跑互動創作（15 場多輪會話：文字、流程圖、參考各 5 場），一次跑單次生成，兩邊對比。這個目錄就是把它變成分布所需的一切，除了那筆錢——啟動 `apps/llm` 對真實閘道那一步要負責人親自起（代理權限擋下了，這裡也一樣擋）。

**Harness**：[`creation_measure_batch_test.go`](../../../../../apps/platform/internal/entrypoint/api/apiserver/creation_measure_batch_test.go) 的 `TestCreationMeasureFifteenSessionsAgainstSingleShot`。任務語料沿用 [`gen-modes-batch/corpus.json`](../gen-modes-batch/corpus.json)：文字任務取 `reference[0..4]` 的描述、流程圖任務取 `diagram[0..4]`、參考任務取 `reference[5..9]`（連同它自己的參考 Skill）。

## 這份量測量的是什麼、量不到什麼

會自動記錄的：格式是否通過（`skillpkg.Validate` 沒擋）、每場會話的輪數／模型呼叫次數／工具呼叫次數／自動確認次數／澄清次數、成本（`Snapshot.SpentUSD`，未知時標 `usage_unknown`）、每次模型呼叫的秒數與 p50/p95、最終狀態、是否產出草稿與是否被擋、驗收條件數、是否建立了 Test Case。

現在會自動記錄的（設定了下面的「跑法（含 Run 階段）」才會有）：**任務達成（`met`）**——materialize 出候選 Skill 後對它自己的 Test Case 跑一次真的 Run（同 `gen009_baseline_test.go` 的呼叫方式：真物件儲存、真 Sandbox provider、真 Judge），等到終態與評估結果，`met` = 評估的 `overall` 是否為 `"met"`；接著把這個 Run 用 `attach_run` 餵回會話，再跑一步看模型是否修改了草稿（`revised_after_run`）。評估沒能跑到終態時 `met` 留 `null`，原因記在 `met_note`。`results.json` 的 `summary` 多了 `met_count`／`met_denominator`（達成數／有評估結果的場數）。

量不到的：
- **真人願意採用（`kept`）** 需要一個人讀過產出的 SKILL.md 判斷值不值得用；這個 harness 只負責把 30 份（15 互動＋15 單次）dump 出來給人讀。

`results.json` 的每筆互動會話列還留了 `met_by_owner`／`kept_by_owner` 兩個欄位：`met_by_owner` 給一個人覆核／推翻自動填的 `met`（值仍是 `null`，只在需要覆核時填），`kept_by_owner` 一樣要人填——填 `kept_by_owner`（與需要覆核時的 `met_by_owner`）是這份文件下面「跑完之後」那一節的事，不是這個測試的事。

## 跑法（約數美元，15 場會話 × 最高 $1 上限，實際多半個位數美元）

```
# 1) 畫流程圖（在任一終端，不花錢）
pwsh docs/plans/mvp/m5/gen-modes-batch/draw.ps1 -Corpus docs/plans/mvp/m5/gen-modes-batch/corpus.json -OutDir <scratch>/diagrams

# 2) 另一個終端：啟動 apps/llm，指向真實閘道（會花錢；只有負責人能起）
task dev:llm

# 3) 再一個終端，從 repo root：用 with-service-key.mjs 簽一把限額 Virtual Key 給這個 go test 進程，
#    當作互動創作每一步要用的 X-Creation-Gateway-Key
node tools/cleanmode/with-service-key.mjs -- env \
  CREATION_MEASURE_CORPUS=<repo 絕對路徑>/docs/plans/mvp/m5/gen-modes-batch/corpus.json \
  CREATION_MEASURE_DIAGRAMS=<scratch>/diagrams \
  CREATION_MEASURE_OUT=<scratch>/out \
  SKILLHUB_E2E_LLM_URL=http://127.0.0.1:8000 \
  SKILLHUB_TEST_DATABASE_URL=<測試庫> \
  SKILLHUB_REQUIRE_DB=1 \
  go -C apps/platform test ./internal/entrypoint/api/apiserver -run TestCreationMeasureFifteenSessionsAgainstSingleShot -timeout 90m -v
```

（`with-service-key.mjs` 的用法是 `node tools/cleanmode/with-service-key.mjs -- <要啟動的命令> [參數…]`；上面用 `env NAME=value... go test ...` 把環境變數和真正要跑的命令一起包進 `--` 後面那段。）

沒有設定五個環境變數（`CREATION_MEASURE_CORPUS`、`CREATION_MEASURE_DIAGRAMS`、`CREATION_MEASURE_OUT`、`SKILLHUB_E2E_LLM_URL`、`LITELLM_API_KEY`）的任何一個，測試會直接 SKIP 並說明會花錢；**代理不會跑這個測試**。

跑完把 `<scratch>/out/results.json` 與 30 份 `*.SKILL.md`（`<id>-interactive.SKILL.md`、`<id>-single.SKILL.md`）搬進這個目錄。

### 跑法（含 Run 階段，選配——多花 Sandbox 與 Judge 那筆錢）

不設定就完全不影響上面的跑法。要讓 `met` 自動填，另外設定：

```
SKILLHUB_E2E_SANDBOX_URL / SKILLHUB_E2E_SANDBOX_TOKEN
OBJSTORE_ENDPOINT / OBJSTORE_ACCESS_KEY / OBJSTORE_SECRET_KEY
SKILLHUB_E2E_PUBLIC_HOST
SKILLHUB_MODEL_GATEWAY_URL / SKILLHUB_MODEL_GATEWAY_KEY
```

這五組就是 `gen009_baseline_test.go`（GEN-009 ③）已經在用的那一組，起 Postgres／SeaweedFS／LiteLLM／`sandboxd` 三個程序的完整配方（含跨平台限制、映像版本與容器內跑測試的理由）不重抄一份，見 [automation.md〈三個程序〉](../../../../development/automation.md#三個程序) 與其後「測試程序要跑在容器裡」兩節。任一變數沒設，這個測試就照舊只到 `candidate_ready`，`met` 全部是 `null`。

**2026-09-06 跑通這段時另外撞到的五件事**（run d／e 的配方，逐字在 [report.md §5](report.md)）：

1. **`DEV_LOGIN=1`**：本機 sandboxd 是 runc，`execution.Match` 只在開發部署接受 `container` 隔離；少了它 14 場全 422。
2. **`with-service-key.mjs` 會把 `SKILLHUB_MODEL_GATEWAY_KEY` 從子程序環境拿掉**——那是它存在的目的（apps/llm 不得拿 master key）。但 Run 階段的 Go 要用它簽每場 Run 的短效 Virtual Key，所以要用 `-e SKILLHUB_MODEL_GATEWAY_KEY="$K"` 明寫（值只在 shell 變數，不落地），不能靠 `-e VAR` 轉送。
3. **`apps/llm` 要 `--host 0.0.0.0`**，容器內以 `http://host.docker.internal:8000` 呼叫它（`--network container:skillhub-postgres-1` 裡解析得到）。
4. **harness 的 River worker 沒註冊 `creation_step`**：log 會刷「Unhandled job kind creation_step」，那是 worker 撿到會話排的工作而不認得；會話由 harness 自己逐步推進，不受影響。
5. **每場的 Run 不會回頭讀會話的 brief**：Test Case 的 prompt 是什麼，Judge 就拿什麼判——run d 用 brief 當 prompt，代理只會反問「請貼逐字稿」，五條驗收條件四條 `undetermined`。這是 `sample_input` 進契約的原因（run e 起）。

## 跑完之後（負責人的事，這個 harness 做不到）

1. 找人讀完 30 份 SKILL.md（15 互動＋15 單次），判斷願不願意採用，填 `kept_by_owner`。
2. 對照 05 R-45 的門檻：格式通過 ≥ 14/15、任務達成 ≥ 9/15、真人採用 ≥ 12/15（單次基線 15/19，多輪不得更差）、每場成本中位 ≤ $0.50、每次模型呼叫等待 p50 ≤ 60s、p95 ≤ 90s（`results.json` 的 `thresholds`／`summary` 已經算好 `format_pass`／成本中位／p50／p95／`met_count`／`met_denominator`；沒跑 Run 階段時後兩者是 0，`kept` 仍要人數）。看到某筆自動 `met` 判斷有問題（例如評估用的驗收條件本身有爭議），在 `met_by_owner` 覆寫並說明。
3. 把跑出來的數字寫回 `05-pending-rulings.md` R-45（或它的後續紀錄），不要回頭改這份 README。

**尚未跑。**

三個路徑都用**絕對路徑**：`go -C apps/platform` 會把相對路徑從套件目錄解析（第一次實跑就撞到 `open …corpus.json: cannot find`）。第二把金鑰要帶 `SKILLHUB_SERVICE_KEY_ALIAS=<不同名字>`，LiteLLM 拒絕重複的 key alias；預算用 `SKILLHUB_SERVICE_KEY_BUDGET_USD`（0～20）。

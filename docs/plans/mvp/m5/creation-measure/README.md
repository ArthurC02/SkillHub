# 互動創作 vs 單次生成的量測（harness 就位，2026-09-06；**尚未跑**）

02:GEN-012 的證據條與 05 R-45 的量測門檻要的是一份分布：同 15 個任務，一次跑互動創作（15 場多輪會話：文字、流程圖、參考各 5 場），一次跑單次生成，兩邊對比。這個目錄就是把它變成分布所需的一切，除了那筆錢——啟動 `apps/llm` 對真實閘道那一步要負責人親自起（代理權限擋下了，這裡也一樣擋）。

**Harness**：[`creation_measure_batch_test.go`](../../../../../apps/platform/internal/entrypoint/api/apiserver/creation_measure_batch_test.go) 的 `TestCreationMeasureFifteenSessionsAgainstSingleShot`。任務語料沿用 [`gen-modes-batch/corpus.json`](../gen-modes-batch/corpus.json)：文字任務取 `reference[0..4]` 的描述、流程圖任務取 `diagram[0..4]`、參考任務取 `reference[5..9]`（連同它自己的參考 Skill）。

## 這份量測量的是什麼、量不到什麼

會自動記錄的：格式是否通過（`skillpkg.Validate` 沒擋）、每場會話的輪數／模型呼叫次數／工具呼叫次數／自動確認次數／澄清次數、成本（`Snapshot.SpentUSD`，未知時標 `usage_unknown`）、每次模型呼叫的秒數與 p50/p95、最終狀態、是否產出草稿與是否被擋、驗收條件數、是否建立了 Test Case。

量不到的：
- **任務達成（`met`）** 需要把候選 Skill 接上一次 Run，讓 `GEN-009 ③④`／評估流程判定；這個 harness 只到「草稿驗證通過」為止，不跑 Sandbox，也不叫 Judge。
- **真人願意採用（`kept`）** 需要一個人讀過產出的 SKILL.md 判斷值不值得用；這個 harness 只負責把 30 份（15 互動＋15 單次）dump 出來給人讀。

`results.json` 的每筆互動會話列都留了 `met_by_owner`／`kept_by_owner` 兩個欄位，值是 `null`——填這兩欄是這份文件下面「跑完之後」那一節的事，不是這個測試的事。

## 跑法（約數美元，15 場會話 × 最高 $1 上限，實際多半個位數美元）

```
# 1) 畫流程圖（在任一終端，不花錢）
pwsh docs/plans/mvp/m5/gen-modes-batch/draw.ps1 -Corpus docs/plans/mvp/m5/gen-modes-batch/corpus.json -OutDir <scratch>/diagrams

# 2) 另一個終端：啟動 apps/llm，指向真實閘道（會花錢；只有負責人能起）
task dev:llm

# 3) 再一個終端，從 repo root：用 with-service-key.mjs 簽一把限額 Virtual Key 給這個 go test 進程，
#    當作互動創作每一步要用的 X-Creation-Gateway-Key
node tools/cleanmode/with-service-key.mjs -- env \
  CREATION_MEASURE_CORPUS=docs/plans/mvp/m5/gen-modes-batch/corpus.json \
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

## 跑完之後（負責人的事，這個 harness 做不到）

1. 對每場互動會話的候選 Skill（`Snapshot.Candidate` 非空的那幾筆）跑一次 Run，用評估流程判定 `met`，把結果填進 `results.json` 對應列的 `met_by_owner`。
2. 找人讀完 30 份 SKILL.md（15 互動＋15 單次），判斷願不願意採用，填 `kept_by_owner`。
3. 對照 05 R-45 的門檻：格式通過 ≥ 14/15、任務達成 ≥ 9/15、真人採用 ≥ 12/15（單次基線 15/19，多輪不得更差）、每場成本中位 ≤ $0.50、每次模型呼叫等待 p50 ≤ 60s、p95 ≤ 90s（`results.json` 的 `thresholds` 與 `summary` 兩個區塊已經算好 `format_pass`／成本中位／p50／p95，`met`／`kept` 那兩項自己數）。
4. 把跑出來的數字寫回 `05-pending-rulings.md` R-45（或它的後續紀錄），不要回頭改這份 README。

**尚未跑。**

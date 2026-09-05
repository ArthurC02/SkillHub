# 兩種新輸入模式各 20 段的量測（harness 就位，2026-09-05；**尚未跑**）

[report-generate-modes.md](../report-generate-modes.md) §6 第一條寫的是「三次各一筆，不是分布」。這個目錄是把它變成分布所需的一切，**除了那筆錢**：語料、畫圖的腳本、harness 的位置與跑法。它在 2026-09-05 收尾批就位，同一天沒有跑——啟動 `apps/llm` 對真實閘道那一步要負責人親自起（代理權限擋下了，而那正是它該擋的）。

| 檔案 | 內容 |
| --- | --- |
| `corpus.json` | `diagram`：20 張流程圖的節點（14 張中文、6 張英文；4～9 個節點；10 張直線、7 張一個判斷且「否」跳段、3 張兩個判斷），每個節點帶一個 `key`——照著圖寫出來的文件幾乎一定會逐字出現的詞（數字、專有名詞、具體物件，不放動詞）。`reference`：20 組「任務描述＋一個參考 Skill」，參考刻意選相鄰但不同的領域，每份參考帶 3 個 `markers`——只會出現在它本文裡的怪句（「每欄最多五條」那種）；16 份中文本文、4 份英文 |
| `draw.ps1` | 用 System.Drawing 把 `corpus.json` 的 `diagram` 畫成 `<id>.png`／`<id>.jpg`（四種字型輪替、兩種底色、四張 JPEG）。圖不進 repo：腳本是決定性的，重畫即得 |

**Harness**：[`generate_modes_batch_test.go`](../../../../../apps/platform/internal/entrypoint/api/apiserver/generate_modes_batch_test.go) 的 `TestTheTwoNewerModesTwentyTimesEach`。每段一列：驗證通過／被擋、嘗試次數、成本、`generation_inputs` 有沒有落地，加兩個機器檢查——流程圖那 20 段數 `key` 在產出裡出現幾個（模型有沒有讀到那個節點），參考那 20 段數 `markers` 被逐字抄了幾個、以及參考本文與產出最長的共同字串有幾個 rune（ADR-066 決策 2 說的「讀過不等於抄」量起來長什麼樣）。

**跑法**（約 US$0.16～0.3，40 段各一次）：

```
pwsh docs/plans/mvp/m5/gen-modes-batch/draw.ps1 -Corpus docs/plans/mvp/m5/gen-modes-batch/corpus.json -OutDir <scratch>/diagrams
task dev:llm                       # 另一個終端；替 apps/llm 簽一把限額 Virtual Key，永不給 master key
GEN_MODES_CORPUS=docs/plans/mvp/m5/gen-modes-batch/corpus.json GEN_MODES_DIAGRAMS=<scratch>/diagrams GEN_MODES_OUT=<scratch>/out \
SKILLHUB_E2E_LLM_URL=http://localhost:8000 SKILLHUB_TEST_DATABASE_URL=<測試庫> SKILLHUB_REQUIRE_DB=1 \
go -C apps/platform test ./internal/entrypoint/api/apiserver -run TestTheTwoNewerModesTwentyTimesEach -timeout 60m -v
```

跑完把 `results.json` 與 20＋20 份 `SKILL.md` 放進這個目錄（同 [`gen009-round-d/`](../gen009-round-d/README.md) 的理由：它們原本只活在一個會被 `DROP SCHEMA` 的資料庫裡），再把數字寫進 `report-generate-modes.md` 的 §7。

**它量的是什麼、不是什麼**：`key` 出現了說的是「節點被看到」，不是「那一步寫對了」；`markers` 沒被抄說的是「沒有逐字照搬」，不是「沒有借」。試跑與人的判定仍然是 `GEN-009` ③④ 的事，這裡一個都沒有。

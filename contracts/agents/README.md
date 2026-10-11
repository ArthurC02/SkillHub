# 平台 Agent 的固定考題

每個平台 Agent 一個 `<agent>.evals.json`，檔名就是它登記的名字。一份考題是一份工具回答的事實快照（`snapshot`），加上一組報告（`cases`），每份標明檢查該不該讓它通過（`passes`）。

兩邊讀同一份檔，各自守一半：

- **Go**（`apps/platform/internal/product/operations`）：每個登記的 Agent 都要有考題；結果檢查對每份報告的判定要等於 `passes`。
- **Python**（`apps/llm/tests/test_agent_evals.py`）：考題與指示一一對應；每份 `passes: true` 的報告都要符合那個 Agent 宣告給模型的結果格式。

新增一個 Agent：Go 端登記定義與工具，`apps/llm/src/skillhub_llm/agents/` 加一個指示模組並登記名字，這裡加一份考題。少了任何一份，上面兩組測試會擋。

考題跑在測試替身上，不呼叫模型、不花錢；為什麼這樣設計見[平台 Agent](../../docs/adr/README.md#平台-agent)。

---
paths:
  - "db/migrations/**"
  - "db/queries/**"
  - "db/query-owners.yaml"
---

一張新表、一個新欄位或一條新約束，完成定義不只正確的型別與約束，還包括「沒讀過程式的代理只憑檔案就讀得懂」：先讀 [資料所有權與核心基礎設施](../../docs/adr/README.md#資料所有權與核心基礎設施) 的那一條決策（`COMMENT ON`、owner 是 Registry 認得的 Bounded Context 或 identity 檔登記的 Generic 套件、Bounded Context 表的約束有 Registry Rule、retention／immutable 宣告）。

會擋的檢查：`tools/data-readiness/measure_readiness.py --fail-on-regression`（CI 對 `baseline.json` 不得退步，進步就把基線一起提上去）、`comment-lint`（SQL 裡不寫 `--` 註解，意義寫進 `COMMENT ON`）、`task gen:check`（表註解會進 Go 型別，改完先 `task gen:sql`）。

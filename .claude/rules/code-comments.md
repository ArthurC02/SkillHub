---
paths:
  - "**/*.go"
  - "**/*.ts"
  - "**/*.tsx"
  - "**/*.js"
  - "**/*.mjs"
  - "**/*.py"
  - "**/*.sql"
  - "**/*.yml"
  - "**/*.yaml"
  - "**/*.toml"
  - "**/*.sh"
  - "**/Dockerfile*"
  - ".env.example"
---

寫註解之前先看根 `AGENTS.md`〈慣例〉的「程式碼註解預設不寫」：用命名說意圖；只有艱難、特殊的演算法或程式碼區塊才寫，一個區塊最多 3 行。施工日誌與決策說明（來龍去脈、理由、編號、日期、實測數字）一律不准，寫進 commit message 或 ADR。

寫完跑 `go -C tools/devctl run . comment-lint <你改的路徑>`；`automation-check` 的 `comment-budget` 會擋。

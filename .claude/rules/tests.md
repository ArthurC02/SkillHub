---
paths:
  - "**/*_test.go"
  - "**/*.test.ts"
  - "**/*.test.tsx"
  - "**/*.test.mjs"
  - "**/*.spec.ts"
  - "**/test_*.py"
  - "**/conftest.py"
  - "**/providertest/**"
---

寫、改或審查測試之前，先載入 `istqb-test-design` 技能（`.claude/skills/istqb-test-design/SKILL.md`）：從產品程式推出等價類、邊界（界上與剛過界）、決策表、狀態轉移與負面案例，每條新測試都要證明會紅（根 `AGENTS.md`〈開發自動化〉第 9 條）。

這一區會擋你的檢查：

- 需要資料庫的 Go 測試在沒有 `SKILLHUB_TEST_DATABASE_URL` 時跳過——那個 ok 不算跑過；跑的時候加 `-count=1`，不然會重播沒接資料庫時的快取結果。會因缺環境停用的套件必須認 `SKILLHUB_REQUIRE_DB`／`SKILLHUB_REQUIRE_OBJSTORE`（`automation-check` 的 `require-db-guard`／`require-objstore-guard`）。
- `infra/images/runtime-agent-sdk/` 底下的 Dockerfile，以及它 `COPY` 進映像的檔一改，`Runtime Image` 的 I-05 就要求同一次推送升 `ARG IMAGE_VERSION` 並補 `UPGRADES.md` 一節；那次推送會發佈新的映像標籤，先問負責人。`run.test.mjs` 沒被複製進映像，改它不觸發。pre-push hook（`devctl preflight --hook`）推送前就會用同一段判斷先擋一次。

---
paths:
  - ".claude/**"
  - "**/AGENTS.md"
  - "**/CLAUDE.md"
---

這裡是 harness 本身。規則在根 `AGENTS.md`〈分區指標與攔阻〉，機器在 `automation-check` 的 `harness` 檢查：技能不得引用 `docs/`、ADR 編號或需求 ID；角色必須指定 `model`（不得 fable／inherit）；根 `AGENTS.md` 有大小上限。

新增一個區域走那一節的三步配方，**不新增角色**。deny 規則加完要親自撞一次。

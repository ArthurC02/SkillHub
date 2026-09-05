---
paths:
  - ".claude/**"
  - "**/AGENTS.md"
  - "**/CLAUDE.md"
---

這裡是 harness 本身。入口在根 `AGENTS.md`〈分區指標與攔阻〉，跨工具流程契約在 `docs/development/agent-instructions.md`，規則本體與新增區域的三步配方在 `docs/development/automation.md`〈Harness〉，機器在 `automation-check` 的 `harness` 檢查：技能不得引用 `docs/`、ADR 編號或需求 ID；角色必須指定 `model`（不得 fable／sol／inherit；預設是各角色 frontmatter 的低階模型，簡報依任務難度升級）；根 `AGENTS.md` 有大小上限。

新增一個區域走那三步配方，**不新增角色**。deny 規則加完要親自撞一次。

# Agent Harness：導覽、洞見、工序與功法

本目錄是人工維護 harness 的導覽與方法手冊。**現行規則不在這裡**：規則以 [`automation.md`〈Harness〉](../automation.md)、根 [`AGENTS.md`](../../../AGENTS.md) 與[開發者指示](../agent-instructions.md)為準。本目錄說明如何從規則找到來源、同步可攜角色與技能，並以唯讀檢查確認安排確實生效。

| 檔 | 回答的問題 |
| --- | --- |
| [01-journey.md](01-journey.md) | 從規則、來源、同步到唯讀驗證的目前流程 |
| [02-insights.md](02-insights.md) | 可操作的設計與驗證原則 |
| [03-procedure.md](03-procedure.md) | 工序：在一個 repo 裡建立或擴充 harness 的可重複步驟與檢核表 |
| [04-techniques.md](04-techniques.md) | 功法：過程中反覆用到、換一個 repo 也成立的具體手法 |

## 一頁摘要

**harness 是什麼**：讓 coding agent 在這個 repo 裡「該讀的會讀到、不該做的做不了、做完的能證明」的那一層東西。它有四層，每層只放一種東西：

| 層 | 放什麼 | 住哪 | 誰吃得到 |
| --- | --- | --- | --- |
| 送達 | 「先讀哪一份、會被哪個檢查擋」的指標 | 根 `AGENTS.md` 的分區表、各目錄 `AGENTS.md`、`.claude/rules/` | 前兩者所有工具；rules 只有 Claude Code |
| 角色 | 按風險切的三個子代理：writer／verify／mutation | `.claude/agents/` | Claude Code |
| 程序 | 換一個 repo 還成立的做法 | `.claude/skills/` | Claude Code |
| 編排 | 固定形狀的多代理程序：什麼平行、哪步驗證、最後跑哪個閘門（2026-09-04 起） | `.claude/workflows/` | Claude Code |
| 攔阻 | 真的會拒絕的指令；守 harness 自己的機器 | `permissions.deny`、`automation-check` 的 `harness`、`skillpkg` 的 dogfood 測試 | deny 只有 Claude；機器所有人 |

**兩條判準**貫穿全部：知識放哪一層，問「**換一個 repo 還成立嗎**」；任何一條規則落地，問「**旁邊那台機器在哪**」。

**一個前提**：這個 repo 同時有多個 coding agent（Claude Code、Codex…）在同一棵工作樹上作業。Claude 專屬的那幾層**只是提早發現**；真正的保證永遠在 `automation-check`、測試與 CI。

## 現行入口（可直接引用）

- 規則：[`automation.md`〈共享工作樹與 SubAgent〉](../automation.md)（模型四級表）、[〈Harness〉](../automation.md)（放置規則、三步配方、deny 清單、送達實測）
- 入口：根 [`AGENTS.md`](../../../AGENTS.md)〈分區指標與攔阻〉；分區卡 [`apps/web/AGENTS.md`](../../../apps/web/AGENTS.md)、[`apps/platform/internal/AGENTS.md`](../../../apps/platform/internal/AGENTS.md)
- 機器：[`tools/devctl/harness.go`](../../../tools/devctl/harness.go)、[`repo_skills_test.go`](../../../apps/platform/internal/shared/skillpkg/repo_skills_test.go)
- Claude 層：[`.claude/agents/`](../../../.claude/agents/)、[`.claude/skills/`](../../../.claude/skills/)、[`.claude/rules/`](../../../.claude/rules/)、[`.claude/settings.json`](../../../.claude/settings.json)
- 具名 workflow：[`ux-text-audit.js`](../../../.claude/workflows/ux-text-audit.js)（讀 → 反駁 → 找漏讀）、[`parallel-page-edit.js`](../../../.claude/workflows/parallel-page-edit.js)（寫 → 驗 → 突變 → 閘門）、[`error-path-audit.js`](../../../.claude/workflows/error-path-audit.js)（一線一讀者 → 逐條反駁 → 找漏讀）。實際適用範圍與完成判定以[開發者指示](../agent-instructions.md)為準。

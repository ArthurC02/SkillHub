# Agent Harness：歷程、洞見、工序與功法

這個目錄記的是 **2026-09-03～09-04 建立 coding-agent harness 的過程本身**——量到什麼、蓋了什麼、撞壞什麼、為什麼最後長成這樣。**規則本體不在這裡**：規則在 [`automation.md`〈Harness〉](../automation.md) 與根 [`AGENTS.md`](../../../AGENTS.md)，本目錄只解釋它們為什麼是那個形狀，讓下一個要改 harness 的人（或 agent）不用把同一條路再走一次。

| 檔 | 回答的問題 |
| --- | --- |
| [01-journey.md](01-journey.md) | 發生了什麼、順序是什麼、哪些當時的判斷後來被推翻 |
| [02-insights.md](02-insights.md) | 從中得到的十二條洞見，每條附證據與後果 |
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

## 這段歷程的產出（可直接引用）

- 規則：[`automation.md`〈共享工作樹與 SubAgent〉](../automation.md)（模型四級表）、[〈Harness〉](../automation.md)（放置規則、三步配方、deny 清單、送達實測）
- 入口：根 [`AGENTS.md`](../../../AGENTS.md)〈分區指標與攔阻〉；分區卡 [`apps/web/AGENTS.md`](../../../apps/web/AGENTS.md)、[`apps/platform/internal/AGENTS.md`](../../../apps/platform/internal/AGENTS.md)
- 機器：[`tools/devctl/harness.go`](../../../tools/devctl/harness.go)、[`repo_skills_test.go`](../../../apps/platform/internal/shared/skillpkg/repo_skills_test.go)
- Claude 層：[`.claude/agents/`](../../../.claude/agents/)、[`.claude/skills/`](../../../.claude/skills/)、[`.claude/rules/`](../../../.claude/rules/)、[`.claude/settings.json`](../../../.claude/settings.json)
- 具名 workflow（09-04 稍晚，第一個案子是 `04` 丙-142 第二批）：[`ux-text-audit.js`](../../../.claude/workflows/ux-text-audit.js)（讀 → 反駁 → 找漏讀）、[`parallel-page-edit.js`](../../../.claude/workflows/parallel-page-edit.js)（寫 → 驗 → 突變 → 閘門）、[`error-path-audit.js`](../../../.claude/workflows/error-path-audit.js)（一線一讀者 → 逐條反駁 → 找漏讀；09-04 第二個案子，六條線）；量到的三件事在 [01-journey.md](01-journey.md) 第 7 節。**09-04 晚上跑了六次之後補進 `parallel-page-edit` 的三條規則，每條各對一次真的踩到的坑**：writer 改了檔就算落地，`stopped_because` 不再能跳過驗證與突變（lab 那支的空白名稱漏洞差點因此漏掉）；突變的「紅」由腳本判，失敗輸出沒有斷言行就記成沒紅（早上 `document is not defined` 被當成紅）；verifier 拿到其他 writer 與 coordinator 同時在改的檔案清單，不再把它們列成越界；另外 Go 閘門沒帶 `SKILLHUB_REQUIRE_DB=1` 直接拒跑（三支整合測試的假綠是 CI 才抓到的）

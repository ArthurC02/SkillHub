# infra/images — 動手前的指標

**這個檔只放指標，不複製規則內容、不抄版本與 digest**：與被指的那一份不一致時，一律以被指的那一份為準。

## 動手之前先讀哪一段

| 你要做的事 | 先讀 | 沒讀會被哪個閘門擋 |
| --- | --- | --- |
| 改 `runtime-agent-sdk/` 底下任何非 `.md` 的檔（Dockerfile、基底 digest、Node、SDK、`constraints.txt`、`package-lock.json`、`run.mjs`） | [README.md](README.md)〈已發佈的 digest 與孤兒清單〉：同一批要升 Dockerfile 的 `ARG IMAGE_VERSION`；[UPGRADES.md](runtime-agent-sdk/UPGRADES.md) 開頭：每次升級加一節實測紀錄 | `Runtime Image` workflow 的 I-05、pre-push 的 `devctl preflight --hook`（同一段判斷）、`automation-check` 的 `image-version` |
| 換基底映像 digest | README.md〈Digest 更新程序（I-02）〉 | 同上；掃描見〈門檻值〉 |
| 重產 `constraints.txt`／`package-lock.json` | README.md〈重新產生依賴鎖檔〉 | `devctl dep-audit`、I-05 |
| 推送一個升了 `IMAGE_VERSION` 的 commit | 先問負責人：推送會發佈新的映像標籤，標籤不可收回 | `skillhub-guards` 的推送閘門（負責人輸入 `/guard-allow` 放行一次） |
| 讓部署改跑新版本 | README.md〈已發佈的 digest 與孤兒清單〉：預設映像在 `apps/sandbox/cmd/sandboxd/main.go`，四項實測通過才移動 | UPGRADES.md 對應那一節要有四項實測輸出 |
| 改服務映像（`platform/`、`web/`、`llm/`、`sandboxd/`、`postgres/`、`devtools/`） | README.md〈服務映像〉 | CI 的 `images` job |

`run.test.mjs` 不會被複製進映像，改它不觸發 I-05。四項實測會呼叫付費模型，子代理不跑（根 `AGENTS.md`〈開發自動化〉第 2 條）。

**這個檔是給所有 coding agent 讀的**（同目錄的 `CLAUDE.md` 只有一行 `@AGENTS.md` 把它 import 進來）。

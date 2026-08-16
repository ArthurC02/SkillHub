# ADR-024：頂層目錄分「跑的」與「讀的」

- 狀態：Accepted
- 日期：2026-08-16
- 決策者：產品負責人、架構規劃
- 相關：[ADR-019](./ADR-019-monorepo-structure-and-cicd.md)（本 ADR 修訂其第 1 節「目錄結構」）、[ADR-016](./ADR-016-language-and-framework-selection.md)（三語言分工）

## 背景

[ADR-019](./ADR-019-monorepo-structure-and-cicd.md) 第 1 節把頂層依「部署產物」分組，`plans/`、`adr/`、`spikes/` 三個目錄則以「既有，保留」的形式列在最後。當時 repo 只有文件，這樣寫沒有代價。

M0～M2 落地後代價出現了：頂層有 11 個目錄，其中 8 個是軟體（`apps`、`services`、`contracts`、`db`、`infra`、`tools`、`packages`、`.github`），3 個是給人讀的文字（`plans`、`adr`、`spikes`）。兩類東西的生命週期、審查方式與 CI 關係完全不同——軟體目錄改了要跑測試、要進 Image；文字目錄改了只跑 `contracts-drift`（ADR-019 §3 已明列）。頂層看不出這條界線，新進者（人或 agent）必須讀完 AGENTS.md 才知道哪些目錄是「會跑的」。

`spikes/` 更是被錯放：它是 M0 的探索紀錄，不進 CI、不被產品程式碼 import（ADR-019 §3），實質是文件的附件，卻與 `services/` 平起平坐。

## 決策

### 1. 頂層兩類

| 類別 | 目錄 | 判準 |
| --- | --- | --- |
| **跑的** | `apps/`、`services/`、`contracts/`、`db/`、`infra/`、`tools/`（＋ CI 產生的 `packages/`） | 有建置產物、有測試、被 CI path filter 涵蓋 |
| **讀的** | `docs/` | 只有人與 agent 讀；改動不觸發任何建置 job |

### 2. `docs/` 下是純前綴搬移

```text
plans/  → docs/plans/
adr/    → docs/adr/
spikes/ → docs/spikes/
```

**三個目錄名一個字都不改。** 這是本 ADR 唯一需要裁定的地方，理由是：repo 內外已存在大量**非連結形式的文字引用**——AGENTS.md 的「見 `plans/mvp/02`、`03`」、ADR 的「`02:SEC-002`」、Go 註解裡的 `plans/mvp/m2/content-baseline-report.md §5.2`、commit message、以及對話與 issue 中的口語引用。這些引用沒有任何工具會修，也不該為了改名去掃。

保留原名讓「搬移 = 加一段前綴」：舊引用 `plans/mvp/02` 讀起來仍然半可辨識，讀者自己補上 `docs/` 就找得到。若同時改名（`plans/` → `docs/product/`、`adr/` → `docs/decisions/`），舊引用就變成必須查表才能還原的死字串，而收益只是名字好看一點。**不另造新名。**

### 3. 軟體目錄維持平鋪，不包一層 `src/` 或 `code/`

不做的三個理由：

1. **已是標準語意命名。** `apps/`、`services/`、`db/`、`infra/`、`contracts/` 在任何 monorepo 都自解釋，再包一層只是把 6 個清楚的名字換成 1 個模糊的名字加 6 個清楚的名字。
2. **成本與收益不成比例。** 這批目錄被寫死在 Go module 路徑、`go.mod` 的 replace、sqlc 輸出路徑、四份 workflow 的 path filter、`Taskfile.yml`、Dockerfile 的 build context，以及 **`infra/compose/docker-compose.yml` 的 bind mount 路徑——那是運行中 stack 的掛載點**。搬它們要停 stack、改遷移、重驗一輪；搬文件不用。
3. **收益遞減。** 「跑的／讀的」這條界線，把文件收進 `docs/` 之後就已經畫完了；再包一層不會讓它更清楚。

### 4. 里程碑產出的歸位（同一波併入）

`docs/plans/mvp/` 內部同時做一次歸位，把**跨里程碑仍被引用的內容**從凍結的 `mX/` 目錄移到主題目錄，遵循 AGENTS.md「一份文件如果會被下一個里程碑繼續改，它就不屬於 `mX/`」：

- `m1/{curated-skill-list,content-summaries,content-candidates}.md` → `content/`
- `m2/{anthropic-sa-license-memo,anthropic-sa-inquiry-draft}.md` → `governance/`
- `m1/gate-test/` → `gate-test/`（M1 閘門材料，D 日未宣告，仍是活的）

**只搬路徑，不動內容。** 原 `mX/README.md` 的檔案地圖標「已移至」，各目的地補一份說明身分與出處里程碑的 README。

## 影響

### 正面

- 頂層從 11 個目錄降到 7 個，「哪些會跑」一眼可判。
- `spikes/` 回到它實際的身分（文件附件），不再看起來像第四個服務。
- CI path filter 的排除項從 `!spikes/**` 收斂為 `docs/**`：**新增文件目錄不再需要同步改四份 workflow**，這正好補上 ADR-019「成本與限制」第一條（手寫 path filter 的靜默漏洞）在文件面的那一半。

### 成本與限制

- 一次性的連結修復：`docs/` 內部互指因為三者同波搬移、相對深度不變而**全部維持有效**；需要改的只有跨界的兩個方向（docs → repo 根與軟體目錄，多一層 `../`；軟體目錄與 CI → docs，多一段 `docs/`）。
- `.github/workflows/egress-allowlist.yml` 內嵌威脅模型路徑的斷言必須同步改，且**改完要做正反向驗證**——改錯了它不會報錯，只會安靜地不再擋任何東西。
- `git log <path>` 需要 `--follow` 才能跨越搬移點；搬移以 `git mv` 進行，blame 與歷史本身不受影響。

## 待決策

無。ADR-019 第 1 節的其餘內容（`services/sandbox/` 獨立 Go module、`packages/` 產物入 repo、M1 只建 M1 目錄）不受本 ADR 影響，繼續有效。

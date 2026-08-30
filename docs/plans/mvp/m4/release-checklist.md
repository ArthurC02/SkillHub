# 封測上線前檢查表（`RELEASE-001`～`010` 的執行面）

- 日期：**2026-08-18**
- 這是什麼：`03` §18 的十項 `RELEASE-*` 是 `02` §7 Definition of Done 的**逐條檢查表**，但它們只寫了「要檢查什麼」，沒有寫「**誰做、做什麼、驗什麼、什麼順序**」。這份補那一半。
- **勾選狀態的唯一事實來源不是本檔**，是 [`../../03-work-items.md`](../../03-work-items.md) §18 與 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md)。本檔是**順序與說明**，M4 完結即凍結；部署期的實際結果回填那兩份。
- 目標事件：**B 日**（封測開始日，[pdm-009-beta-proposal.md §5.2](pdm-009-beta-proposal.md)）。B 日的前提是本檔 §2～§4 全部成立。
- 相關：[README.md §4](README.md)（甲類四項的到期裁定）、[README.md §5](README.md)（程式／人的分界）、[beta-design.md §8](beta-design.md)（封測開始前的 11 項檢查，本檔是它的擴充）、[audit.md](audit.md)（勾選與不勾的逐項理由）

---

## 0. 一頁摘要

| 段 | 內容 | 誰 | 幾項 | 現況 |
| --- | --- | --- | ---: | --- |
| **§1 程式面已完成** | 已勾選、可作為後續各段的前提 | — | **11** | ✅ 全部成立 |
| **§1.9 程式面尚缺** | 有承接者就做得完，**不需要任何新決策** | agent 或開發者 | **12** | ⬜ 全部開著 |
| **§2 部署期** | 真機、migration、種入、探針、告警 | 負責人 ＋ 真機 | **7 段** | ⬜ 一段都沒開始 |
| **§3 負責人動作** | 拍板、追認、宣告、寄信 | 負責人 | **11** | ⬜ 全部開著 |
| **§4 B 日當天** | 名單生效與最後確認 | 負責人 | **5** | ⬜ — |

**三段之間的順序是硬的**：§1.9 與 §2 可並行 → §3 的前四項（乙-13、乙-14、PDM 追認、D 日）必須在 §2 完成之前就開始 → §4 是最後一道。

**✅ 2026-08-29：這份文件第一次有日期了。**

| 日期 | 是什麼 | 對這份文件的意思 |
| --- | --- | --- |
| **2026-09-11** | **M1 閘門 D 日**（負責人宣告，[`05` R-5](../../05-pending-rulings.md)）；**同時是本輪夯實工作的最後期限** | §3 的 H-8 有值了。**先閘門、再封測**那條 ⛔ 邊界不變，所以封測的 B 日在 D 日 ＋ 10 天之後 |
| **2026 年 9 月中下旬** | **Demo**（M6 的 Pitch） | 它不在這份文件的範圍內（這裡是封測），但它與 §2 搶同一台真機與同一個人 |
| **M6 真正完成之後** | **凍結新功能** | 「真正完成」＝ `03` §20 的十一項全部收束，不是「程式面收斂」。凍結後只接受修既有缺陷的變更 |

**這三個日期改變的是排序而不是內容**：§2「一段都沒開始」這件事，從今天起有一個會到期的東西在旁邊。[`05` §0.1](../../05-pending-rulings.md) 那句話仍然是這條路線上投報率最高的一件事——**「把『一台節點、跑通一個 Run』獨立成最小驗證，不要等其餘做完才發現節點跑不起來」，而它不等任何人的簽名。**

---

## 1. 程式面已完成（可作為前提的十一項）

**這一節不是待辦，是「後面各段可以站在什麼上面」。** 逐項證據見 [audit.md §1.1](audit.md)。

- [x] `CORE-008` 重要操作的 Audit Event（`02` NFR-001 第 4 條五項全齊；下載與使用者資料刪除是最後補上的兩個事件源）
- [x] `PACK-001` 標準打包流程（zip root ＝ Skill root、雙雜湊、四欄冪等鍵、不動 Skill Version）
- [x] `PACK-002` 打包前重跑規格驗證（對交出去的位元組重跑匯入路徑，重用 `EVAL-010` 的同一條）
- [x] `PACK-004` 排除 Secrets、測試憑證、內部路徑與受保護資料（白名單；對產出 zip 逐 entry 斷言）
- [x] `PACK-005` 可散布 Test Case 與範例資料（兩層判準、四種拒絕理由分得開、使用者上傳的沒有勾選框）
- [x] `PACK-006` 三份安裝 Profile（資料驅動；目錄缺席即 503）
- [x] `PACK-008` 未驗證 Agent 顯示未驗證（支援狀態在任何指示之前；相容性三層不外推）
- [x] `QA-002` 規格驗證測試資料集（21 個破壞變體，24／24）
- [x] `QA-003` 正常／失敗／逾時／取消／清理 Run 測試
- [x] `QA-004` Dataset 與 Secrets 權限測試
- [x] `QA-005` Trace 完整性與遮罩測試

**另有五組「已落地但工作項未勾」的機制，B 日要靠它們**（未勾的原因見 `03` 行內）：

| 機制 | 落點 | 為什麼 B 日靠它 |
| --- | --- | --- |
| 封測准入閘門 | `identity/http.go` 的 `RequireInvited`（Fork／建 Run／下載三條路由） | 名單以外的人不會吃掉配額 |
| 免費額度強制 | `internal/run/quota.go`＋建立 Run 的同一交易 | 成本上限的唯一執行機制 |
| 四個漏斗事件 | `internal/analytics` ＋ `0029` | `BETA-002` 的全部原料 |
| `POST /feedback` | `analytics/feedback.go` ＋ `feedback_reports` | `BETA-003`／`004`／`005` 的管道（**缺前端入口**，見 §1.9） |
| 停派送開關 | `internal/trial/execution/halt.go` ＋ `0030` ＋ `/admin/dispatch*` | P1 與 X-04 共用的同一個煞車 |

### 1.9 程式面尚缺（十二項，不需要任何新決策）

**這一節每一項都對應 `04` 的一條丙類殘項，補完即可重新勾選對應工作項。**

> **2026-08-18 補記（本檔凍結，只加這一段，表格原文不動）**：屬 Go／後端／工具的那些已補——**P-1／P-2／P-3／P-4／P-7／P-8／P-11 完成，P-5／P-9／P-10 的後端半邊完成**（commit `50624e2`／`8a17652`／`04bb5e6`）。`03` 的 `PACK-003`／`QA-007`／`QA-001` 三項改判勾選。**P-6／P-12 與 P-5／P-9／P-10 的前端半邊仍在 `apps/web`。**
>
> **P-7 的表格文字要當作已被推翻讀**：「刪掉平台加的三樣檔案後重新匯入，得到同一個 `content_hash`」**寫不出來**——`content_hash` 是使用者當初上傳的那份壓縮檔位元組的 SHA-256，重新壓縮同一組檔案必然不同。落地的是同一句設計的另一半（「除平台新增檔案外逐位元組相同」），並把 `standard.json` 裡叫使用者做那個檢查的那一步一併改寫。逐項見 [audit.md §7](audit.md)。

> **2026-08-18 第二次補記（本檔凍結，只加這一段，表格與其後的段落原文不動）**：**§1.9 的十二項全部完成。** P-6 與 P-12、以及 P-5／P-9／P-10 的前端半邊由 UI 批與其後的收尾批補上；`03` 的 `DESIGN-012`／`WS-004`／`PACK-007`／`CORE-007`／`O11Y-004` 五項改判勾選（前兩項見 [audit.md §8](audit.md)，後三項見 [§9](audit.md)）。
>
> **本節末尾那句「另有兩項要先有 §3 的決策才能動」要當作已被推翻讀**（同 P-7 的處置）。**兩項都不需要新決策，而且兩項的可做部分都已經做完**：①`SEC-012` 的自動觸發——「接哪幾條判準」不是偏好而是查得出來的事實，平台自己看得見的兩條（`TraceMaskingStopped`、Reconciler 停擺 > 10 分鐘）已接上，其餘三條的訊號在本 process 之外，屬 §2 部署期；**`SEC-012` 仍不勾**。②無障礙的對比守門——`QA-008` 管的是**算繪後的合成像素**，而 token 層的靜態十六進位值不需要版面計算，已落地為 `apps/web/src/contrast.test.ts`；合成像素（alpha、`opacity`、真瀏覽器算繪）仍無守門，**`QA-009`／`DESIGN-013` 仍不勾**。**兩次是同一個形狀：一段不需要決策的工作，被掛在一個真的要人拍板的東西旁邊一起等。** 逐項見 [audit.md §9](audit.md) 與 [`04` 丙-21／丙-26](../../04-backlog-and-handoffs.md)。

| # | 做什麼 | 驗什麼 | 解鎖 |
| --- | --- | --- | --- |
| P-1 | 一支測試斷言 `LICENSE`／`LICENSE.repo`／`LICENSE.repo.provenance.json` 出現在三個目標的產出 zip 裡且內容與來源相同 | 對**匯出的位元組**斷言，不對 manifest 欄位斷言 | `PACK-003`、`QA-007`（丙-17） |
| P-2 | 作者資訊二選一：manifest 加 additive 的 author 欄位，或 `02:PACK-001` 就地寫明「隨套件位元組保留、不另立欄位」＋測試 | 兩條路都要有一個可判定的落點 | `PACK-003`（丙-17） |
| P-3 | 拿掉或兌現 `standard.json` 那句「INSTALL.md lists them, including the ones Skill Hub's own runtime image refuses」 | 產生器真的查 `infra/images/README.md` 的拒收表，或那句話不再出貨 | `PACK-007`（丙-18） |
| P-4 | `dependencyNotes()` 納入 `undeclared-dependency` | INSTALL.md 出現「套件沒宣告但程式碼 import 了」那一類 | `PACK-007`（丙-18） |
| P-5 | `targetView` 補 `env_vars` 與依賴摘要；前端型別跟上 | `02:PACK-002` 第 1 條的五項在**下載頁**都看得到 | `PACK-007`、`DESIGN-012`（丙-18） |
| P-6 | `Packaging.tsx` 引用既有的 `CompatibilityStatus` | 三層相容性在打包／下載頁分開呈現 | `DESIGN-012`（丙-18） |
| P-7 | `TestTheStandardPackageRoundTripsToTheSameContentHash` | 刪掉平台加的三樣檔案後重新匯入，得到同一個 `content_hash` | `PACK-009`（丙-19） |
| P-8 | 一支以 API 為主的旅程測試（Fork → Test Case → preflight → Run → 評估 → 打包 → 下載，Provider 用既有 fake） | **接縫**，不是各段——四次退回都發生在兩段之間 | `QA-001`、`RELEASE-002`（丙-20） |
| P-9 | Run Artifact 的單項刪除端點；`GET /me` 回 `deletion_requested_at` | `02:WS-002` 第 3 條的「Artifact」與 `02:SEC-006` 的「可追蹤狀態」 | `CORE-007`、`RELEASE-005`（丙-22） |
| P-10 | `GET /runs` 列表端點；Run 列表與個人 Skill 列表兩個畫面；下載紀錄逐筆列出 | `02:WS-002` 第 1 條的五項都有使用者碰得到的平面 | `WS-004`（丙-24） |
| P-11 | 漏斗的跨表查詢（七段，四段查 `analytics_events`、五段查領域表）＋報表要標明精度限制 | 同一個 session 能把漏斗事件與阻斷回報串起來（`BETA-004` 的判定尺） | `O11Y-004`、`BETA-002`／`004`（丙-25） |
| P-12 | `POST /feedback` 的前端入口（全站可及，帶 `page_path` 與可選 `run_id`） | 卡住的人按得到；不自動抓畫面 | `BETA-003`／`004`（丙-27） |

**另有兩項要先有 §3 的決策才能動**：`SEC-012` 的自動觸發（丙-26，要先決定接哪幾條判準）、無障礙的對比守門（丙-21，綁在 `QA-008` 的路線決策上）。

---

## 2. 部署期（真機）

> **2026-08-23 更正：本節標題原為「`SEC-009` 未過即不得開放外部使用者提交 Skill 執行」，而 [ADR-050](../../../adr/ADR-050-beta-runs-in-parallel-with-the-sandbox-acceptance.md) 已由負責人裁定**封測與甲類驗收並行**（`04` 乙-14）。**
>
> **甲類四項因此不再是封測 D 日的阻擋項**，下面 §2.1 的標題與 §5 的 H-2 一併更正。**但它們的內容一個字都沒改**：`02:SEC-009` 的門檻仍是 45 項全數 pass、0 項 unknown，`infra/nodes/gvisor-baseline.txt` 仍必須填實際版本才算數。改的是**這個門檻在什麼時點之前必須達成**，不是門檻本身。
>
> **ADR-050 明示接受的風險**：甲類通過前，不受信任程式碼會在**逃逸邊界尚未被驗證**的節點上執行。該 ADR 同時寫下一件本節該知道的事——**Suite 1（T1／T2／T3／T4／T8 映像半）只要 Linux ＋ Docker ＋ runsc**，在那台節點跑一次是一天的事；「全部到期」與「什麼都不做」之間不是只有兩個選項。**要不要把它訂為第一位外部使用者之前的下限，是 ADR-050 的待決策 1，目前沒有答案。**

**依據不是新的**：ADR-015 定案紀錄。M0～M3 能把甲類當平行工作是因為**沒有任何外部使用者**；封測第一次讓外部人員在真實部署上建立 Run。裁定見 [README.md §4](README.md)，拍板見 [`04` 乙-14](../../04-backlog-and-handoffs.md)。

### 2.1 甲類四項（~~封測阻擋項~~ **與封測並行，ADR-050**）

| # | 誰 | 做什麼 | 驗什麼 |
| --- | --- | --- | --- |
| 甲-1 | 負責人＋真機 | `SEC-009` 十個測項全跑 | **45 項基線全數 pass、0 項 unknown**。任一 fail 或 unknown 即不得開放，**無例外流程**。證據落 `m4/sec-009-acceptance/<日期>-<節點>/`（判定表 ＋ `versions.txt` 進 repo，原始輸出留 CI artifact 並附連結），保存 ≥ 1 年 |
| 甲-2 | 同上 | `SBX-010` 的工作項側 | 同甲-1。**現有的真實容器驗證（非 root、唯讀 rootfs、無主機掛載、pids 上限、逾時強停、清理冪等）不等於逃逸測試通過** |
| 甲-3 | 同上 | `SBX-005`／`007` 的生產網路面 | 每 Run netns ＋ `--icc=false`（關掉 dev 現存的**不需逃逸**的跨 Run 橫向路徑）；nftables default-deny ＋固定 DNS；`infra/egress/allowlist.yaml` 的 `pinned_ip` 已填實際值且**不是控制平面節點**（ADR-022 Q2 強制條件 6，由測項 T5-7 抓）。**LiteLLM 必須移到沙箱面專屬節點**——現行 compose 的 `127.0.0.1:4000` 是 dev 形態，生產不可複製 |
| 甲-4 | 同上 | `SBX-002` 的閘門 A 節點准入探針 | 探針在**真實節點上**查得到已發佈映像的 SBOM 與掃描 attestation；到期前 7 天告警的**發送端**已接 |

**兩個要在第一台節點上先確認的未知數**（[README.md §10 R1](README.md)）：

1. **gVisor 的 `systrap` 平台是否真的不需巢狀虛擬化**（ADR-022 只說「待部署批第一台節點實測確認」）。**把「一台節點、跑通一個 Run」獨立成最小驗證，不要等其餘做完才發現節點跑不起來。**
2. `infra/nodes/gvisor-baseline.txt` 必須填實際版本（**非 `unset`**），否則 `SEC-009` 的前置條件②不成立、整批判 unknown ＝ fail。

- **`unset` 在兩個檔案裡都會安靜地通過**（2026-08-25 補記）：`infra/egress/allowlist.yaml` 的 `pinned_ip: unset` 與 `infra/nodes/gvisor-baseline.txt` 的 `unset` 都代表「還沒有節點」，兩者都 fail-closed（前者 render 不出任何 accept 規則，後者沒有可比對的基準），所以**節點會正常起來、Run 會安靜地到不了閘道**。`.github/workflows/egress-allowlist.yml` 只檢查 YAML 的不變式，**沒有任何檢查會說「這台活著的節點是用 `unset` 建的」**。因此：**節點建置後、跑第一個 Run 之前，在真機上確認 `nft list ruleset` 有指向閘道 IP:port 的 accept 規則、且 `runsc --version` 對得上 `gvisor-baseline.txt` 的實際版本；任一處仍是 `unset` 就停止建置**——不要用「Run 失敗」去發現它，那是部署日最難診斷的一種失敗。

### 2.2 Migration 套用（順序是硬的）

**部署 schema 必須到目前 HEAD `0034`。** 下列十一份互相依賴；不要再以手抄的「缺幾份」判斷目前版本：

> **0034 維護窗要求**：它會在 partitioned `trace_events` 上以 volatile sequence default 回填 `ingest_seq`，可能持有強鎖。正式資料庫不得直接照開發環境時間推估：先在 production-sized clone 量測鎖定與回填時間，設定 `lock_timeout`，安排停止 Trace ingestion 的維護窗並確認 rollback 空間；未留下這份演練證據即視為 deployment blocker。MVP 尚未上線，現階段不為尚不存在的線上零停機需求引入雙寫 migration。

```bash
# 順序不可調換；每一份都 --single-transaction，中途失敗不留半套
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0024_evaluation.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0025_suggestion_proposed_content.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0026_test_case_rubric.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0027_packaging.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0028_download_retention_and_object_reconcile.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0029_beta_analytics.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0030_dispatch_halts.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0031_trace_event_id_dedupe.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0032_run_state_and_trace_stream_guards.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0033_evaluation_model_usage.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0034_trace_incremental_reads.sql
# 然後才是資料回填（不是 migration，可重跑）
psql -v ON_ERROR_STOP=1 --single-transaction -f tools/content/backfill-redistribution.sql

# schema HEAD smoke check：四項任一為空都不可啟動 HEAD binary
psql -Atqc "SELECT to_regclass('public.evaluation_model_usage')"
psql -Atqc "SELECT to_regprocedure('public.enforce_run_status_transition()')"
psql -Atqc "SELECT to_regprocedure('public.enforce_trace_stream_seq()')"
psql -Atqc "SELECT to_regclass('public.trace_events_run_ingest_seq_idx')"
```

**驗什麼**：回填應回報 **`UPDATE 90`**，分佈為 **41 `allowed`／4 `blocked`／0 `unknown`**（4 筆 blocked ＝ `anthropics/skills` 的 `docx`／`pdf`／`pptx`／`xlsx`）。**數字對不上就不要往下做**——打包的授權閘門是照這個欄位擋的。該分佈已在一個乾淨的拋棄式部署上獨立複現過（[README.md §14.2](README.md)）。

**套用之後必須做的三件事**：

- [ ] **重建 `cmd/api`／`cmd/worker`／`sandboxd` 到 HEAD** 並帶齊環境變數（見 §2.3）。舊二進位在新 schema 上會以難看的方式失敗（丙-8 記錄的正是反過來的那一半）。
- [ ] **`0024`～`0034` 套用並通過上述 schema HEAD smoke check 後，`EVAL-013` 的 B 輪回歸解除**（丙-8）：依 [`../m3/report-judge-regression.md` §13.4](../m3/report-judge-regression.md) 的五個 `test_case_id` 走 `PATCH /test-cases/{id}` 改 Prompt → 走既有 Run 路徑重發 5 筆 → `judge_regression.py --rubric` 重評。**第 ⑤ 步之前先確認新 Run 進得了 `skill_runtime_compatibility`**，否則 harness 取到的仍是 M2 的舊 Run，而且不會有任何提示。成本約 $0.3～0.5。
- [ ] **`0027` 套用前，dev 上的打包路徑只會回 `license_unknown`**（`redistribution` 欄位不存在 ⇒ fail-closed）。這是正確行為，不是故障——回填前的實測已記在 §14.2 末段。

### 2.3 部署設定（未設會安靜地關掉功能）

| 變數 | 不設的後果 | 值從哪來 |
| --- | --- | --- |
| `PACKAGING_PROFILES_DIR` | 零個打包目標，**每條打包路由 503** | 預設 `contracts/packaging/profiles` |
| `DOWNLOAD_ARTIFACT_RETENTION` | **未設或非法即 fail-closed：打包建立回 503，不得產生下載 Artifact** | **PDM-006 追認後才設定正值**（§3） |
| `ANALYTICS_RETENTION` | **不設 cookie、不寫任何一列** ⇒ `BETA-002` 的漏斗量不到任何東西；**且 `maintenance rotate-partitions` 整個 job 拒絕執行**（§2.6） | **PDM-006 追認**（ADR-029 提案 180 天） |
| `TRACE_RETENTION` | **`maintenance rotate-partitions` 整個 job 拒絕執行**（兩個保存期在任何語句之前一起讀）⇒ **`trace_events` 與 `analytics_events` 的月分割既不會被預先建立，也不會被丟棄**。Trace 寫入本身不受影響，事件照收，只是全部落進 `trace_events_default`（§2.6） | **PDM-006 追認**（`0004` 註解寫的 90 天是提案，至今沒有任何東西在執行它） |
| `BETA_ALLOWLIST` | 閘門關閉，**任何有 GitHub 帳號的人都能用** | PDM-009 追認後的 12 個 GitHub 帳號 |
| `RUN_QUOTA` | 額度不強制，`GET /me/quota` **不掛載**，preflight 不帶配額區塊 | PDM-010 擇一後開啟 |
| `RATE_LIMIT`（2026-08-24 新增） | **只有 `off` 會關掉它**；未設或填任何其他值（含填一個數字）都是啟用預設 60/min、burst 30。方向與上面幾列**相反**：未設＝有保護 | 保持未設 |
| `FEEDBACK_RETENTION`（2026-08-29 新增） | **fail-closed，比照 `AUDIT_RETENTION`**：未設或非法時 `maintenance purge-feedback` 拒絕啟動 ⇒ `feedback_reports` 的自由文字**沒有保存期、沒有清除**。這是本 repo 唯一一個「在收、卻沒有期限也沒有 sweep」的資料類別，而它收的是受測者用自己的話寫的東西 | **PDM-006 追認**（與其餘保存期同一批） |
| `OPERATOR_USER_IDS` | 沒有人能操作 `/admin/dispatch*`（P1 停派送只能改 DB） | 負責人自己 |
| `LITELLM_API_KEY` | **這裡放的必須是一把由 master key 簽發的 Virtual Key，帶 `max_budget` 與模型白名單；放 master key 是部署缺陷。** master key 不只是「一把預算很大的 key」，它是閘道**管理 API 的管理員憑證**——可以簽發 Virtual Key、讀取全部 key 的 spend、（`STORE_MODEL_IN_DB` 開啟時）改動模型路由。把它交給 `apps/llm`，等於讓一個處理**不受信任套件內容與使用者 prompt** 的行程持有整個模型出口的管理權，而 ADR-017 決策段逐字寫著「Python 服務與 Sandbox 只持有 Virtual Key」。**repo 裡每一份記錄過實跑的文件都是直接把 master key 填進來的**（m3 的兩份報告、`generate_integration_test.go` 的重現指令、`04` 丙-56），所以這一列是**部署期一定要撞到的一件事**，不是提醒 | 部署時由 master key 簽發；`LITELLM_MASTER_KEY` 只留在閘道那一側 |
| `SKILLHUB_MODEL_GATEWAY_*` | 沙箱被派到 `--network none`，Run 全部失敗 | 既有 |
| 物件儲存設定 | `ErrNoStore` ⇒ 打包 503 | 既有 |

**⚠️ 反向代理下速率限制會退化成「全體共用一個桶」**（2026-08-24，`04` 丙-54）：限制器以 `RemoteAddr` 分桶，**刻意不讀 `X-Forwarded-For`**（客戶端能設的標頭就是客戶端能選的桶）。所以只要前面擺了 TLS 終止層或任何代理，**十二位受測者共用 60/min、burst 30，而且是一起被 429**。部署時二選一：①在代理那一層做限制、②只在代理與 API 之間是可信網段時，才在代理上設定把真實來源 IP 傳進來並改讀它（**要先改程式，今天不讀**）。IPv6 已按 /64 分桶（單一配置有 2^64 個位址，按位址分桶等於沒有限制）。

**兩個 roster 的 fail-closed 方向相反，部署時要各驗一次**：`operator.roster` 寫不成 ⇒ **不承認任何 operator**；`beta.roster` 寫不成 ⇒ **誰都進不來**。兩者都是啟動時寫一筆 audit event。

### 2.4 策展內容種入

- [ ] 先確認 dev／生產上**哪個帳號持有目錄 Workspace**（`workspaces.is_catalog` 沒有端點，由建目錄時的 SQL 設定）
- [ ] `python tools/content/import_seed.py`（45 筆目錄）→ 驗 **45/45 imported**
- [ ] `python tools/content/seed_testcases.py --api <url> --user <目錄策展帳號> --dry-run` → 驗 **15 筆都解析得到 Skill**
- [ ] 拿掉 `--dry-run` 重跑 → 驗 **15 建立、67 條驗收條件、5 筆帶 rubric、22 個 rubric item 逐筆指得到真的驗收條件、每筆 2 個 Dataset**
- [ ] 再跑一次驗冪等 → **15 筆 `exists_skipped`，零寫入**

**為什麼這一步不能跳過**：`selectTestCases` 以 `workspaces.is_catalog` 判定「這是不是平台策展的產物」，**沒有種入就沒有任何 Test Case 能入包**，`PACK-005` 在該部署上永遠只會產生 `not_curated` 排除。

### 2.5 可觀測性與告警

- [ ] **Alertmanager 部署 ＋ 通知路由**——單人團隊裡，最高級告警必須送得到那一個人
- [ ] Grafana dashboard；`O11Y-003` 的門檻值上線後回填（首發值是預設非實測校準值）
- [ ] 驗 `TraceMaskingStopped` 這條規則真的會觸發並送達（**`NFR-002` 沒有其他偵測器**）
- [ ] 驗閘門 A 到期前 7 天告警的發送端（甲-4 的一部分）

### 2.6 對帳器與排程

> **2026-08-29：這一節少了五行，而少掉的那五行是同意書上寫給受測者看的承諾。** `cmd/maintenance` 有七個保存期子命令，此前只有兩個（`purge-accounts`、`rotate-partitions`）在這份文件裡接上排程；其餘五個在整份檢查表裡**零命中**。Trace 與分析事件靠分割表輪替、帳號刪除有自己的 cron，**剩下的位元組在部署上沒有任何東西會去刪它們**——而畫面已經會把過期的那一列標成「檔案已刪除，這筆紀錄保留」。**畫面說的話會比事實更乾淨，那正是本專案反覆記載的那種缺陷。**<br>**`devctl automation-check` 的 `purge-schedule` 自 2026-08-29 起機械對帳**：`main.go` 的 dispatch switch 裡每多一個 `purge-*`，這一節就要多一行含它名字與 `cron` 的句子，否則 CI FAIL。**一個沒有排程的 purge 子命令，就是一句沒有人執行的保存政策。**

- [ ] `cmd/maintenance purge-accounts` 接上 cron（**程式刻意不自帶 scheduler**）；`PURGE_GRACE` 預設 720h
- [ ] `cmd/maintenance purge-run-artifacts` 接上**每日** cron。承諾對象：同意書 §3「試跑產出的檔案」（**2026-08-29 起為 90 天**，[`05` R-11](../../05-pending-rulings.md)）。**驗一次真的刪掉了位元組，不只是標了欄位**——`expires_at` 到期只會讓畫面標示過期，位元組要這個 job 才會走
- [ ] `cmd/maintenance purge-datasets` 接上**每日** cron。承諾對象：同意書 §3「上傳的 Dataset 90 天」。逐列 `expires_at`，**刻意沒有單一環境變數**（理由在 `cmd/maintenance/main.go`）。**同樣要驗位元組**
- [ ] `cmd/maintenance purge-audit` 接上**每週** cron；`AUDIT_RETENTION` fail-closed（未設即拒絕啟動）。承諾對象：同意書 §3「稽核 400 天」。**這一類是 2026-08-25 才補上 job 的**——在那之前同意書從第一版就宣告了 400 天而那個 job 不存在
- [ ] `cmd/maintenance purge-feedback` 接上**每週** cron；`FEEDBACK_RETENTION` fail-closed（比照 `AUDIT_RETENTION`，見 §2.3）。承諾對象：`feedback_reports` 的自由文字——**受測者用自己的話描述他們卡在哪**，是三類裡最敏感的一種。**同批要驗 `GET /policy/data-retention` 真的把 feedback 列出來**：那個端點今天只列四個 analytics 事件，並逐字宣告「表裡沒有任何自由文字欄位」，那句話對 `analytics_events` 為真、對這個部署為假
- [ ] `cmd/maintenance purge-deleted-skills` 接上**每日** cron。**⚠️ 紅字：模板留空 ⇒ 承諾未執行。** `.env.example` 的 `SKILL_DELETION_GRACE=` 是**刻意的空值**（fail-closed，理由硬：那個 job 刪的是使用者自己的內容），後果是**照著模板部署的環境從來沒有執行過它**，而刪除畫面上逐字寫著「30 天寬限期後清除」。**這一列不是待辦，是一個正在對使用者說謊的狀態**：值由負責人簽 PDM-006 §6.1 的 30 天（§3），簽之前這句承諾在畫面上要能被關掉或改寫
- [x] ~~**⏱ `cmd/maintenance rotate-partitions` 要在 2026-09-01 之前手動先跑一次**~~ **✅ 2026-08-30：不再需要人手，因為建立那一半已經不在這個子命令裡了。** 2026-08-29 的夯實批把建立與刪除拆開（`partition.CreateUpcoming`），建立的那一半成為 worker 的週期性工作 `PartitionCreateArgs`，**且以 `RunOnStart: true` 註冊**——`entrypoint/worker/worker.go` 的註解逐字寫著理由：「月底才上線的部署必須在月份翻過去之前就有下個月的分區，不是一個間隔之後」。刪除那一半仍留在本子命令，在它 fail-closed 的保存期變數後面（見下一行的 cron）。<br>**實測 2026-08-30**：淨測試模式一次冷啟動即印出 `partitions created table=analytics_events partitions="[analytics_events_2026_09 analytics_events_2026_10]"` 與 `trace_events` 的同一行——**在 9 月到來之前就有了 9 月與 10 月**。<br>**因此本項的殘餘義務只剩一句**：任何部署都必須真的有 worker 在跑（`cmd/worker`，或淨測試模式那種 in-process 的形態）。**只起 API 而不起 worker 的部署仍然會掉進 default 分割**，而那時的補救成本仍是一次抽乾。原文保留在刪除線裡，因為它記的是一個當時為真的期限。
- [ ] `cmd/maintenance rotate-partitions` **接上每月 cron**（DDD-032 新增；需要 `DATABASE_URL`）：一次執行同時處理 `trace_events` 與 `analytics_events`——建立**當月與其後兩個月**的分割，並丟棄超過保存期的月份。**`TRACE_RETENTION` 與 `ANALYTICS_RETENTION` 皆無預設，兩者在任何語句之前一起讀，任一未設即整個 job 拒絕執行**（H-5 的 PDM-006 追認之前設不了值 ⇒ **分割不會被丟棄，也不會被預先建立**）。**排程至少每月一次**：預建兩個月是給「連續漏跑一次」的餘裕，連漏兩次就會開始寫進 default，而 default 是分割丟棄永遠碰不到的地方
- [ ] 驗**第一次執行時 default 分割的抽乾**：2026-09-01 至該 job 首次執行之間寫入的 trace 事件都在 `trace_events_default`，`CREATE TABLE ... PARTITION OF` 會因此被 Postgres 的 `23514` 拒絕。**錯誤訊息本身就是操作步驟**（detach／建月份／搬列／re-attach／重跑），程式刻意不自動抽乾——這是一次性動作，不是每月動作（`0019` 早已預告）。**`analytics_events` 現在不需要抽乾但可能需要**：`ANALYTICS_RETENTION` 未設之前一列都不寫，所以 `analytics_events_default` 是空的；**若先設了它才接 cron**，事件就會落進 default 而重演同一件事——設定與接 cron 請同一次做完
- [ ] 驗一次**重跑**：同一個月再跑一次應該印出 `created=[] dropped=[]`（`slog` 的 `partitions rotated`）。job 是冪等的，cron 觸發兩次不是故障；哪天它不再是空的，就是有事發生了
- [ ] 物件存在性對帳器一小時一輪且**刻意沒有 `RunOnStart`** ⇒ **部署後第一小時是空窗**，知道就好
- [ ] 驗對帳器要兩輪才標記（`object_reconcile_sightings`），且無法連線的儲存產生**零次觀測**而不是一次假的
- [ ] **一個新端點要有一次真實閘道呼叫的紀錄。** 任何新增或改動模型呼叫路徑的端點，上線前要有一次對真實 LiteLLM 閘道的呼叫並把結果落檔（時間、端點、HTTP 狀態、實付金額、落點文件）。**理由是 `04` 丙-56**：M5 的生成路徑寫完之後**對真實閘道一次都沒有成功過**，schema 在 `strict` 下不合法、**每一次請求都會 400**，而那件事是在寫完之後很久才被一次付費呼叫查出來的。**單元測試與整合測試都不會發現它**——它們打的是替身。成本是一次呼叫（M5 那次是 $0.02 級）

### 2.7 上線後的第一次真實驗證

- [ ] 一個人走完 `02` §7 DoD 第一條的旅程（意圖搜尋 → 詳情 → Fork → 試跑 → 評估 → 打包下載），落檔
- [ ] 驗 `anthropics/skills` 那四筆**真的打不出包**（兩道鎖各驗一次：撤下 hold 之後 `redistribution` 仍應擋住）——**這是寄詢問信前的實測要求**（§3）
- [ ] 驗至少一個 `documents` 類精選 Skill **可下載**（[beta-design.md §8](beta-design.md) 第 11 項；那四筆受限讓該類最好的樣本不可下載，PDM-002 早已要求補 2–3 個 OSI 授權的替代品，**那是 `documents` 類的必要條件不是加分項**）

### 2.8 仍待定值或部署驗證的技術債

- [ ] **`DEPLOY-IAC-001`**：部署負責人建立 ADR-022 的 sandbox node IaC／cloud-init/render；pinned IP 未填時不得產生放行規則，並以 SEC-009 真機證據驗收。
- [ ] **`RUNTIME-PYTHON-001`**：負責人先定值 Python runtime 版本；部署負責人令 runtime image、文件與真實 gVisor 證據一致。不得把目前 image 的版本視為追認。
- [ ] **`LLM-RES-001`（partial）**：既有 query 長度三層上限保留；部署負責人補 anonymous search 的分散式 rate limit／成本保護，並證明拒絕請求不會呼叫 embedding 或 match-reason LLM。
- [ ] **`SUPPLY-RUNTIME-LOCK-001`**：runtime image owner 將 Python／Node transitive dependency 改為 repo-owned lock 或 constraints；以乾淨 cache 的兩次 build 證明 dependency tree 一致。

`LLM-EVAL-007` 是產品／帳務決策，不在部署清單中；其 owner、決策與 contract evidence 見 [`04` N-8](../../04-backlog-and-handoffs.md)。

---

## 3. 負責人動作（agent 做不到）

| # | 誰 | 做什麼 | 驗什麼（什麼算完成） | 擋住什麼 |
| --- | --- | --- | --- | --- |
| H-1 | 負責人 | **乙-13 拍板**：G7／G8 二選一——(a) `artifact` 型引用不得滿足 `evidence_required`；(b) 保留但標示「此引文未經回驗」。**G7 與 G8 一起裁**（同一把 defence 3 的尺），並同時回答「引文比對要不要正規化」 | 裁定寫進 `04` 乙-13 並回填 ADR-026 | `QA-006`、`RELEASE-007`；**封測**（封測者是第一批讀評估報告的非團隊成員） |
| ~~H-2~~ **已拍板 2026-08-23** | 負責人 | ~~**乙-14 拍板**：甲類四項是否在封測前到期。建議「到期」~~ **裁定為「並行」，與原建議相反**（[ADR-050](../../../adr/ADR-050-beta-runs-in-parallel-with-the-sandbox-acceptance.md)）。甲類四項不是封測 D 日的阻擋項；`02:SEC-009` 的 45 項門檻本身不變。**ADR-050 產生一件新工作**：同意書必須據實說明執行環境尚未完成逃逸驗收（併入乙-16 的法務清單），**並留下三條待決策**，其中第一條（第一位外部使用者的 Run 之前要不要求 Suite 1 在該節點跑過一次）**沒有答案就等於「否」** | 裁定已寫進 `04` 乙-14 與 ADR-050 | ~~封測 D 日取決於誰~~ **已解除**；改為 ADR-050 待決策 2（甲類的目標完成日仍無人寫下） |
| H-3 | 負責人 | **PDM-009 追認**：[pdm-009-beta-proposal.md §8](pdm-009-beta-proposal.md) 的十項檢查清單全部 `- [x]`。**追認時一併過報酬預算**（最大單項支出，`cost-estimation.md` 沒有任何一行涵蓋它） | 該清單十項全勾 ＋ 回寫 `03` §1 與 `04` 乙-15 | `BETA-001`／`005`、`RELEASE-009` |
| H-4 | 負責人 | **PDM-010 擇一**：首月 `min(20,30)=20` 或 20+30=50。提案自己要求「明確擇一，不要留給實作推斷」 | `internal/run/quota.go` 的四個常數拿掉「待追認」 ＋ `RUN_QUOTA` 開啟 | 配額**顯示**（強制可先做，顯示必須等值定案——乙-2 的教訓） |
| H-5 | 負責人 | **PDM-006 追認**：保存期限分級表 ＋ §6.1 的帳號刪除分類 | `DOWNLOAD_ARTIFACT_RETENTION`、`ANALYTICS_RETENTION` 與 `TRACE_RETENTION` 三者都有值（第三個是 DDD-032 新增，未設時分割輪替整個停擺，見 §2.3／§2.6）＋ 同意書 §3 的 ⬜ 填完 | `SEC-006`、`RELEASE-005`、**整個 `BETA-002`**（未定值前一列都不收） |
| H-6 | 負責人 | **PDM-008 追認**：打包目標清單與對外措辭。~~**「2 個已驗證 Profile」目前只成立 1 個**，追認時要決定改口徑還是等 H-9~~ **2026-08-23：H-9 已完成，這個數字現在是真的，追認時不必再處理它**；追認本身仍缺 | `m0/pdm-proposals.md` §9.1 該列打勾 | `PACK-006` 的決策依據 |
| H-7 | 負責人 | **PDM-004／005 的定案紀錄**（值實質已定，缺追認；PDM-005 另有「兩份文件對是否已定案說法不一致」要裁一個，乙-9） | 同上 | `03` §1、乙-9 |
| H-8 | 負責人 | **✅ 2026-08-29 已宣告：D 日 ＝ 2026-09-11。** 本列從「要一個日期」變成「要照那個日期做完」——**10 天的排程、9 位受測者的行事曆、以及在那之前必須簽完的同意書（H-11 ＋ [`05` R-11](../../05-pending-rulings.md) 的保存期改動要重走法務確認）**。⚠️ **這是本次唯一一個新的關鍵路徑風險**：R-11 把同意書 §3 的一列從 30 天改成 90 天，而那份文件自己立的規則是保存期限再變動就要重新確認一次。<br>~~**M1 閘門 D 日宣告**~~與其後 10 天（1 場 pilot ＋ 9 場正式 ＋ 分析）。**先閘門、再封測**，三個理由見 [README.md §5.3](README.md)<br>**⏳ 2026-08-23：有一個 PDM 暫時放行，本列不在它的範圍內。** 那個放行只解除 [`m5/README.md` §啟動條件](../m5/README.md) 的第 2 列，讓 M5 的**規劃**不必等閘門讀數。**本列一個字都沒被放行**——D 日仍要宣告、10 天仍要跑、`gate-test/analysis.md` 仍是本列的證據。範圍見 [`04` 乙-10](../../04-backlog-and-handoffs.md)。**若有人拿那個放行來主張封測可以開始，那是誤讀**：M5 是 MVP 之外的里程碑，封測是 MVP 之內的閘門，兩者共用「D 日」這三個字而已。<br>**同日稍晚放行擴大到 M5 的三個啟動條件全部**（[ADR-052](../../../adr/ADR-052-m5-starts-in-parallel-with-an-unfinished-mvp.md)），**其中一項逐字就是「MVP 封測結束」**——但它被放行的是「阻擋 M5 開工」這個效力，**不是封測本身**。封測仍未開始，本列仍是它的前置。**這一條現在比擴大放行前更容易被誤讀，所以講第三次**：放行讓 M5 動得了，不讓封測動得了 | `gate-test/analysis.md` 的閘門結論 | **封測不能與閘門並行**；`CONTENT-011` 的解凍也等它 |
| ~~H-9~~ **✅ 2026-08-23 完成** | 負責人 | ~~**一次本機安裝**：套件放進 `~/.claude/skills/`、`/skills` 看得到、跑一次驗證 Prompt；落檔後把 `claude-code.json` 的 `support_status` 改 `verified` 並進 `version` 版號~~ **三步全部走完**：平台經真實 HTTP 路徑產出的 `claude-code` 套件（`content_hash 6be1065…`）→ 解進 `~/.claude/skills/` → `/skills` **同一個 session** 就列出（使用者與 agent 兩邊各自確認）→ 以 profile 的 `verification_prompt` 叫用，載入成功、回出約定 marker、讀到 SKILL.md 旁的 `reference.md` | `claude-code.json` `support_status=verified`、`version` 1.1.0，落點與**不成立的部分**同寫在 `known_limitations[0]`（一個套件／一個 OS／純提示型／未裝依賴未執行腳本） | `PACK-009` **已勾**、PDM-008 的「2 個已驗證」**現在是真的** |
| H-10 | 負責人＋法務 | **`anthropics/skills` 法務終判**（乙-10）；並在寄詢問信前**擇一 4A／4B** 且**實測那四筆真的打不出包**（不得以政策文件代替實際試過） | 終判紀錄 ＋ 寄出的信 | `CONTENT-003`／`004`、`RELEASE-003` |
| H-11 | 負責人＋法務 | **同意書定稿**：[`../gate-test/consent-and-data-policy.md` §9](../gate-test/consent-and-data-policy.md) 的待填清單全部有值 ＋ 法務確認用語與法域 ＋ 未成年受測者的處理（草稿未涵蓋） | §9 十一項全勾 | **招募寄確認信時沒有東西可簽**；`BETA-001`；乙-16 |

**兩項衛生工作（不擋任何事，但別忘了）**：

- [ ] GHCR 上孤兒 image 的刪除（需 `delete:packages` scope，本機 token 沒有）
- [ ] dev 物件儲存裡的 `skillhub-seedtest` bucket 可以直接丟（容器已刪、bucket 沒有；與 `skillhub` 隔離）

**一項要決策而不只是寫程式的**：`QA-008` 的路線——要為 MVP 引入瀏覽器驅動（CI 時間、維護成本、單人團隊），還是改以人工在三個瀏覽器各走一次主要流程並落檔。**它連帶決定無障礙的對比能不能有自動守門**（丙-21）。

---

## 4. B 日當天

- [ ] §2 與 §3 全部成立（尤其 `SEC-009` 45 項全 pass、0 unknown）
- [ ] 12 個 GitHub 帳號填入 `BETA_ALLOWLIST` 並重啟；**驗 `beta.roster` audit event 真的寫成了**（寫不成 ⇒ 誰都進不來）
- [ ] 未受邀者的路徑走一次：可搜尋、可看詳情、Fork／Run／下載回 403 並指向 `POST /feedback`
- [ ] 配額走一次：剩餘次數看得到、用完會擋、重置時間顯示得出來
- [ ] 停派送開關走一次：`PUT /admin/dispatch/halt` → 建立 Run 回 503 → `DELETE` 解除 → 恢復（**解除不會自己發生**）

---

## 5. 逐條回答 `RELEASE-001`～`010`

| # | 誰做 | 做什麼 | 驗什麼 |
| --- | --- | --- | --- |
| `RELEASE-001` | agent／開發者 | 產出**需求 ID × 測試**對照表（`QA-003`／`004`／`005` 的測試現在分別掛在 `RUN-*`／`SBX-*`／`TRACE-*` 名下） | 每個 MVP 必要需求 ID 至少對到一支具名測試，且該測試最近一次執行結果有記錄 |
| `RELEASE-002` | 開發者 ＋ 一位真人 | §1.9 的 P-8（旅程測試）＋ §2.7 的第一次真實走查 | 六段的**接縫**都被走過一次；四次退回都發生在接縫上 |
| `RELEASE-003` | 負責人＋法務 | H-10；並補 2–3 個 OSI 授權的 `documents` 替代品 | `CONTENT-003`／`004` 可勾；**至少一個 `documents` 精選可下載** |
| `RELEASE-004` | 負責人＋真機 | 甲-1～甲-4 | 45 項全 pass、**0 unknown**；證據落 `sec-009-acceptance/` |
| `RELEASE-005` | 負責人 ＋ 開發者 | H-5（PDM-006）＋ §1.9 的 P-9 | 保存期限有值且可測；Run Artifact 刪得掉；刪除狀態查得到；稽核已成立（`CORE-008` 已勾） |
| `RELEASE-006` | 負責人 | `SEC-010` 的 runbook 與一鍵停用流程本體 ＋ §2.5 的 Alertmanager | 告警**送得到那個人**；停派送與解除各走過一次 |
| `RELEASE-007` | 負責人（拍板）＋ 開發者 | H-1（乙-13）＋ 依裁定改 defence 3 | 帶 `evidence_required` 的條目不會被一段沒人驗過的引文滿足 |
| `RELEASE-008` | 開發者 ＋ 負責人 | §1.9 的 P-1／P-7 ＋ H-9 | 授權檔隨包走有測試；round-trip 得同一雜湊；`claude-code` 有人真的裝過 |
| `RELEASE-009` | 負責人 | H-3 → 封測 14 天 → 三條門檻逐條計算 | B1 ≥ 8／12、B2 前六段無一段低於 50%、B3 未觸發；**不通過走決策樹，重測上限一次** |
| `RELEASE-010` | 負責人 | 收斂「已知限制」（素材 ＝ [audit.md](audit.md) 的不勾清單 ＋ `04` 三類殘項 ＋ 各 ADR 的待決策）＋ 發佈決策 ＋ 下一階段範圍 | 三份素材都被讀過且逐項有處置；`BETA-005` 的複審已完成 |

---

## 6. 這份檢查表刻意沒有做的事

| 沒做 | 為什麼 |
| --- | --- |
| **沒有給日期** | B 日取決於甲類四項，而甲類的第一個未知數在第一台節點上。給一個猜的日期會讓後面每一格都變成猜的 |
| **沒有把本檔做成可勾選的活文件** | 兩份清單一定會漂移（`04` 的既有教訓）。**勾選狀態只有一份**：`03` §18 與 `04` |
| **沒有為「不通過」寫決策樹** | 封測的已經有了（[pdm-009-beta-proposal.md §4.4](pdm-009-beta-proposal.md)）；`SEC-009` 不通過的處置是 `02:SEC-009` 明文的「不得開放」，**無例外流程，沒有第二條路可寫** |
| **沒有估工時** | 單人團隊 ＋ 大量項目取決於負責人什麼時候有空拍板。估出來的數字只會被拿去當承諾 |

## 7. 補記（2026-08-22）：`QA-008` 的路線已拍板，兩處記載作廢

**補記，不改寫上面任何一行。**

- **§1.9 末尾與 §3 的「`QA-008` 的路線」不再是待決事項。** 負責人裁定採**引入瀏覽器驅動**，**否決人工在三個瀏覽器各走一次並落檔**（[ADR-036](../../../adr/ADR-036-real-browser-verification-tier.md)，實作 commit `fe7aa13`）。
- **因此 §1.9「另有兩項要先有 §3 的決策才能動」現在只剩一項**（`SEC-012` 的自動觸發）：**無障礙的對比守門（丙-21）已不再等任何決策**，該殘項**整列結案**，`QA-009`／`DESIGN-013` 同批改判勾選。
- **`QA-008` 本身仍不勾**，因此 §5 對 `RELEASE-001` 的回答不變——它的字面判準含「與目標作業系統測試」，而 CI 仍只有 `ubuntu-latest`。**剩下的是一個尚未做的取捨（OS 矩陣要做哪幾格），不是一個沒有工具的缺口**：工具已經在了。

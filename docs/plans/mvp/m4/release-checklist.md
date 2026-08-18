# 封測上線前檢查表（`RELEASE-001`～`010` 的執行面）

- 日期：**2026-08-18**
- 這是什麼：`03` §18 的十項 `RELEASE-*` 是 `02` §7 Definition of Done 的**逐條檢查表**，但它們只寫了「要檢查什麼」，沒有寫「**誰做、做什麼、驗什麼、什麼順序**」。這份補那一半。
- **勾選狀態的唯一事實來源不是本檔**，是 [`../03-work-items.md`](../03-work-items.md) §18 與 [`../04-backlog-and-handoffs.md`](../04-backlog-and-handoffs.md)。本檔是**順序與說明**，M4 完結即凍結；部署期的實際結果回填那兩份。
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
| 停派送開關 | `internal/run/halt.go` ＋ `0030` ＋ `/admin/dispatch*` | P1 與 X-04 共用的同一個煞車 |

### 1.9 程式面尚缺（十二項，不需要任何新決策）

**這一節每一項都對應 `04` 的一條丙類殘項，補完即可重新勾選對應工作項。**

> **2026-08-18 補記（本檔凍結，只加這一段，表格原文不動）**：屬 Go／後端／工具的那些已補——**P-1／P-2／P-3／P-4／P-7／P-8／P-11 完成，P-5／P-9／P-10 的後端半邊完成**（commit `50624e2`／`8a17652`／`04bb5e6`）。`03` 的 `PACK-003`／`QA-007`／`QA-001` 三項改判勾選。**P-6／P-12 與 P-5／P-9／P-10 的前端半邊仍在 `apps/web`。**
>
> **P-7 的表格文字要當作已被推翻讀**：「刪掉平台加的三樣檔案後重新匯入，得到同一個 `content_hash`」**寫不出來**——`content_hash` 是使用者當初上傳的那份壓縮檔位元組的 SHA-256，重新壓縮同一組檔案必然不同。落地的是同一句設計的另一半（「除平台新增檔案外逐位元組相同」），並把 `standard.json` 裡叫使用者做那個檢查的那一步一併改寫。逐項見 [audit.md §7](audit.md)。

> **2026-08-18 第二次補記（本檔凍結，只加這一段，表格與其後的段落原文不動）**：**§1.9 的十二項全部完成。** P-6 與 P-12、以及 P-5／P-9／P-10 的前端半邊由 UI 批與其後的收尾批補上；`03` 的 `DESIGN-012`／`WS-004`／`PACK-007`／`CORE-007`／`O11Y-004` 五項改判勾選（前兩項見 [audit.md §8](audit.md)，後三項見 [§9](audit.md)）。
>
> **本節末尾那句「另有兩項要先有 §3 的決策才能動」要當作已被推翻讀**（同 P-7 的處置）。**兩項都不需要新決策，而且兩項的可做部分都已經做完**：①`SEC-012` 的自動觸發——「接哪幾條判準」不是偏好而是查得出來的事實，平台自己看得見的兩條（`TraceMaskingStopped`、Reconciler 停擺 > 10 分鐘）已接上，其餘三條的訊號在本 process 之外，屬 §2 部署期；**`SEC-012` 仍不勾**。②無障礙的對比守門——`QA-008` 管的是**算繪後的合成像素**，而 token 層的靜態十六進位值不需要版面計算，已落地為 `apps/web/src/contrast.test.ts`；合成像素（alpha、`opacity`、真瀏覽器算繪）仍無守門，**`QA-009`／`DESIGN-013` 仍不勾**。**兩次是同一個形狀：一段不需要決策的工作，被掛在一個真的要人拍板的東西旁邊一起等。** 逐項見 [audit.md §9](audit.md) 與 [`04` 丙-21／丙-26](../04-backlog-and-handoffs.md)。

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

## 2. 部署期（真機，`SEC-009` 未過即不得開放外部使用者提交 Skill 執行）

**依據不是新的**：ADR-015 定案紀錄。M0～M3 能把甲類當平行工作是因為**沒有任何外部使用者**；封測第一次讓外部人員在真實部署上建立 Run。裁定見 [README.md §4](README.md)，拍板見 [`04` 乙-14](../04-backlog-and-handoffs.md)。

### 2.1 甲類四項（封測阻擋項）

| # | 誰 | 做什麼 | 驗什麼 |
| --- | --- | --- | --- |
| 甲-1 | 負責人＋真機 | `SEC-009` 十個測項全跑 | **45 項基線全數 pass、0 項 unknown**。任一 fail 或 unknown 即不得開放，**無例外流程**。證據落 `m4/sec-009-acceptance/<日期>-<節點>/`（判定表 ＋ `versions.txt` 進 repo，原始輸出留 CI artifact 並附連結），保存 ≥ 1 年 |
| 甲-2 | 同上 | `SBX-010` 的工作項側 | 同甲-1。**現有的真實容器驗證（非 root、唯讀 rootfs、無主機掛載、pids 上限、逾時強停、清理冪等）不等於逃逸測試通過** |
| 甲-3 | 同上 | `SBX-005`／`007` 的生產網路面 | 每 Run netns ＋ `--icc=false`（關掉 dev 現存的**不需逃逸**的跨 Run 橫向路徑）；nftables default-deny ＋固定 DNS；`infra/egress/allowlist.yaml` 的 `pinned_ip` 已填實際值且**不是控制平面節點**（ADR-022 Q2 強制條件 6，由測項 T5-7 抓）。**LiteLLM 必須移到沙箱面專屬節點**——現行 compose 的 `127.0.0.1:4000` 是 dev 形態，生產不可複製 |
| 甲-4 | 同上 | `SBX-002` 的閘門 A 節點准入探針 | 探針在**真實節點上**查得到已發佈映像的 SBOM 與掃描 attestation；到期前 7 天告警的**發送端**已接 |

**兩個要在第一台節點上先確認的未知數**（[README.md §10 R1](README.md)）：

1. **gVisor 的 `systrap` 平台是否真的不需巢狀虛擬化**（ADR-022 只說「待部署批第一台節點實測確認」）。**把「一台節點、跑通一個 Run」獨立成最小驗證，不要等其餘做完才發現節點跑不起來。**
2. `infra/nodes/gvisor-baseline.txt` 必須填實際版本（**非 `unset`**），否則 `SEC-009` 的前置條件②不成立、整批判 unknown ＝ fail。

### 2.2 Migration 套用（順序是硬的）

**執行中的 dev DB 停在 `0026`。** 缺的是 `0024`／`0025`／`0026` 之後的七份，而它們互相依賴：

```bash
# 順序不可調換；每一份都 --single-transaction，中途失敗不留半套
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0024_evaluation.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0025_suggestion_proposed_content.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0026_test_case_rubric.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0027_packaging.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0028_download_retention_and_object_reconcile.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0029_beta_analytics.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0030_dispatch_halts.sql
# 然後才是資料回填（不是 migration，可重跑）
psql -v ON_ERROR_STOP=1 --single-transaction -f tools/content/backfill-redistribution.sql
```

**驗什麼**：回填應回報 **`UPDATE 90`**，分佈為 **41 `allowed`／4 `blocked`／0 `unknown`**（4 筆 blocked ＝ `anthropics/skills` 的 `docx`／`pdf`／`pptx`／`xlsx`）。**數字對不上就不要往下做**——打包的授權閘門是照這個欄位擋的。該分佈已在一個乾淨的拋棄式部署上獨立複現過（[README.md §14.2](README.md)）。

**套用之後必須做的三件事**：

- [ ] **重建 `cmd/api`／`cmd/worker`／`sandboxd` 到 HEAD** 並帶齊環境變數（見 §2.3）。舊二進位在新 schema 上會以難看的方式失敗（丙-8 記錄的正是反過來的那一半）。
- [ ] **`0024`～`0026` 套用後，`EVAL-013` 的 B 輪回歸解除**（丙-8）：依 [`../m3/report-judge-regression.md` §13.4](../m3/report-judge-regression.md) 的五個 `test_case_id` 走 `PATCH /test-cases/{id}` 改 Prompt → 走既有 Run 路徑重發 5 筆 → `judge_regression.py --rubric` 重評。**第 ⑤ 步之前先確認新 Run 進得了 `skill_runtime_compatibility`**，否則 harness 取到的仍是 M2 的舊 Run 而且不會有任何提示。成本約 $0.3～0.5。
- [ ] **`0027` 套用前，dev 上的打包路徑只會回 `license_unknown`**（`redistribution` 欄位不存在 ⇒ fail-closed）。這是正確行為，不是故障——回填前的實測已記在 §14.2 末段。

### 2.3 部署設定（未設會安靜地關掉功能）

| 變數 | 不設的後果 | 值從哪來 |
| --- | --- | --- |
| `PACKAGING_PROFILES_DIR` | 零個打包目標，**每條打包路由 503** | 預設 `contracts/packaging/profiles` |
| `DOWNLOAD_ARTIFACT_RETENTION` | 走程式內的 90 天預設 | **PDM-006 追認**（§3） |
| `ANALYTICS_RETENTION` | **不設 cookie、不寫任何一列** ⇒ `BETA-002` 的漏斗量不到任何東西 | **PDM-006 追認**（ADR-029 提案 180 天） |
| `BETA_ALLOWLIST` | 閘門關閉，**任何有 GitHub 帳號的人都能用** | PDM-009 追認後的 12 個 GitHub 帳號 |
| `RUN_QUOTA` | 額度不強制，`GET /me/quota` **不掛載**，preflight 不帶配額區塊 | PDM-010 擇一後開啟 |
| `OPERATOR_USER_IDS` | 沒有人能操作 `/admin/dispatch*`（P1 停派送只能改 DB） | 負責人自己 |
| `SKILLHUB_MODEL_GATEWAY_*` | 沙箱被派到 `--network none`，Run 全部失敗 | 既有 |
| 物件儲存設定 | `ErrNoStore` ⇒ 打包 503 | 既有 |

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

- [ ] `cmd/maintenance purge-accounts` 接上 cron（**程式刻意不自帶 scheduler**）；`PURGE_GRACE` 預設 720h
- [ ] 物件存在性對帳器一小時一輪且**刻意沒有 `RunOnStart`** ⇒ **部署後第一小時是空窗**，知道就好
- [ ] 驗對帳器要兩輪才標記（`object_reconcile_sightings`），且無法連線的儲存產生**零次觀測**而不是一次假的

### 2.7 上線後的第一次真實驗證

- [ ] 一個人走完 `02` §7 DoD 第一條的旅程（意圖搜尋 → 詳情 → Fork → 試跑 → 評估 → 打包下載），落檔
- [ ] 驗 `anthropics/skills` 那四筆**真的打不出包**（兩道鎖各驗一次：撤下 hold 之後 `redistribution` 仍應擋住）——**這是寄詢問信前的實測要求**（§3）
- [ ] 驗至少一個 `documents` 類精選 Skill **可下載**（[beta-design.md §8](beta-design.md) 第 11 項；那四筆受限讓該類最好的樣本不可下載，PDM-002 早已要求補 2–3 個 OSI 授權的替代品，**那是 `documents` 類的必要條件不是加分項**）

---

## 3. 負責人動作（agent 做不到）

| # | 誰 | 做什麼 | 驗什麼（什麼算完成） | 擋住什麼 |
| --- | --- | --- | --- | --- |
| H-1 | 負責人 | **乙-13 拍板**：G7／G8 二選一——(a) `artifact` 型引用不得滿足 `evidence_required`；(b) 保留但標示「此引文未經回驗」。**G7 與 G8 一起裁**（同一把 defence 3 的尺），並同時回答「引文比對要不要正規化」 | 裁定寫進 `04` 乙-13 並回填 ADR-026 | `QA-006`、`RELEASE-007`；**封測**（封測者是第一批讀評估報告的非團隊成員） |
| H-2 | 負責人 | **乙-14 拍板**：甲類四項是否在封測前到期。建議「到期」；替代路徑「只讀不跑的封測」的代價已寫清楚（北極星指標量不到、DoD 第一條走不完） | 裁定寫進 `04` 乙-14 | 封測 D 日取決於誰 |
| H-3 | 負責人 | **PDM-009 追認**：[pdm-009-beta-proposal.md §8](pdm-009-beta-proposal.md) 的十項檢查清單全部 `- [x]`。**追認時一併過報酬預算**（最大單項支出，`cost-estimation.md` 沒有任何一行涵蓋它） | 該清單十項全勾 ＋ 回寫 `03` §1 與 `04` 乙-15 | `BETA-001`／`005`、`RELEASE-009` |
| H-4 | 負責人 | **PDM-010 擇一**：首月 `min(20,30)=20` 或 20+30=50。提案自己要求「明確擇一，不要留給實作推斷」 | `internal/run/quota.go` 的四個常數拿掉「待追認」 ＋ `RUN_QUOTA` 開啟 | 配額**顯示**（強制可先做，顯示必須等值定案——乙-2 的教訓） |
| H-5 | 負責人 | **PDM-006 追認**：保存期限分級表 ＋ §6.1 的帳號刪除分類 | `DOWNLOAD_ARTIFACT_RETENTION` 與 `ANALYTICS_RETENTION` 有值 ＋ 同意書 §3 的 ⬜ 填完 | `SEC-006`、`RELEASE-005`、**整個 `BETA-002`**（未定值前一列都不收） |
| H-6 | 負責人 | **PDM-008 追認**：打包目標清單與對外措辭。**「2 個已驗證 Profile」目前只成立 1 個**，追認時要決定改口徑還是等 H-9 | `m0/pdm-proposals.md` §9.1 該列打勾 | `PACK-006` 的決策依據 |
| H-7 | 負責人 | **PDM-004／005 的定案紀錄**（值實質已定，缺追認；PDM-005 另有「兩份文件對是否已定案說法不一致」要裁一個，乙-9） | 同上 | `03` §1、乙-9 |
| H-8 | 負責人 | **M1 閘門 D 日宣告**與其後 10 天（1 場 pilot ＋ 9 場正式 ＋ 分析）。**先閘門、再封測**，三個理由見 [README.md §5.3](README.md) | `gate-test/analysis.md` 的閘門結論 | **封測不能與閘門並行**；`CONTENT-011` 的解凍也等它 |
| H-9 | 負責人 | **一次本機安裝**：套件放進 `~/.claude/skills/`、`/skills` 看得到、跑一次驗證 Prompt；落檔後把 `claude-code.json` 的 `support_status` 改 `verified` 並進 `version` 版號 | 落檔證據 ＋ 兩個欄位改動 | `PACK-009`、PDM-008 的「2 個已驗證」 |
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

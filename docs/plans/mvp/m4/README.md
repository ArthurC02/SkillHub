# M4：打包與封閉測試 — 執行計畫

- 日期：2026-08-17（計畫）／**2026-08-18（M4 程式面收斂，本目錄自此凍結）**
- 狀態：**程式面已收斂，封測待部署期與負責人動作。** 逐項對帳見 [audit.md](audit.md)（49 項：**11 勾選、38 誠實不勾、零項退回**），封測上線前的完整檢查單見 [release-checklist.md](release-checklist.md)。**本文件維持計畫時的原文**（§1～§12 除檔案地圖外一字未改）——它記的是「M4 開工時看到的東西」，而對帳記的是「實際交出了什麼」，兩份不互相改寫。
- **舊狀態原文**：**計畫；程式面未開工，文件面已跑過一批。** 本文件是 M4 的入口：範圍、接點對應、批次分解、程式／人的分界、未決點與檔案地圖。
  - **2026-08-17 文件批已完成（不含任何產品程式碼、不勾選任何工作項）**：①§8.2 建議的三份 ADR 全部已立並 Accepted（[ADR-027](../../../adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md)／[028](../../../adr/ADR-028-beta-admission-and-quota-enforcement-points.md)／[029](../../../adr/ADR-029-product-analytics-events-and-audit-trace-boundaries.md)），刻意不立的兩項維持不立；②§7 的八項 `02`／`03` 差異依建議全數落地；③§8.4 的可散布性欄位與 §8.5 的 DESIGN 處置**已裁定**（併入 ADR-027 決策 4 與 `03` §3）；④§8.1 缺的 PDM-009 提案已補（[pdm-009-beta-proposal.md](pdm-009-beta-proposal.md)，**待追認**）；⑤§6.2 的詢問信草稿已修訂。**第 1 批的前置決策因此只剩 PDM-008 的追認**。
- 前提：M3 已完結（[../m3/README.md](../m3/README.md)、[../m3/audit.md](../m3/audit.md)）；**M1 驗證閘門的 D 日仍待負責人宣告**（[../gate-test/](../gate-test/)）——與 M0～M3 不同的是，**M4 的封測與那個閘門不能再平行**，理由見 §5.3。
- 上游輸入：[`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) 的「移交 M4」六條接點（逐項對應見 §3）、甲類四項（§4）、乙類三項（§8.3）、丙類五項未結案（§3.7）。
- 這是 MVP 的**最後一個里程碑**。做完 M4，`02` §7 的 Definition of Done 九條要全部成立——`03` §18 的 RELEASE-001～010 是它的逐條檢查表。

## 1. 一句話範圍

> M4 把「平台裡的不可變 Skill Version」變成「使用者手上一個能解壓、能重新驗證、裝得進目標 Agent、不含 Secrets、帶著來源與授權的檔案」，並在真實部署上讓一批外部封測者把 `01` §6 的完整旅程走完一次。

**但這句話的後半有一半不是程式能完成的**——真機部署、SEC-009 的十個測項、招募、D 日宣告、成功門檻判定都是人的動作。分界逐項寫在 §5，**本計畫不假裝 agent 做得到那一半**。

## 2. 進與不進

### 2.1 進 M4 的需求 ID

| 需求 ID | 承接工作項 | 備註 |
| --- | --- | --- |
| `02:PACK-001` 標準 Agent Skill 套件 | `03` PACK-001／002／003／004／005 | 五條允收準則逐條對應五個工作項，見 [packaging-design.md](packaging-design.md) §1 的對照表 |
| `02:PACK-002` 安裝說明 | `03` PACK-006／007／008 | 打包目標取 PDM-008 提案的 **1 標準套件 ＋ 2 安裝 Profile**（`standard`／`claude-code`／`claude-agent-sdk`）；PDM-008 尚未追認，見 §8.1 |
| （跨兩者） | `03` PACK-009 | 「可被解壓、重新驗證與依說明安裝」——**「重新驗證」程式做得到，「依說明安裝」的最後一哩要人**，見 §5.2 |
| `02:WS-002` 第 1 條的「**下載紀錄**」 | `03` WS-004（掛在 §8／M1，未勾） | M1 當時沒有下載這個動作，紀錄自然是空的；M4 才有可列的東西。**不搬章節**，比照 `TEST-012` 前例只加排期註記（§7 差-3） |
| `02:NFR-001` 第 4 條「重要操作保留稽核事件……包括**下載**」 | `03` CORE-008（掛在 §5／M1，未勾） | 下載端點是 CORE-008 最後一個必要事件源（ADR-007「稽核事件」節明列 Artifact 下載）。同上不搬章節（§7 差-5） |
| `02:SEC-006` 保存及刪除政策 | `03` CORE-007、`04` 丙-9 | Download Artifact 的保存期限是 PDM-006 提案的最後一列，追認後才可判定（§8.1） |
| `02:SEC-007` 來源、License 與下架處理政策 | `03` PACK-003／004 的一半、`03` SEC-007 | 「授權未知或不可散布者不得進入打包流程」是本節與 PACK 的接縫；`anthropics/skills` 四筆是它的第一個真實案例（§6.2） |
| `02:SEC-009`／`02:SEC-002` | `03` SEC-009、SBX-002／005／007／010、SEC-012 | **部署期，不是程式期**；甲類四項在 M4 從「平行工作」變成「封測阻擋項」，見 §4 |
| `02:SEC-010` 事件回應與緊急停用 | `03` SEC-010、SEC-012 | SEC-012 明文「落地時機在部署批之前」，因此排在 M4 的實作批而非部署批 |
| `01` §11 的核心漏斗 | `03` BETA-001～005、QA-001～009、RELEASE-001～010 | 漏斗的**量測管線目前沒有任何工作項承接**，見 §7 差-4 |

### 2.2 不進 M4（附理由）

| 不進的東西 | 理由 |
| --- | --- |
| **Agent Profile 的 plugin／擴充機制** | ADR-012 待決策 2（Profile 設定格式與 Adapter Plugin 機制）。MVP 只有三個打包目標且全部內建，先做一套外掛機制是為一個不存在的第二方鋪路（`01` §7.2「少量安裝 Profile」）。Profile 以**版本化設定 ＋ 內建 Adapter**落地即滿足 ADR-012 |
| **Codex／Cursor／Gemini CLI 的 Profile** | PDM-008 明文：這些平台在 Skill Hub 上一次都沒跑過，加進來三層相容性全部只能填「未驗證」，Profile 退化成猜測的路徑表，正面違反產品原則 5。M4 之後依 `BETA-005` 的需求訊號再評估 |
| **打包產物的數位簽章與撤銷** | ADR-012 待決策 3。它需要一組簽章金鑰的生命週期（產生、保管、輪替、撤銷清單），而 MVP 的下載是**登入後的短效授權**、不是公開發布——簽章要解決的「拿到檔案的人怎麼知道是你給的」在這個形態下由 Session ＋短效 URL 已經回答。建議寫進 ADR-027 作為明文的**不做**，而不是留在待決策裡（§8.2） |
| **使用者自備模型 API Key（BYO Key）** | PDM-010 提案取選項 B（免費額度、不做 BYO）。ADR-017 已寫明 BYO 是閘道的功能、平台程式碼不變，所以這是產品時機不是架構問題 |
| **公開 Beta、Marketplace、付費** | `01` §7.3 明列不含。M4 的產出是「依封測結果決定公開 Beta 的範圍」（`03` BETA-005），不是公開 Beta 本身 |
| **遠端 MCP／Local Runner 的打包面** | 兩者已移出 MVP 首發。Profile 的 MCP 設定對映欄位留型別佔位並顯示為不適用，同 `TRACE-003`／`EVAL-008` 的既有前例 |
| **多 Skill 平行 Benchmark、跨模型 Judge A／B** | `01` §7.3 與 M3 §2.2 已排除，M4 不重開 |
| **`CONTENT-011` 增強重跑** | 負責人已裁定 D 日前不執行（`03` CONTENT-011）。**它在 M4 會到期**——D 日一過、閘門結案，凍結標的解除，這一項就該排入；但它屬內容批不屬打包批，見 §5.3 |
| **`artifacts` 表的歷史回填與 M2 Run 補評**（丙-13） | 順序是硬的（先回填再評估），但它服務的是「補評 M2 歷史 Run」這個目前沒有人要求的動作。M4 不做，`04` 丙-13 的警告維持開著 |

## 3. 「移交 M4」六條接點的逐項對應

**六條全部進計畫，一條不漏**（出處：[`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) §移交 M4）。

| 接點 | M4 怎麼接 | 落在哪一批 |
| --- | --- | --- |
| **M4-1** `PACK-002`「打包前重新執行規格驗證」已有前例 | **重用 `EVAL-010` 的那一條路徑**：對打包後的位元組跑同一個 `skillpkg.Validate`，任一阻擋級 finding 即整批拒絕、不產生 Download Artifact。**不另寫一套「打包用的判準」**——兩套會在「什麼算阻擋級」上漂移，而漂移的方向必然是打包側比較鬆（因為它在使用者最想成功的那一步）。設計見 [packaging-design.md](packaging-design.md) §3 | 第 2 批 |
| **M4-2** `PACK-003`「保留衍生關係」對建議生成的版本已有答案 | 溯源**由 `evaluation_suggestions.applied_skill_version_id` 反向查詢**取得（「哪幾條建議造出它、出自哪一份評估」），不是一個 `derived_from_evaluation_id` 欄位——後者不存在且刻意不補（[../m3/evaluation-design.md](../m3/evaluation-design.md) §5.3 的 2026-08-17 更正）。打包 manifest 記的是**這個關係的結果**（來源 Skill、來源版本、Fork 鏈、由哪份評估的哪幾條建議改出），不是關係本身 | 第 1 批（形狀）＋第 2 批（實作） |
| **M4-3** `PACK-005` 與丙-12 是同一件事的兩半 | 一起做。`PACK-005`「可散布的 Test Case 與範例資料」這條準則已經預設 Test Case 搬得動；丙-12 說的是策展側目前沒有「種進全新部署」的形式。**共用同一個可攜 Test Case 形狀**（[packaging-design.md](packaging-design.md) §5）：打包時匯出它、種入部署時匯入它，一份 schema 兩個方向。分開做會得到兩種格式 | 第 1 批（schema）＋第 3 批（匯出）＋第 7 批（策展側種入） |
| **M4-4** `PACK-004`「排除 Secrets 與 Run 資料」要涵蓋評估產物 | **明文列入排除清單**：建議的 `problem`／`expected_impact`／證據 `excerpt`、rubric、評估報告、Trace、Run artifact **一律不進套件**——它們是 Run 資料不是 Skill 內容。目前沒有任何規則說這件事，M4 要把它寫成**白名單而非黑名單**（[packaging-design.md](packaging-design.md) §4）：只有來自該 Skill Version 套件位元組的檔案能進包，其餘一律不進。黑名單漏一項就是洩漏，白名單漏一項只是少一個檔案 | 第 1 批（規則）＋第 2 批（實作） |
| **M4-5** 封測會第一次讓非團隊成員讀到評估報告 | 三件事在封測前要有答案：①ADR-025 待決策（執行狀態與任務判定兩列的實際文案與版面）——承接者是 `DESIGN-010`／`011`，而 §3 全 13 項零勾選，所以它其實**沒有承接者**（§8.5）；②`04` **乙-13（G7）**必須拍板，否則封測者會讀到「這條要求引文 → 通過 → 證據是 `report.md, 4096 bytes`」（§8.3）；③`EVAL-013` 報告 §2 的三個空格，**注入抵抗那一格的樣本可以合成、不需真 Run，是三格裡最便宜的**，M4 補這一格 | 第 5 批（②③）＋§8.5（①待拍板） |
| **M4-6** 甲類四項是外部使用者開放的硬前提 | **裁定見 §4：四項全部在封測前到期。** 依 ADR-015 定案紀錄，`SEC-009`／`SBX-010` 未通過不得開放外部使用者提交 Skill 執行；封測的漏斗（`03` BETA-002）第三段就是「試跑」，所以封測必然涉及外部人員實際跑 Run | 部署批（非程式批），§5.2 |

### 3.7 丙類五項未結案的處置

| # | M4 怎麼處置 |
| --- | --- |
| 丙-8 `EVAL-013` 的 **B 輪回歸未跑** | **排入第 5 批。** 它要發 5 次真實 Run（沙箱、映像、閘道費用）並產生新的 Test Case 快照，而 M4 本來就要在部署後跑 `QA-003`／`PACK-009` 的真實 Run，兩者可共用同一次環境開機。**成本要先講**：5 筆 Run ＋ 5 次 Judge，以 M2 基準的中位數估約 $0.3～0.5 |
| 丙-9 `inputs_available` 缺物件存在性**對帳器** | **排入第 4 批，與 `CORE-007`（刪除流程）一併承接**——`04` 已指出兩者重疊。理由是 M4 第一次有「使用者刪掉東西之後平台還宣稱它在」的產品後果（下載紀錄指向一個已被清掉的 artifact） |
| 丙-10 UI 分不出「引用回驗失敗」與「模型自己說不知道」 | **排入第 6 批**（UI 批）。資料層已經分得出來（`reason` 以 `evidence_unverifiable: ` 開頭），缺的只是呈現面。與 M4-5 ②同一個理由：封測者是第一批會被這個模糊性誤導的人 |
| 丙-12 策展內容沒有「種進全新部署」的路徑 | 見 M4-3，與 `PACK-005` 一起做 |
| 丙-13 歷史 M2 Run 的 `artifacts` 表全空 | **不做**（§2.2）。警告維持開著；M4 不補評 M2 歷史 Run |

## 4. 甲類四項：封測前是否到期的裁定

**建議裁定：四項全部到期，且是封測 D 日的阻擋項，不是平行工作。**

依據不是新的——ADR-015 定案紀錄早就寫著「`SEC-009`／`SBX-010` 未通過**不得開放外部使用者提交 Skill 執行**」。M0～M3 之所以能把甲類當平行工作，是因為那段期間沒有任何外部使用者；M4 的封測**第一次讓外部人員在真實部署上建立 Run**，前提就此成立。

| # | 項目 | 在 M4 的地位 | 誰做 |
| --- | --- | --- | --- |
| 甲-1 | `SEC-009` 十個測項、45 項基線全數 pass 且 0 unknown | **封測阻擋項**。無例外流程（`02:SEC-009`）。證據落 `m4/sec-009-acceptance/<日期>-<節點>/`，該路徑已由 ADR-022 預留 | 部署批（人 ＋ 真機） |
| 甲-2 | `SBX-010` 同上的工作項側 | 同甲-1 | 同上 |
| 甲-3 | `SBX-005`／`007` 的生產網路面 | **封測阻擋項，且是甲-1 的前置**——T5（網路外洩八子項）與 T10（P-02 常駐探針）測的正是這一面。含 ADR-022 Q2 強制條件 6：LiteLLM 必須移到沙箱面專屬節點 | 同上 |
| 甲-4 | `SBX-002` 的閘門 A 節點准入探針 | **封測阻擋項**，是 `SEC-009` 的前置條件①（受測映像須已發佈至 GHCR 且附 SBOM 與掃描 attestation，且探針要在真實節點上查得到） | 同上 |

**唯一的替代路徑，以及為什麼不建議**：封測可以設計成「只讀不跑」——只搜尋、看詳情、下載既有精選套件，不建立任何 Run。那樣甲類確實不必到期。**不建議**，兩個理由：①`03` BETA-002 要量的漏斗有六段，只讀不跑等於量前兩段，而 `01` §11.1 的北極星指標是「每週通過使用者驗收並完成打包的 Skill 數量」——沒有 Run 就沒有驗收；②`02` §7 的 DoD 第一條要求新使用者走完「意圖搜尋 → 詳情 → Fork → **試跑** → 評估 → 打包下載」，只讀不跑的封測不能用來宣告 MVP 完成。

**這一項需要負責人拍板**（§8.3 乙-14），因為它決定的是封測的日期取決於誰：取「四項到期」，封測日期由部署批決定；取「只讀不跑」，封測可以更早但 MVP DoD 走不完。

## 5. 程式能完成 vs 負責人／部署期動作

**這一節是本計畫最重要的一節。** M0～M3 的殘項一再證明，把「人的動作」寫成工作項會讓帳面看起來在前進而實際沒有。

### 5.1 程式能完成（agent 批次做得完）

| # | 事項 | 落在 |
| --- | --- | --- |
| P-1 | 打包管線：匯出形狀、規格重驗、Secrets 與 Run 資料排除、License 與溯源隨附、manifest 與規範化雜湊 | 批 1、2 |
| P-2 | 三個打包目標（`standard`／`claude-code`／`claude-agent-sdk`）與安裝說明產生、未驗證 Agent 的限制揭露 | 批 3 |
| P-3 | 可攜 Test Case 與範例資料的匯出／匯入形狀（`PACK-005` ＋丙-12） | 批 1、3、7 |
| P-4 | 下載授權：Workspace scope、短效 URL、`access_restriction` 阻擋、下載紀錄（`WS-004`）、下載稽核事件（`CORE-008`） | 批 4 |
| P-5 | 物件存在性對帳器（丙-9）與使用者刪除流程（`CORE-007`） | 批 4 |
| P-6 | 免費額度的**強制點**與退還語意、封測邀請閘門（若拍板要做） | 批 5 |
| P-7 | 漏斗事件的產生與儲存、封測回饋收集入口 | 批 5 |
| P-8 | `SEC-012`：P1 事件的自動第一動作（停止派送、drain、保留現場），與 ADR-022 X-04 **共用同一個開關** | 批 5 |
| P-9 | 注入抵抗的合成樣本回歸（`EVAL-013` 三個空格裡最便宜的那一格）、`EVAL-013` B 輪 | 批 5 |
| P-10 | UI：下載頁、安裝說明、工作區的下載紀錄、丙-10 的呈現面 | 批 6 |
| P-11 | `QA-001`～`009` 的自動化部分、`RELEASE-001`／`002` 的檢查表落檔 | 批 7 |
| P-12 | 同意書與受測者資料保存政策的**文件本體**（招募材料缺的那一塊，見 §6.3） | 批 5 或 7 |

### 5.2 負責人／部署期動作（agent 做不到，本計畫只列清單）

| # | 事項 | 阻擋什麼 | 負責人動作是什麼 |
| --- | --- | --- | --- |
| H-1 | **真機部署**：Linux 節點、gVisor `systrap` 實測、nftables ＋固定 DNS、每 Run netns ＋ `--icc=false`、LiteLLM 移到沙箱面專屬節點 | 甲-1～甲-4 全部 | 開機器、跑部署、回填 `infra/nodes/gvisor-baseline.txt` 與 `infra/egress/allowlist.yaml` 的 `pinned_ip` |
| H-2 | **`SEC-009` 十個測項實跑**，45 項全 pass、0 unknown | 封測開放 | 依 ADR-022 第三部分逐項執行，判定表與 `versions.txt` 進 `m4/sec-009-acceptance/` |
| H-3 | **閘門 A 節點准入探針**在真實節點上查得到 SBOM 與掃描 attestation；到期前 7 天告警的**發送端** | 甲-4、`SBX-002` 勾選 | 部署探針、接 Alertmanager |
| H-4 | **Alertmanager 部署、通知路由與 Grafana dashboard**；`O11Y-003` 的門檻值上線後回填 | `O11Y-003` 的完整性、`SEC-010` | 部署與校準 |
| H-5 | **M1 閘門 D 日宣告**與其後 10 天的執行（9 場 ＋ pilot） | 見 §5.3 | 排程、凍結、找主持人與記錄員 |
| H-6 | **`anthropics/skills` 的法務終判**（乙-10） | `CONTENT-003`／`004` 勾選；**以及 `PACK-001` 上線後上游詢問信裡「we do not offer these Skills for download」那句話的真偽**（§6.2） | 與法務定案，或寄出 `governance/anthropic-sa-inquiry-draft.md` |
| H-7 | **PDM-004／005／006／008／009／010 的逐項定案紀錄** | `03` §1 六項未勾；PDM-008／009／010 直接決定 M4 的形狀 | 在 `m0/pdm-proposals.md` §9 的定案檢查清單上逐項打勾，或裁定不同的值 |
| H-8 | **招募封測者**（`BETA-001`）：發文案、篩選、排程、報酬給付 | 整個 §17 | 用 `gate-test/recruit.md` 的既有材料（可重用，見 §6.3） |
| H-9 | **封測成功門檻的判定**與 `BETA-005` 範圍複審、`RELEASE-009`／`010` 的發佈決策 | MVP 宣告完成 | 依 PDM-009 定的門檻對照結果 |
| H-10 | GHCR 上孤兒 image 的刪除（需 `delete:packages` scope，本機 token 沒有） | 無（衛生） | 用有 scope 的 token 刪 |
| H-11 | **乙-13（G7）拍板**、**§8.5 的 DESIGN 系列處置拍板**、**§4 的甲類到期裁定** | 見各節 | 二選一即可，不需要寫程式 |

### 5.3 封測與 M1 閘門的關係（這一條必須明寫）

M2 與 M3 都與 M1 閘門**並行**，理由在兩份計畫裡寫得很清楚：那兩個里程碑的技術內容不受閘門結果影響。**M4 不同，三個理由**：

1. **閘門不過的處置是「先修正搜尋與內容，不進入 M2」**（`01` §M1）。閘門的重測規則（`gate-test/analysis.md` §6.3）明文「`03-work-items.md` 不勾選任何工作項目」——而 M4 的收斂批要勾的正是 `RELEASE-001`～`010`。**在一個未結案的閘門上宣告 MVP 完成，是把兩個相反的結論同時寫進同一份文件。**
2. **兩批受測者不能混用**：`gate-test/recruit.md` §5.2 第 11 項已建議「9 位閘門受測者不計入 PDM-009 的封測人數」，因為他們已知答案。反過來也成立——封測者若先看過產品，就不能再當閘門受測者。**先跑閘門、再跑封測**是唯一不浪費樣本的順序。
3. **閘門凍結的是搜尋與內容**（增強 prompt 版本分佈、`MaxCosineDistance = 0.75`、目錄筆數）。M4 的打包批不碰這三樣，所以**打包可以與閘門並行**；**封測不行**。

**因此本計畫的排序建議是**：批 1～4（打包管線與下載）與閘門並行 → 閘門 D 日與 D+10 結論 → 部署批（H-1～H-3）→ 批 5～7 ＋封測。這個排序不需要任何新決策，它只是把既有規則接起來。

## 6. 三個必須先講清楚的界線

### 6.1 匯出＝匯入的逆向，而且必須過自家匯入驗證

`PACK-001` 的第一條允收是「下載內容保留正確的 Skill 目錄結構與 `SKILL.md`」。這句話有一個可機械判定的形式：**把匯出的套件重新丟回 `SKILL-001`／`SKILL-002` 的匯入路徑，必須得到同一個內容雜湊、零阻擋級 finding。** 匯出與匯入若不是同一份規則的兩個方向，平台就會產出自己都收不下的套件——而 `PACK-009` 的「可被重新驗證」正是在問這件事。設計與具名測試形狀見 [packaging-design.md](packaging-design.md) §2、§6。

### 6.2 `anthropics/skills` 四筆：打包上線之後，一句既有的承諾會變成可被證偽的

`governance/anthropic-sa-inquiry-draft.md` §3 已經自己點名了這件事：詢問信裡寫著「We do not offer these Skills for download」，而**那句話在 `PACK-001` 上線之前為真是因為根本沒有下載功能**。上線之後它為真的唯一理由，是打包管線確實擋掉 `access_restriction IS NOT NULL` 的 Skill。

因此 M4 的打包管線有一條**不可協商的阻擋規則**：`access_restriction` 非 NULL 即不得產出任何 Download Artifact（含 `standard`），未知原因碼 fail-closed 視為受限——與 `/files` 的 403 與 `POST /runs` 的 422 同一份旗標、同一個方向。`02:CONTENT-002` 的「**已人工確認不等於可再散布**」是同一條規則的另一半：**License 狀態為 `Confirmed` 不得成為放行條件**。具名測試見 [packaging-design.md](packaging-design.md) §4.3。

### 6.3 封測招募：可重用的與必須新做的

閘門材料（`gate-test/recruit.md`）已經備妥且**可整套重用**：招募文案兩版、5 題篩選問卷、三層受測者定義與分層規則、排除條件、報酬設計四原則（不用產品額度折抵、對應 1 小時市場價、當下無條件給付、未完成場次照給）。改的只是把「40 分鐘一對一」的框架換成封測的時間跨度。

**必須從零做的只有一項：同意書與受測者資料（錄影／逐字稿／查詢原文）的保存與刪除政策。** `recruit.md` §5.2 第 5、6 項已經把它掛在那裡並點名了對齊對象（M0 的資料保存政策與 NFR-002 的遮罩要求）。做一次，M1 閘門與 M4 封測共用。這件事的**文件本體 agent 寫得出來**（P-12），**簽署與法務確認是人的動作**。

## 7. 與 `03-work-items.md` 的差異（**本計畫不改 `03`**）

比照 M3 §5 的慣例：差異記在這裡，由負責人裁定後再由後續批次回寫 `02`／`03`。

> **2026-08-17 更新：八項差異已由文件批依本節建議全數落地**（負責人授權「依最佳實務做決策」）。**本節維持原文不改寫**——它記的是「M4 開工時 `03` 長什麼樣」，那是一份某一時點的帳；落地結果逐項記在下表最右欄新增的「已落地」註記。**沒有任何工作項因此被勾選**：這一批補的是可判定性與承接者，不是完成度。

| # | 差異 | 說明 | 建議處置 |
| --- | --- | --- | --- |
| 差-1 | **`03` §3 的里程碑標記是「M0–M3」，但 `DESIGN-012`／`013` 的內容屬 M4** | `DESIGN-012` 是「設計打包下載、安裝及驗證說明」（對應 `PACK-006`～`008`）、`DESIGN-013` 是無障礙檢查（與 `QA-009` 逐字重疊） | §3 的標題改為 `M0–M4`，或逐項標里程碑。與差-6 的 DESIGN 處置一起裁定 |
| 差-2 | **`03` §15 有九個 PACK 工作項，`02` §4.6 只有兩個 PACK 需求 ID** | 九對二本身不是問題（M3 的 EVAL 也是十三對四）。問題在 **`PACK-005` 不可判定**：`02:PACK-001` 第 5 條只寫「使用者可選擇是否包含**可散布的** Test Case 與範例資料」，而「可散布」沒有判準——範例 Dataset 的授權從哪來？使用者上傳的資料算不算？丙-12 又指出策展側根本沒有可種入的形式 | **`02:PACK-001` 補一條允收準則**定義「可散布」：只有(a)平台策展產生、且(b)不含使用者上傳位元組的 Test Case 與範例資料可入包；使用者自己上傳的 Dataset **預設不入包**且不提供選項（授權不明，且它是 `PACK-004` 要排除的「不應散布的 Run 資料」的近親） |
| 差-3 | **`03` §15 沒有承接「下載紀錄」** | `02:WS-002` 第 1 條明文要求使用者可查看下載紀錄，承接者是 §8 的 `WS-004`（M1，未勾）——M1 沒有下載，所以它從第一天就不可能完成 | `WS-004` **不搬章節**（搬了 M1 的帳會動），行尾加排期註記「實作排入 M4 第 4／6 批」，同 `TEST-012` 前例 |
| 差-4 | **`BETA-002` 要量漏斗，但沒有任何工作項承接「漏斗事件的產生與儲存」** | `O11Y-001` 量的是平台指標（搜尋延遲、Run 排隊、成功率），不是使用者漏斗；`CORE-008` 的 audit event 是「誰做了什麼」的合規紀錄，語意與保存期限都不同（PDM-006 提案給 audit 400 天、只存 actor／動作／資源 ID／時間戳、**不含內容**）。拿 audit 當分析來源會同時做壞兩件事 | **新增工作項**（建議 `O11Y-004`，掛 §13）：漏斗事件的產生、儲存與查詢。**同時建議立 ADR-029**（§8.2）劃清 audit／分析／Trace 三者的邊界，因為鐵律 11 明文「Secrets 不得出現在……分析事件」，而目前沒有任何文件定義「分析事件」是什麼 |
| 差-5 | **`CORE-008`（重要操作的 Audit Event）未勾，而下載是它最後一個必要事件源** | ADR-007 的稽核清單明列「Artifact 下載及使用者資料刪除」；`SEC-011` 已示範過 audit event 的既有形式（actor 取自 session、與欄位變更同交易） | `CORE-008` 行尾加排期註記「實作排入 M4 第 4 批」，不搬章節 |
| 差-6 | **`03` §3「資訊架構與體驗設計」13 項自 M0 起零勾選** | m3/audit §4.5 6-b 已點名：本專案至今沒有產出過設計交付物，UI 是直接依允收準則實作的；要改變這個狀態需要的是一個**關於「這個專案要不要有設計交付物」的決定**，不是某個里程碑的收尾動作 | 三個選項與建議見 §8.5。**需要拍板**，因為它改的是 `03` 的一整節 |
| 差-7 | **`03` §17 的 `QA-002`（Agent Skills 規格驗證測試資料集）與 M0 的既有產物重疊** | `skillpkg` 已有 45 個 pin commit 套件的實跑驗證、`tools/eval-regression/` 有回歸 harness 的前例 | 不新增需求，`QA-002` 的實作直接以那 45 個套件 ＋ 刻意破壞的變體為資料集；行尾註明來源 |
| 差-8 | **`03` §1 的 PDM-005 追認（乙-9 的殘半）** | `04` 乙-9 的**契約缺口已由 M3 批 1 補上**（`estimated_cost` 已進 `public.yaml`，見 [../m3/audit.md](../m3/audit.md) §4.1 的 1-c）。剩下的是 `02` 與 `m0/pdm-proposals.md` 對「PDM-005 是否已定案」的說法不一致 | 併入 H-7 的逐項定案紀錄一次處理；**本計畫不代為改 `04`**（活文件由負責人或收斂批更新） |

**八項差異的落地結果（2026-08-17 文件批）**：

| # | 已落地 |
| --- | --- |
| 差-1 | `03` §3 標題改為 **`M0–M4`**；`DESIGN-012`／`013` 保留為 M4 實作項並各自寫出範圍（後者與 `QA-009` 明文以**同一份檢查結果**判定，不做兩次） |
| 差-2 | `02:PACK-001` 新增「可散布」的**兩層判準**（第一層 Skill 能不能產出任何 Download Artifact＝`redistribution` 三態 ＋ `access_restriction` 兩道獨立的鎖；第二層哪些 Test Case 與範例資料可入包＝使用者上傳的 Dataset 預設不入包且不提供選項）；`03` `PACK-005` 加引用註記。判準的機制面見 [ADR-027](../../../adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 4 |
| 差-3 | `03` `WS-004` 加排期註記（M4 第 4／6 批），**不搬章節** |
| 差-4 | `02` 新增 **§4.8 `O11Y-004`**（需求 ID ＋允收準則）、`03` §13 新增 `O11Y-004` 工作項；邊界由 [ADR-029](../../../adr/ADR-029-product-analytics-events-and-audit-trace-boundaries.md) 定 |
| 差-5 | `03` `CORE-008` 加排期註記（M4 第 4 批）並寫明「下載紀錄與 audit event 不得合併」；`CORE-007` 一併加註記（與丙-9 的對帳器同批） |
| 差-6 | `03` §3 依 §8.5 選項 (a) 收斂：11 項標「不再追蹤」並逐項指向已落地畫面，**維持 `- [ ]` 不勾**；兩處殘留缺口交棒到 `04` 丙-14 與丙-10；ADR-025 的待決策已就地回填此裁定 |
| 差-7 | `03` `QA-002` 加註記，寫明**可重用的一半**（45 個 pin commit 套件＝合法樣本、`tools/eval-regression/` 的 harness 形式）與**必須新做的一半**（刻意破壞的變體與期望 finding 清單——合法樣本只能證明不誤擋） |
| 差-8 | `04` 乙-9 更新：契約半邊結案，該列縮小為「兩份文件對『是否已定案』的說法不一致」一件事 |

## 8. 未決點

### 8.1 需要負責人追認的 PDM（直接決定 M4 的形狀）

| PDM | 提案在哪 | 不追認的後果 |
| --- | --- | --- |
| **PDM-008** 首批打包 Profile | `m0/pdm-proposals.md` §7（1 標準套件 ＋ `claude-code` ＋ `claude-agent-sdk`；source-available 一律不產出任何 Download Artifact） | 批 3 沒有可實作的目標清單。**提案完整且與 PDM-003 的 Runtime 一致，建議直接追認** |
| **PDM-009** 封測人數、招募方式、成功門檻 | ~~**完全沒有提案**~~ → **2026-08-17 已補**：[pdm-009-beta-proposal.md](pdm-009-beta-proposal.md)（**待追認**）。此前 `m0/pdm-proposals.md` 沒有 PDM-009 這一節，`pdm-proposals` 假設 30 人、`cost-estimation` 用 20 人，兩份文件互斥且都沒有推導 | `BETA-001`／`005` 與 `RELEASE-009` 全部不可判定。~~建議在批 1 之前先補一份提案~~ **提案已在，拍板仍在人**；殘項入列 [`../04` 乙-15](../../04-backlog-and-handoffs.md)。提案要點：12 人（三層各 4）、三條門檻＋不通過決策樹、招募與報酬整套重用 `gate-test/recruit.md`、測試期 14 天、先閘門再封測 |
| **PDM-010** 免費額度與 BYO Key | `m0/pdm-proposals.md` §8（首月 20 次、其後每月 30 次、每日 5 次、並行 2、退還語意）。**其中「首月 20 vs 20+30」提案自己標了「請負責人明確擇一，不要留給實作推斷」** | 批 5 的額度強制點沒有值可寫。**擇一即可** |
| **PDM-006** 保存期限 | `m0/pdm-proposals.md` §6（Download Artifact **90 天**、Run Artifact 30 天、Trace 90 天、audit 400 天、帳號刪除的兩類處理） | `SEC-006` 仍不可判定；下載頁的到期日顯示沒有值 |
| **PDM-004** Runtime 語言與版本 | 實質已定（`2026.08-3` ＋ SDK 0.3.233），缺的是定案紀錄與 `runsc` 上的完整 Run 生命週期實測（＝`SEC-009` T4） | 部署批一併帶掉 |

### 8.2 建議新立的 ADR（**本計畫不代寫，編號自 ADR-027 起**）

> **2026-08-17 更新：三份全部已立且 Accepted**（負責人授權「依最佳實務做決策」）。**下表維持原文**——它記的是「為什麼需要一份 ADR」，那個理由不因 ADR 寫完而改變。
>
> | 編號 | 核心決策一句話 |
> | --- | --- |
> | [ADR-027](../../../adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md) | **兩個雜湊各答一個問題**（`content_hash` 答「是不是這個檔」、規範化的 `manifest_hash` 答「內容一不一樣」）＋zip 寫入規範化使 `content_hash` 在同一個打包器版本內可重現；**MVP 明文不簽章**（下載是登入後的短效授權、沒有驗證端、簽章是一組金鑰生命週期），撤銷只到「停止再發」；可散布性成為 `skills.redistribution` 三態欄位，**預設 `unknown`、只有 `allowed` 放行**，與 `access_restriction` 是兩道獨立的鎖 |
> | [ADR-028](../../../adr/ADR-028-beta-admission-and-quota-enforcement-points.md) | 准入採**允許清單疊在 OAuth 之上**（比照 `SEC-011` 的 `OPERATOR_USER_IDS`，不做邀請碼）；**配額的強制點是平台自己的計數器**，落在建立 Run 的同一個交易與 advisory lock——`max_budget` 管單次金額、`tpm_limit` 管速率、並行上限管同時幾個，三者都不是配額；**顯示必須排在強制之後**（乙-2 的教訓） |
> | [ADR-029](../../../adr/ADR-029-product-analytics-events-and-audit-trace-boundaries.md) | 分析事件是**第五類且明確從屬**的資料、**不是事實來源**；七段漏斗只有四個事件是新的，其餘用既有領域表；形狀採**屬性白名單、不記查詢原文**（鐵律 11 的「分析事件」自此有定義）；與 audit（400 天、不可刪改、具名 actor）和 Trace（Run 之內、要遮罩）三方邊界逐項劃開；存 Postgres 按月分割，**不新增資料產品** |
>
> **兩項刻意不立的 ADR 維持不立**，且已在 ADR-012 的待決策就地註明理由（Profile plugin 機制）。

| 編號 | 主題 | 為什麼需要一份 ADR 而不是一段實作 |
| --- | --- | --- |
| **ADR-027** | **Download Artifact 的形狀、可重現性與完整性** | ADR-012 §「可重現性」要求「相同輸入與版本應能產生語意等價的套件；若內容雜湊因壓縮時間等非語意 metadata 不同，需另有**規範化 Manifest Hash**」——那是一個**沒有被回答的架構問題**（tar 的 mtime／uid／排序都會讓位元組不同）。同一份 ADR 一併回答 ADR-012 待決策 3（簽章、完整性驗證與撤銷）並建議明文寫「MVP 不簽章」及其理由（§2.2） |
| **ADR-028** | **封測准入與配額的強制點** | ADR-020 待決策 2 逐字問的就是「封閉測試（`BETA-001`）是否需要邀請碼／白名單閘門疊在 OAuth 之上」。它連帶決定 PDM-010 額度的**強制點在哪一層**（閘道 Virtual Key 的 `max_budget` 是每 Run 的、不是每帳號每月的——PDM-005 §5.2a 已經有過一次「顯示但不強制」的教訓，乙-2） |
| **ADR-029** | **產品分析事件與 audit／Trace 的邊界** | 差-4。鐵律 11 明文禁止 Secrets 進「分析事件」，但全 repo 沒有任何文件定義分析事件是什麼、存在哪、留多久、誰能讀。ADR-009 只劃了 O11y／Trace／Evaluation 三者。封測的漏斗量測是第一個真實需求，**在寫第一行事件產生程式碼之前需要這條線** |

**刻意不建議立 ADR 的兩項**：①Profile 的設定格式——內建三個目標、版本化設定即可，ADR-012 已經給了框架，再寫一份是為不存在的擴充點背書；②打包管線的執行平面歸屬——打包**不執行任何東西**（只讀套件位元組、只做文字與結構操作），所以它跑在控制平面，與 M3 的評估同一個理由，不需要新決策。

### 8.3 乙類三項與 M4 的關係

| # | 與 M4 的關係 |
| --- | --- |
| **乙-9**（PDM-005 一致性） | 契約面已由 M3 批 1 補完；剩下的併入 H-7。**不阻擋 M4 任何一批** |
| **乙-10**（`anthropic-sa` 法務終判 ＋ 閘門 D 日） | **兩半都阻擋 M4**：法務終判那一半見 §6.2（打包上線會讓詢問信的一句承諾變成可被證偽）；D 日那一半見 §5.3（封測不能與閘門並行）。**這是 M4 最重要的一項乙類** |
| **乙-13**（G7：`artifact` 型引用的引文從未回驗） | **阻擋封測，不阻擋打包**。理由見 M4-5 ②：封測者是第一批會讀到評估報告的非團隊成員，而目前平台分不出「歸錯類」與「編的」。二選一：(a) `artifact` 型引用不得滿足 `evidence_required`；(b) 保留但在儲存與 UI 上標示「此引文未經回驗」。**建議 (a)**（最小改動且與「引文用來證明存在」的原意一致），但**它動的是 ADR-026 defence 3 的判準，所以要拍板** |
| **乙-14（新，本計畫提出）** | **甲類四項是否在封測前到期**（§4）。建議「到期」；替代路徑「只讀不跑的封測」的代價已在 §4 寫清楚。**2026-08-17 已入列 [`../04` 乙-14](../../04-backlog-and-handoffs.md)**，仍待拍板 |
| **乙-15（新，2026-08-17）** | **PDM-009 的封測提案待追認**（§8.1）。提案已產出（[pdm-009-beta-proposal.md](pdm-009-beta-proposal.md)），已入列 [`../04` 乙-15](../../04-backlog-and-handoffs.md)。**追認時要一併過報酬預算與 PDM-010 的首月語意擇一** |

### 8.4 可散布性要不要成為一個資料庫欄位（**M4 最具體的一個新決策**）

盤點的事實：`skills`／`skill_versions` **沒有** `redistributable`、`source_available` 或 `license_status` 欄位。「source-available 一律不產出任何 Download Artifact」這條政策目前只活在三個**資料庫以外**的地方——`tools/content/seed-skills.json` 的策展欄位、`catalog/trust.go` 的註解、以及 `0023_access_restriction.sql` 第 22 行那句「**Download packaging is unaffected here**」（那句話在 M4 之前為真，只因為沒有打包功能）。

**建議**：新增 `skills.redistribution`（`allowed`／`blocked`／`unknown`，預設 `unknown`，只有 `allowed` 放行），與 `access_restriction` 同層、隨 Fork 複製。形狀、判準與回填見 [contract-deltas.md](contract-deltas.md) §4.1、[packaging-design.md](packaging-design.md) §4.5。

~~**要拍板的是兩件事**~~ → **2026-08-17 已由 [ADR-027](../../../adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 4 定案**（負責人授權「依最佳實務做決策」），兩項皆採本節的建議值：①**欄位放 `skills`**（與 `access_restriction` 同層、隨 Fork 複製、可撤銷）——理由同 `0023` 的既有裁定，`skill_versions` 不可變、放不進一個可撤銷的判定；②**預設 `unknown`**（未知即擋）——ADR-021 §5.3 記錄的那個真實誤判錯的正是放行方向，在放行方向上錯不起的欄位，預設值必須是保守的那一個。**兩道鎖都要**（人工 hold ＋內容屬性），且 `license_status = Confirmed` 不得成為放行條件。判準五條與回填方式見該 ADR。<br>**原文保留**：~~①欄位放 `skills`（可撤銷）還是 `skill_versions`（不可變）——建議 `skills`；②預設值是 `unknown` 還是 `allowed`——建議 `unknown`。建議併入 ADR-027。~~

### 8.5 `DESIGN` 系列 13 項零勾選的處置建議

~~**三個選項，建議 (a)，但需要拍板。**~~ → **2026-08-17 已裁定取 (a) 並落地於 `03` §3**（負責人授權「依最佳實務做決策」）。落地形式：`DESIGN-001`～`011` 標「**不再追蹤**」並逐項指向已落地的畫面，**一律維持 `- [ ]` 不勾**（那些設計交付物確實從未產出，勾選是說謊）；`DESIGN-012`／`013` 保留為 M4 實作項並各自寫出範圍；章節里程碑改為 `M0–M4`。**兩處殘留缺口已具名交棒，沒有隨收斂消失**——`DESIGN-007` 的 preflight 版本選擇器 → [`../04` 丙-14](../../04-backlog-and-handoffs.md)；`DESIGN-010`／`011` 承接的 ADR-025 待決策 → 實質已由 `RunEvaluation.tsx`／`RunCompare.tsx` 的兩列狀態回答，剩餘的呈現缺口是 [`../04` 丙-10](../../04-backlog-and-handoffs.md)，**ADR-025 的待決策已就地回填此裁定**。下表原文保留。

| 選項 | 內容 | 評價 |
| --- | --- | --- |
| **(a) 收斂（建議）** | 承認本專案沒有「設計交付物」這個階段，把 §3 的 13 項逐項改為**指向已落地畫面的紀錄**（`DESIGN-002` → `apps/web` 的首頁與搜尋、`DESIGN-010`／`011` → `RunEvaluation.tsx`／`RunCompare.tsx`……），並**保留 `DESIGN-012`／`DESIGN-013` 為 M4 的實作項**——這兩項在 M4 有真正的新畫面（下載頁）與可判定準則（`QA-009` 已寫出無障礙的判準）。ADR-025 待決策（兩列狀態的文案與版面）改由**一個具名的 UI 工作項**承接，而不是掛在一個從未有人做的設計項上 | 誠實：它記錄了真實發生的事（UI 直接依允收準則實作），而不是假裝有一份不存在的設計。代價是承認 `01`／`02` 隱含的「先設計後實作」流程在本專案沒有發生過 |
| (b) 刪除整節 | 直接刪掉 §3 | **不建議**：ADR-025 的待決策與差-1 的兩項會失去承接者，變成無人負責的洞 |
| (c) 補做 13 份設計交付物 | 真的產出 | **不建議**：M0～M3 的 UI 已經照允收準則落地，事後補一份設計文件只會產生第二份真相，而且是沒有人會照著改程式的那一份 |

## 9. 批次分解

比照 M2／M3 的模式：**契約先行 → 實作 → 收斂 → audit**。每批 1～5 個平行 SubAgent，同一批內的 agent **不得碰同一個檔案**（M2 的教訓：共用工作樹平行作業只以 pathspec stage 自己的檔案，**禁止 `git stash`**）。

```text
第 1 批 契約 ──┬─> 第 2 批 打包管線核心 ─┬─> 第 4 批 下載授權與紀錄 ─┐
               ├─> 第 3 批 Profile 與安裝說明 ┘                      ├─> 第 6 批 UI ─┐
               └─> 第 5 批 封測面（配額／漏斗／SEC-012／回歸）───────┘               ├─> 第 7 批 收斂＋audit
                                                                                       │
   〔部署批 H-1～H-3：甲類四項〕──────────────〔封測執行 H-8～H-9〕──────────────────┘
```

| 批 | 內容 | 平行 agent | 依賴 | 前置決策 |
| --- | --- | --- | --- | --- |
| **1 契約與資料模型** | ①`contracts/openapi/public.yaml`：打包與下載的 path 與 schema；②`db/migrations/0027_packaging.sql` ＋ `db/queries/`（**`artifacts` 表已預留 `kind = 'download_package'`，只補側表、下載紀錄與可散布性欄位**，見 [contract-deltas.md](contract-deltas.md) §0、§4）；③`contracts/packaging/`：Download Artifact manifest ＋ Agent Packaging Profile ＋ 可攜 Test Case 三份 JSON Schema | **3**（三組檔案互不重疊） | — | PDM-008（打包目標清單）；ADR-027（含 §8.4 的可散布性欄位歸屬） |
| **2 打包管線核心** | Go `internal/packaging`：匯出建構、`skillpkg.Validate` 重驗（M4-1）、白名單式內容過濾（M4-4）、License 與溯源隨附（M4-2）、規範化 manifest hash、`access_restriction` 阻擋（§6.2） | **3**（建構器／過濾與驗證／溯源與 manifest） | 1 | — |
| **3 Profile 與安裝說明** | 三個打包目標的 Adapter 與版本化設定、安裝說明產生（安裝位置／依賴／環境變數／驗證 Prompt）、未驗證 Agent 的限制揭露、可攜 Test Case 匯出（`PACK-005`） | **2** | 1（可與 2 並行） | PDM-008 |
| **4 下載授權、紀錄與稽核** | 短效下載授權、Workspace scope、下載紀錄（`WS-004`）、下載 audit event（`CORE-008`）、物件存在性對帳器（丙-9）、使用者刪除流程（`CORE-007`） | **2** | 1、2 | PDM-006（保存期限） |
| **5 封測面** | 免費額度強制點與退還（PDM-010）、邀請閘門（ADR-028）、漏斗事件（差-4）、回饋收集入口、`SEC-012`（P1 自動第一動作，與 X-04 共用開關）、注入抵抗合成回歸 ＋ `EVAL-013` B 輪（丙-8） | **3** | 1 | PDM-009／010、ADR-028／029、乙-13 |
| **6 UI** | 下載頁與安裝說明、工作區的下載紀錄、丙-10 的呈現面、`DESIGN-012`（若取 §8.5 (a)）、`DESIGN-013`／`QA-009` 的無障礙 | **2** | 2、3、4 | §8.5 的裁定 |
| **7 收斂＋audit** | `QA-001`～`009`、策展 Test Case 的種入路徑（丙-12）、同意書與保存政策文件（P-12）、殘項回填 `04`、`m4/audit.md` 逐項對帳、`03` 勾選、`RELEASE-001`～`010` 檢查表 | **3** | 全部 ＋部署批 | — |

**第 1 批必須先行的理由是鐵律 12**，不是流程偏好：Download Artifact 的 manifest 同時被 Go（產生）、前端（顯示）與**使用者手上的解壓工具**（驗證）消費，先寫 schema 才不會三邊各長一套。三份 schema 裡有兩份是新的檔案類型（`contracts/packaging/`），這是 `contracts/` 目錄第一次承載非 OpenAPI／非事件的契約——**形狀比照 `contracts/events/` 的既有慣例**（JSON Schema ＋同目錄 README ＋版本演進規則）。

## 10. 風險

| # | 風險 | 對策 |
| --- | --- | --- |
| R1 | **封測日期由部署批決定，而部署批的第一個未知數在第一台節點上**（gVisor `systrap` 是否真的不需巢狀虛擬化——ADR-022 只說「待部署批第一台節點實測確認」） | 把 H-1 的第一步獨立成一個「一台節點、跑通一個 Run」的最小驗證，**排在批 2 完成之前**就開始，不要等打包做完才發現節點跑不起來 |
| R2 | **`PACK-009` 的「依說明安裝」在單人團隊裡沒有第二個環境可驗** | 三個打包目標裡有兩個（`claude-code`／`claude-agent-sdk`）就是開發者手上的工具，本機可驗；`standard` 的驗證形式是「重新匯入平台得到同一個雜湊」（§6.1），**不宣稱在未驗證的 Agent 上可用**（`02:PACK-002` 第 2 條正是為此存在） |
| R3 | **打包會第一次讓平台把內容交出去**，任何排除規則的漏洞都是不可撤回的 | 白名單而非黑名單（M4-4）；`access_restriction` fail-closed（§6.2）；`PACK-004` 的具名測試要**對匯出的位元組**斷言而不是對意圖斷言（同 `TestCostEstimateIsOutsideTheConfirmedHash` 對回應位元組重算 sha256 的前例） |
| R4 | **PDM-009 連提案都沒有**，封測的成功門檻可能在封測跑完之後才定 | 事後定門檻等於沒有門檻。建議把「補一份 PDM-009 提案」排在批 1 之前，且提案要包含「不通過時做什麼」——`gate-test/analysis.md` 的決策樹是現成的形狀 |
| R5 | **漏斗事件是新的資料類別**，一旦開始收集就會有保存期限、遮罩與使用者揭露的義務 | ADR-029 先行（§8.2）；在寫第一行事件產生程式碼之前定義邊界，否則會重演 PDM-005 「顯示但不強制」那一類的洞 |
| R6 | **平行 agent 撞檔** | 第 1 批三組檔案互不重疊已是刻意安排；批 2／3／5 的拆分同理。仍須遵守 pathspec stage 與 `git pull --rebase`，**禁止 `git stash`** |
| R7 | **封測成本**：30 人 × 30 Run/月 ＝ 900 Run，PDM-010 §8.2 估中位 $328／月、打滿 $553／月（模型 ＋ 平台） | 額度是成本上限的執行機制（PDM-010），但它必須**真的強制**；ADR-028 要回答強制點在哪一層 |

## 11. 架構鐵律在 M4 的落點（開工前必讀）

1. **鐵律 1／2**：打包**不執行任何東西**——只讀套件位元組、只做文字與結構操作、不解壓執行、不在 Web／API 程序內跑套件內 Script。打包因此跑在控制平面，**不需要 Sandbox**（同 M3 的評估）。
2. **鐵律 3**：下載端點、下載紀錄、可攜 Test Case 匯出一律 Workspace scope 取自 session；非擁有者一律 404。
3. **鐵律 4**：打包**不建立也不修改** Skill Version——它讀一個既有的不可變版本產出一個新的 Download Artifact。Download Artifact 本身也不可變（重打包＝新的一筆，不覆寫）。
4. **鐵律 9**：下載授權簽發、下載紀錄與 audit event **同交易**；Download Artifact 的清理（到期刪除）冪等、可安全重複。
5. **鐵律 11**：**Secrets 不得出現在套件**——這是 `PACK-004` 的字面要求，也是本里程碑最不可協商的一條。排除規則採白名單，且對匯出位元組有具名測試。
6. **鐵律 12**：第 1 批的存在理由。三份新 schema 先寫，CI 以 codegen 檢查 drift。
7. **ADR-003 的物件存取**：下載用短效授權，**不公開永久物件 URL**；執行平面永遠沒有列舉 bucket 的權限。
8. **ADR-012 的三層相容性**：格式／能力／行為分開呈現，未驗證就寫未驗證。**不得因為「格式驗證通過」而暗示「裝得起來」。**

## 12. 檔案地圖

本目錄依 `AGENTS.md`「里程碑目錄固定骨架」（M3 起適用）：`README.md` ＋ `audit.md` ＋ `report-*`，檔名不重複 `m4` 前綴。

| 檔案 | 類型 | 一句話用途 | 狀態 |
| --- | --- | --- | --- |
| [`README.md`](README.md)（本檔） | 計畫 | M4 的範圍、六條接點與甲類對應、程式／人的分界、批次分解、未決點與風險 | **計畫，未開工** |
| [`packaging-design.md`](packaging-design.md) | 設計 | 打包管線：匯出形狀與匯入的互逆、規格重驗、內容白名單、License 與溯源隨附、可攜 Test Case、下載授權與紀錄 | 已產出，未實作 |
| [`beta-design.md`](beta-design.md) | 設計 | 封測面：准入與邀請、配額與成本上限的強制點、漏斗量測、回饋收集、受測者資料處理 | 已產出，未實作 |
| [`contract-deltas.md`](contract-deltas.md) | 設計 | 第 1 批要先寫的 OpenAPI／JSON Schema 增量清單（只列形狀，不寫 YAML 實體） | 已產出，未實作 |
| [`pdm-009-beta-proposal.md`](pdm-009-beta-proposal.md) | 提案 | 封測的人數、招募方式與成功門檻（含不通過的決策樹、時程與成本）。**放這裡不放 `m0/`**：`m0/pdm-proposals.md` 已隨 M0 完結凍結，補不進去；而 PDM-009 決定的是 M4 的形狀，M4 結束即凍結 | **提案，待負責人追認**（[`../04` 乙-15](../../04-backlog-and-handoffs.md)） |
| [`audit.md`](audit.md) | 審計 | M4 全工作項的逐項對帳（49 項的勾選裁定、三把判定尺、**每一批出入清單的採納／退回／轉殘項**、已勾項覆核、新殘項彙總、對帳自己的限制） | **已產出**（2026-08-18，第 7 批後半） |
| [`report-completed-work-items-audit-2026-08-19.md`](report-completed-work-items-audit-2026-08-19.md) | 報告 | 跨 M0–M4 已勾項覆核；123 個原勾選項中確認 122 個完成，`CONTENT-010` 已退勾 | **已凍結** |
| [`release-checklist.md`](release-checklist.md) | 檢查表 | `RELEASE-001`～`010` 的執行面：程式面已完成 11 項 ＋ 尚缺 12 項、部署期七段（甲類四項／migration 順序／部署設定／策展種入／告警／排程／第一次真實驗證）、負責人 11 項動作、B 日當天 5 項。**勾選狀態的事實來源仍是 `03` §18 與 `04`，本檔只給順序與「誰做什麼驗什麼」** | **已產出**（2026-08-18，第 7 批後半） |
| [`report-platform-ddd-boundary-convergence-2026-08-19.md`](report-platform-ddd-boundary-convergence-2026-08-19.md) | 報告 | Platform DDD 邊界收斂的凍結摘要；現行治理以 ADR-032～035／038／040 為準 | **已凍結** |
| [`report-technical-debt-remediation-2026-08-19.md`](report-technical-debt-remediation-2026-08-19.md) | 報告 | 技術債盤點與修復的凍結摘要；仍開項指向 `04` 與 release checklist | **已凍結** |
| `sec-009-acceptance/<日期>-<節點>/` | 證據 | `SEC-009` 十個測項的判定表與 `versions.txt`（原始輸出留 CI artifact 並附連結），保存 ≥ 1 年 | **尚未產出**（部署批）；路徑由 [ADR-022](../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第三部分指定 |

本目錄外、M4 期間會被修訂而**不屬於本目錄**的東西：

| 位置 | 內容 |
| --- | --- |
| [`../../../../contracts/packaging/profiles/`](../../../../contracts/packaging/profiles/) | 三個打包目標的設定實體（第 3 批前半，見 §13）。**它們同時是 `packaging-profile.schema.json` 的 examples**——schema 因此不再帶 inline example |
| [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) | **殘項的唯一入口，活文件不凍結。** M4 的新殘項與「MVP 之後」的接點都在那裡 |
| [`../../../../tools/qa/skillpkg-corpus/`](../../../../tools/qa/skillpkg-corpus/) ＋ `services/platform/internal/ingest/qa002_corpus_test.go` | `QA-002` 的破壞變體生成腳本、期望 finding 資料檔與對照 harness（第 7 批前半，見 §14.1） |
| [`../../../../tools/content/seed_testcases.py`](../../../../tools/content/seed_testcases.py) ＋ [`seed-testcases/`](../../../../tools/content/seed-testcases/) | 策展 Test Case 的種入路徑與兩個範例 Dataset（丙-12，第 7 批前半，見 §14.2） |
| [`../gate-test/`](../gate-test/) | M1 閘門材料；封測的招募文案、篩選問卷與報酬原則從這裡重用（§6.3）。**§6.3 點名「必須從零做的唯一一項」已於 2026-08-18 補上** → [`consent-and-data-policy.md`](../gate-test/consent-and-data-policy.md)（一份政策兩份同意書，**草稿待法務確認**）。**放那裡不放本目錄**：它由閘門與封測共用、M4 完結後仍會被修訂，依 `AGENTS.md` 的檔案放置規則不屬於任何 `mX/` |
| [`../governance/`](../governance/) | `anthropic-sa` 授權備忘與詢問信草稿；**打包上線會改變詢問信 §3 那句話的真偽**（§6.2） |
| `m0/pdm-proposals.md` | PDM-008／009／010 的追認落在該檔 §9 的定案檢查清單（H-7） |

## 13. 第 3 批（前半）的交付紀錄：Profile 內容與可散布性回填

- 日期：2026-08-17。**不勾選任何工作項**——這一批交的是 Profile 的內容與一支回填腳本，不是 `PACK-006`～`008` 的產生器。
- 交付：[`contracts/packaging/profiles/`](../../../../contracts/packaging/profiles/) 三份、`packaging-profile` 契約與 validator 的對應更正、[`tools/content/backfill-redistribution.sql`](../../../../tools/content/backfill-redistribution.sql)。
- 一句話：三個打包目標第一次有可執行的內容，`skills.redistribution` 第一次有推導得出的值——而其中兩件事的誠實答案是「還沒」。

### 13.1 安裝位置的依據

| 目標 | 位置 | 依據 |
| --- | --- | --- |
| `standard` | **無** | 刻意：不指名 Agent 就不假裝知道你的 Agent 把 Skill 放哪（schema 的 `maxItems: 0` 是這句話的可判定形式） |
| `claude-code` | 使用者層 `~/.claude/skills/<name>/`；專案層 `.claude/skills/<name>/` | Claude Code 官方文件的 Skill 發現位置；另含「往上找到 repo 根」與「session 啟動時不存在的目錄不會被 watch，需重啟」兩條，寫進 `verification_steps` |
| `claude-agent-sdk` | 工作目錄層 `.claude/skills/<name>/`，**相對於傳給 `query()` 的 `cwd`** | 本 repo 實測：`run.mjs` 的 `cwd: workDir` ＋ [`UPGRADES.md`](../../../../infra/images/runtime-agent-sdk/UPGRADES.md) `2026.08-3`（`skill_activation` ＋套件內腳本真的執行），以及 M2 基準 45／45 |

兩個 Profile 的路徑在磁碟上相同，差別在**誰解析它**（Claude Code 自己 watch vs 你的程式傳進去的 `cwd`），逐字寫進兩份的 `known_limitations`。**不確定的一律進 `known_limitations` 而不是路徑表**：Python 套件未實測、plugin 與 claude.ai 同步的 Skill 在別處、省略 `settingSources` 會讓 SDK 連使用者自己的 `~/.claude` 一起載入（平台是在 HOME 為空的 per-run tmpfs 裡量的）。

### 13.2 `support_status`：判的是目標 Agent，不是打包器的產出

`claude-agent-sdk` ＝ **`verified`**（有實測落點）；`claude-code` ＝ **`unverified`**（**沒有人把 Skill Hub 套件放進 `~/.claude/skills/` 跑過**，路徑來自官方文件不等於做過）；`standard` ＝ **`unverified`**（不指名 Agent，格式有效不等於裝得起來）。

**PDM-008 對外的「2 個已驗證安裝 Profile」因此目前只成立 1 個**，差的不是程式而是一次本機安裝 ＋ 落檔（§13.5 出-1）。錯標 verified 是對使用者的承諾，錯標 unverified 只是少賣一句話——`PACK-008` 只有一個安全方向。

### 13.3 ADR-023 的更正（本批發現契約寫反了一次）

[ADR-023](../../../adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md) §2 測項 1 在 SDK 0.3.233 上量到的載入條件是四項：`cwd` 指向放 `.claude/skills/` 的目錄、**`settingSources`／`setting_sources` 省略**（傳 `["project"]` 發現到**零個** skill，與 0.2.137 及官方文件的讀法相反）、`skills: "all"`、工具清單。**四者缺一即零個 skill 且不報錯。**

第 1 批有三處把第 2 項讀成「必須傳 `setting_sources`」：`packaging-profile.schema.json` 的 `snippet` description **與其 inline example**（該 example 逐字寫著 `setting_sources=["project"]`）、`contracts/packaging/README.md` §4、[`packaging-design.md`](packaging-design.md) §6。**三處已就地更正，依據是實測不是推理**（ADR-023 §3）。

落地的 snippet：設 `cwd`、傳 `skills: "all"` 與工具清單、**不傳 `setting_sources` 並在註解寫明為什麼**——那句註解同時滿足 schema 的 lookahead。形式取實測過的 TypeScript；Python 同名選項只在 `known_limitations` 點名為未實測。

同批把 schema 的 inline `examples` **移除**（同一份文件兩份會漂移，而被移除的那份已經漂了），validator 改讀 `profiles/*.json` 並斷言目錄裡恰好是 PDM-008 的三個 id；反例補 4 個（`standard` 寫安裝位置、Profile 自稱 `standard_package`、`claude-agent-sdk` 沒有 snippet、帶 MCP 設定）。**`python tools/contracts/validate_packaging.py`：3 schemas, 9 examples, 22 counterexamples, 0 failure(s)。**

### 13.4 `skills.redistribution` 回填：判準、分佈、與**尚未套用**

腳本 [`backfill-redistribution.sql`](../../../../tools/content/backfill-redistribution.sql) 比照同目錄既有慣例（可重跑、逐列可追溯、資料不進 migration），判準逐條照 [ADR-027](../../../adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 4，且**從 `skill_versions.license_expression` 推導，不從 `seed-skills.json` 抄**——那是判準本來就寫在上面的欄位，也是明天使用者自己上傳的 Skill 唯一會有的東西；而**把 45 列抄進 `VALUES` 正是 `deps` 欄位漏抄過一次的方式**。策展欄位改當人工對照。`license_status = Confirmed` 不在腳本裡（`02:CONTENT-002`）。Copyleft 刻意不在放行清單內（允許再散布，但義務打包器不實作），落在 `unknown` 而被擋。Fork 沿用 `restrict-anthropic-sa-display.sql` 的形狀：root 算判定、Fork 遞迴繼承。

**dry-run（腳本的 CTE 鏈原樣對線上 dev DB 唯讀執行）**：

| 範圍 | `allowed` | `blocked` | `unknown` |
| --- | --- | --- | --- |
| catalog（45 筆） | **41** | **4** | **0** |
| 其 Fork（45 筆，繼承） | **41** | **4** | **0** |

41 筆 `allowed` 的授權運算式是 `MIT`（39）或 `Apache-2.0`（2）。**4 筆 `blocked` 就是 `anthropic-sa` 的 `docx`／`pdf`／`pptx`／`xlsx`**——DB 內的運算式為 `Proprietary. LICENSE.txt has complete terms`（`license_source = manifest`），命中 source-available 樣式。**這四筆不得是 `allowed`，實測結果是 `blocked`。** 它們同時帶著 `access_restriction = 'license-review'`：方向一致，但**兩道鎖獨立**——hold 撤下不會讓 `redistribution` 變 `allowed`。`unknown` 為 0，所以「判不了的清單」是空的（腳本末尾那支 SELECT 仍保留：空結果是目標，不是省掉檢查的理由）。與 `seed-skills.json` 的 `redistributable` 策展欄位**逐筆一致，零分歧**。

**尚未套用。** 執行中的 dev DB 停在 `0026`（`skills` 沒有 `redistribution`，也沒有 `download_artifacts`／`download_records`），本批嘗試套用 `0027` 時被工具層的權限判定擋下，**沒有繞過**：同一工作樹上另有 agent 平行做打包引擎，對共用 dev DB 做 schema 變更該由需要它的那一批決定時機。套用步驟兩行，順序是硬的：

```bash
psql -v ON_ERROR_STOP=1 --single-transaction -f db/migrations/0027_packaging.sql
psql -v ON_ERROR_STOP=1 --single-transaction -f tools/content/backfill-redistribution.sql
```

套用後預期 `UPDATE 90` 與上表的分佈。**數字對不上就不要往下做**——第 2 批的授權閘門是照這個欄位擋的。

### 13.5 出入清單

| # | 事項 | 誰接 |
| --- | --- | --- |
| 出-1 | **PDM-008 的「2 個已驗證 Profile」目前只成立 1 個。** 差一次本機安裝：套件放進 `~/.claude/skills/`、`/skills` 看得到、跑一次驗證 Prompt，落檔後把 `claude-code.json` 的 `support_status` 改 `verified` 並進 `version` 版號 | `PACK-009` 的人工那一段（§5.2、§9） |
| 出-2 | **`0027` 未套用、回填未執行**（§13.4） | 第 2 批（打包引擎要靠這個欄位擋） |
| 出-3 | **schema 的 lookahead 擋不住「真的傳了 `setting_sources`」**，只擋得住「從沒提過」。收緊 pattern ＝收緊值域＝major bump，為一個內建目標不值得；限制已寫進 `contracts/packaging/README.md` §4，不留給下一個人自己發現 | 已記錄，不需動作 |
| 出-4 | **Python 的 `claude-agent-sdk` 未實測**，省略規則沒有被推廣為承諾 | 有需求訊號再走 ADR-023 §2 四項清單 |
| 出-5 | **`redistribution` 沒有寫入端點與 audit**（ADR-027 待決策），今天改它只能直接跑 SQL | `SEC-011` 窮舉清單擴充，M4 首發不做 |

## 14. 第 7 批（前半）的交付紀錄：`QA-002` 語料與策展 Test Case 的種入路徑

- 日期：2026-08-18。**不勾選任何工作項**——`QA-002` 交的是資料集與 harness，`03` 的九項 `QA-001`～`009` 要一起對帳才有意義（第 7 批後半）；丙-12 本來就沒有工作項承接。
- 交付：[`tools/qa/skillpkg-corpus/`](../../../../tools/qa/skillpkg-corpus/)（生成腳本＋期望 finding 資料檔）、`services/platform/internal/ingest/qa002_corpus_test.go`（對照 harness）、[`tools/content/seed_testcases.py`](../../../../tools/content/seed_testcases.py) 與 [`tools/content/seed-testcases/`](../../../../tools/content/seed-testcases/)（兩個範例 Dataset）。
- 一句話：規格驗證第一次有**會被擋的樣本**，策展 Test Case 第一次進得了一個全新部署——而前者查出三個真缺陷，後者證實了 `M4-3`「丙-12 與 `PACK-005` 是同一件事的兩半」。

### 14.1 `QA-002`：21 個破壞變體 × 期望 finding，與查出的三個缺陷

形式照 `03:QA-002` 的重用範圍註記：合法樣本沿用 45 個 pin commit 套件（本批取其中 3 個當基底：`humanizer`／`csv-to-json`／`excel-freeze`，皆 MIT），**新做的是刻意破壞的變體與期望清單**。harness 走的是匯入路徑的同兩支呼叫——`ingest.PackageFS` → `skillpkg.Validate`——不是另寫一套判準。

**入庫的是生成腳本不是 45 份二進位**：一個變體一個函式，讀得出哪裡壞了；committed zip 不透明，而且會對著 `skillpkg` 悄悄腐爛。期望清單是資料檔（`expected-findings.json`），error 與 warning 逐 code **精確比對**，info 只做子集比對——揭露類 finding（外部 URL、依賴清單）隨基底套件內容變動，而阻擋決策不是由它做的。

| 破壞類型（`03:QA-002` 點名） | 變體數 | 期望 finding | 結果 |
| --- | --- | --- | --- |
| 缺 `SKILL.md` | 1 | `skill-md-missing`（error，blocked） | ✅ 相符 |
| frontmatter 壞 | 4 | `frontmatter-missing`／`-unterminated`／`-invalid-yaml`（error）、`-unknown-field`（warning，不阻擋） | ✅ 相符 |
| 必要欄位缺 | 2 | `name-missing`／`description-missing`（error） | ✅ 相符 |
| 名稱／長度違規 | 3 | `name-invalid`／`name-too-long`／`description-too-long`（error） | ✅ 相符 |
| 檔案引用逃逸出套件 | 2 | `file-ref-escapes-package`／`file-ref-missing`（warning） | ✅ 相符 |
| 內嵌可執行程式碼 | 2 | `embedded-script`／`binary-file`（warning） | ✅ 相符 |
| 疑似 Secret | 2 | `possible-secret`（error，blocked，且不回顯命中值） | ✅ 相符 |
| zip 炸彈 | 1 | `PackageFS` 回 `ErrBadArchive`，**沒有 report 也沒有 finding** | ✅ 相符 |
| 路徑逃逸的 entry | 3 | — | ⚠️ **三個都不產生任何 finding，見下** |
| （另加）掃描上限 | 1 | `file-not-scanned`（info） | ✅ 相符 |
| （另加）合法基底對照 | 3 | 0 error；`excel-freeze` 有 1 個 `undeclared-dependency` warning | ✅ 不誤擋 |

**24 個判定全數與期望相符**（3 基底＋21 變體），`golang:1.25` 容器內 `go test ./internal/ingest -run QA002`：`ok 0.140s`。

> **查出的三個缺陷（如實記錄，沒有改期望去遷就）**
>
> | # | 變體 | 觀測 | 為什麼記下來 |
> | --- | --- | --- | --- |
> | D-1 | `zip-path-traversal`（entry 名為 `../../evil.sh`） | **零 finding**，code 集合與乾淨基底逐項相同 | `archive/zip` 的 `fs.FS` 檢視在本 repo 任何一行程式看到它之前就把名字改寫成 `evil.sh`。**沒有 zip-slip**——平台從不把樹寫到磁碟——但那個 entry **無聲地換了身分**，審核者核可的檔案樹不是壓縮檔宣告的那一份 |
> | D-2 | `zip-absolute-path`（entry 名為 `/etc/cron.d/evil`） | **零 finding**，且比 D-1 更安靜 | 改寫成 `etc/cron.d/evil` 之後連副檔名都不是腳本，`script-file` 那一層 info 揭露也不會出現 |
> | D-3 | `zip-symlink-escape`（符號連結指向 `/etc/passwd`） | **零 finding** | zip 的 `fs.FS` 把符號連結當成一個內容為目標路徑的 11 bytes 純文字檔，掃描因此**從頭到尾沒說過這個套件含有連結**。匯入本身安全（不落地），但**任何會把這些位元組寫到磁碟的消費端**都會把連結建出來——`PACK-001` 的下載套件與使用者手上的解壓工具正是兩個這樣的消費端 |
>
> **三者都不在本批修**：D-1／D-2 的修法是在 `fs.Sub` 之前讀原始 `zip.Reader` 的 entry 名，屬 `ingest` 的判斷；D-3 指向的是 `04` M4-4／`PACK-004`（打包排除規則目前沒有一條講符號連結）。**已在 `expected-findings.json` 逐列以 `gap` 欄記著**，而不是寫成 pass——一個期望值被調鬆的語料，下次就沒有人知道它曾經抓到過什麼。

**兩項執行紀律**：①假憑證在執行時才由片段組出來，原始碼裡沒有完整字串，`--selftest` 有一條斷言直接檢查——會被自己的 pre-push 掃描擋下的語料是 commit 不進來的語料；②`zip-bomb` 用的是**真的 260 MiB 零位元組**（deflate 後 282 KB），不是竄改過的檔頭：`PackageFS` 讀的是中央目錄宣告的未壓縮大小，語料若在那裡說謊，等哪天讀法改了它就悄悄停止測試那個上限。

**harness 沒有 `QA002_CORPUS` 就跳過**，形式比照 `SKILLHUB_TEST_DATABASE_URL` 的既有前例（生成要抓三個 pin commit repo 壓縮檔，CI job 沒有那個網路預算）。`go test ./internal/ingest` 在沒有語料時照舊全綠。

**過程中修掉自己的一個錯**：`frontmatter-unterminated` 起初量到的是 `frontmatter-invalid-yaml`——`cutClosingDelimiter` 在**第一個**裸 `---` 就把 frontmatter 收尾，而 29 KiB 的 `SKILL.md` 裡有好幾條水平線。生成器因此先把 body 裡的裸 `---` 行去掉。**名字與行為不一致的語料比沒有語料更糟**，所以修的是生成器不是期望值。

### 14.2 丙-12：策展 Test Case 的種入路徑

`CONTENT-007` 承諾每個精選一組範例 Dataset、User Prompt 與驗收條件，`writing` 五個另加 rubric；這些此前只以文件與「M2 手工建的臨時 Workspace」存在。[`seed_testcases.py`](../../../../tools/content/seed_testcases.py) 就是那條路徑，形式比照同目錄慣例（只走公開 HTTP API、不碰 DB、驗證工具不進 CI）。

**種入的 Workspace 必須是目錄 Workspace，這一條是設計上的硬要求不是偏好**：`internal/packaging` 的 `selectTestCases` 以 `workspaces.is_catalog` 判定「這是不是平台策展的產物」（`PACK-005`），種進個人 Workspace 的 Test Case 會被每一次匯出以 `not_curated` 排除。因此腳本以**目錄策展帳號**登入、只從 `GET /skills` 解析 Skill，**刻意不做「搜尋目錄再 Fork」的後備路徑**——Fork 會落在個人 Workspace，那條後備路徑做出來就是錯的。`is_catalog` 沒有端點，由建目錄時的 SQL 設定，這一點寫在腳本的 docstring 裡。

**沒有任何策展文字被複製第二份**：Prompt 模板取 [`m2/content-baseline-report.md` §3](../m2/content-baseline-report.md)，任務句取 `summaries.json` 該筆自己的第一句 `task_examples`，`writing` 的第 4 條規則取 [`content/writing-rubrics.md`](../content/writing-rubrics.md) §3，rubric 全文**直接讀** `tools/eval-regression/rubric-content-007-writing-v1.json`。一份事實一個位置，第二份一定會漂。

**冪等取「跳過」，不取「更新」**：驗收條件的 id 由伺服器配發，而 rubric item 的 id 就是它所強化的那條驗收條件的 id——原地更新等於去對帳一份識別鍵不歸本工具所有的清單，是一套會半套套用的第二機制。`--replace` 走「刪除再建立」，一次到位到策展狀態。跳過同時保證**策展人事後的編輯不會被重跑蓋掉**。

**新做的只有兩個 Dataset 檔**（`seed-testcases/data.csv`、`draft.md`）：M2 §3 只留下對它們的**描述**（8 列訂單、重複列、混合日期格式、含空白的金額、缺值、國名異形；一段有贅詞與自誇語氣的 Q2 更新草稿），位元組從未留存。本批依該描述重建，**是新的合成資料不是 M2 那兩份**——`02:CONTENT-007` 第 4 條（無 Secrets／憑證／個資）由 `--selftest` 的兩條斷言把關。

**驗證：拋棄式容器，全程沒有碰執行中的 dev DB。** 新開 `pgvector/pgvector:pg17` ＋ 新開一個 `golang:1.25` 的 `cmd/api`（`DEV_LOGIN=1`，物件儲存用既有 SeaweedFS 的另一個 bucket `skillhub-seedtest`），套用 `0001`～`0030` 全部 migration。

| 步驟 | 結果 |
| --- | --- |
| `import_seed.py` 種目錄 | **45/45 imported** |
| `seed_testcases.py --dry-run` | 15 筆 `would_create` |
| `seed_testcases.py` | **15 建立**；DB 實查 **67 條驗收條件**（15×3 基準 ＋ 22 條 rubric 對應條件）、**5 筆帶 rubric**、**22 個 rubric item** |
| rubric item id 是否都指得到真的驗收條件 | **22/22 matched**（逐筆 SQL 比對 `acceptance_criteria` 的 id） |
| Dataset | 每筆 2 個，共 **30 筆**（`data.csv`／`draft.md`，`content_type` 由 magic bytes 判為 `text/plain`） |
| 再跑一次（冪等） | **15 筆 `exists_skipped`**，零寫入 |
| `--replace --only humanizer` | 1 筆 `replaced`；舊列軟刪除保留，live 仍 15 筆 |

**順帶驗掉 `M4-3` 與 §13.4 兩件事**：①同一個拋棄式 DB 上跑 `backfill-redistribution.sql`，分佈為 **41 `allowed`／4 `blocked`／0 `unknown`**，與 §13.4 對線上 dev DB 的 dry-run **逐格相同**——那份 dry-run 因此在一個乾淨部署上獨立複現過一次；②回填後對 `humanizer` 走 `packaging/preview` 與 `POST .../packaging`（target `claude-code`、`include_test_cases=true`），**種入的 Test Case 被列為 `included`、`excluded` 為空**，下載的 artifact 位元組裡確實有 `humanizer/test-cases/content-007-humanizer-<id>/case.json` 與 `data/data.csv`、`data/draft.md`，`case.json` 帶著 `rubric_version = content-007/writing/v1` 與 4 個 item。**丙-12 與 `PACK-005` 的兩半在同一次驗證裡接上了。**

（回填前那次 preview 回的是 `allowed: false` / `license_unknown`——`redistribution` 預設 `unknown` 而 fail-closed，方向正確，一併記在這裡當作 ADR-027 決策 4 的實測。）

### 14.3 出入清單

| # | 事項 | 誰接 |
| --- | --- | --- |
| 出-6 | **`seed_testcases.py` 尚未對執行中的 dev DB 或任何 live 部署執行過。** 阻擋原因與 §13.4 出-2 同源：執行中的 dev DB 停在 `0026`，沒有 `0027`～`0030`，而本工具本身只需要 `0026`（rubric 欄位）——**它其實跑得動**，不跑是因為 ①目錄 Workspace 的策展帳號是誰要先確認（`is_catalog` 由 SQL 設定，dev 上是哪個 user 未查），②同一工作樹上有平行批次，對共用 dev DB 的寫入該由需要它的那一批決定時機。套用步驟：`python tools/content/seed_testcases.py --api http://localhost:8080 --user <目錄策展帳號> --dry-run` 先看 15 筆是否都解析得到 Skill，再拿掉 `--dry-run` | 部署批／第 7 批後半 |
| 出-7 | **三個 archive 層缺陷（D-1／D-2／D-3）沒有承接者。** D-1／D-2 屬 `ingest`（原始 entry 名的揭露），D-3 屬 `PACK-004`／`04` M4-4（打包排除規則沒有一條講符號連結）。**語料已經在庫，修好之後把 `expected-findings.json` 那三列的 `gap` 換成期望 finding 即是回歸測試** | ~~`04` 丙類（本批同時回填該清單）~~ **已於 2026-08-18 同日修畢**：D-1／D-2 為 `entry-path-escape`（error，Blocked，讀原始 zip entry 名），D-3 為 `symlink-entry`（warning，匯出端本就以白名單剝除）；期望清單三列的 `gap` 已換成期望 finding，harness 24/24。逐項理由見 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) 丙-15 |
| 出-8 | **`QA-002` 未勾。** 資料集與 harness 已具備且全綠，但 `03` §17 的九項要一起對帳（第 7 批後半），且本項的允收字面是「建立測試資料集」——資料集成立，勾選仍留給對帳那一步一次處理 | 第 7 批後半 |
| 出-9 | **驗證用的 SeaweedFS bucket `skillhub-seedtest` 留在 dev 物件儲存裡**（容器已刪，bucket 沒有）。無害且與 `skillhub` bucket 隔離，但下次清理 dev 環境時它是可以直接丟的一個 | 不需動作，記著即可 |

## 15. 第 7 批後半的交付紀錄：同意書、勾選對帳與發佈檢查表

- 日期：2026-08-18。**這一批只動 `docs/` 與 `AGENTS.md`，不動任何產品程式碼。**
- 交付：[`../gate-test/consent-and-data-policy.md`](../gate-test/consent-and-data-policy.md)（P-12）、[`audit.md`](audit.md)、[`release-checklist.md`](release-checklist.md)、`03` 的 49 項勾選裁定、`04` 的殘項重整、`AGENTS.md`／`../README.md`／`gate-test/README.md` 的現狀同步。
- 一句話：M4 的程式面在這一批被逐項量了一次——**11 項勾選、38 項誠實不勾**，而不勾的 38 項裡有 17 項的原因是「有人要做一件不是寫程式的事」。

### 15.1 勾選對帳的三個結果

**十一項勾選**：`CORE-008`、`PACK-001`／`002`／`004`／`005`／`006`／`008`、`QA-002`／`003`／`004`／`005`。**三十八項誠實不勾**，分五類（[audit.md §1.2](audit.md)）：缺一小段程式 8、等人拍板 3、等部署或真機 6、等 PDM 追認 4、等真實發生 17。**零項退回**——M4 開工時這 49 項全部是 `- [ ]`，沒有任何一項是「本來勾著、這次改成不勾」。

**三個最值得記住的裁定**：

1. **`PACK-007` 不勾的理由是一句話。** `standard.json` 的 `known_limitations` 對使用者寫著 INSTALL.md 會列出「including the ones Skill Hub's own runtime image refuses」，而產生器從不查那張拒收表。**那句話隨每一個 `standard` 套件出貨**——形狀與 `04` 乙-2 記下的完全相同（讓使用者相信一件平台不執行的事），只是這一次它印在下載給人的檔案裡。
2. **`PACK-003` 與 `QA-007` 一起不勾，理由同源**：沒有任何測試斷言 `LICENSE`／`LICENSE.repo`／`LICENSE.repo.provenance.json` 真的出現在匯出的 zip 裡。白名單會帶走它們**這個推理是對的，但它是推理**，而 §10 R3 自己要求打包的具名測試對匯出的位元組斷言。
3. **`SEC-012` 不勾的不是開關，是觸發。** 開關本體做得很乾淨（P1 與 X-04 共用一張表、每個讀者 fail-closed、解除不是自動的、incident 接管 threshold 且不被降級），**但五條 P1 判準沒有一條會自動翻它**。而這條準則的價值全在「不等人」。

### 15.2 六條接點與五項殘項的處置

「移交 M4」六條**逐項裁定**：M4-1／M4-2／M4-3／M4-4 **關閉**，M4-5 的三件事兩件關閉一件轉乙-13，M4-6 轉乙-14。丙類五項未結案（§3.7）的處置結果：**丙-9 結案**（物件存在性對帳器，`bf86f01`）、**丙-10 結案**（呈現面，`8b7d840`）、**丙-12 結案**（種入路徑，`f9443da`）、丙-8 仍開（B 輪，阻擋原因已具體化為 dev DB 停在 `0026`）、丙-13 仍開（不做，警告維持開著）。

**新增十二項丙與一項乙**（[audit.md §5](audit.md)）。**十二項丙沒有一項在等決策**，補法合計約：一支旅程測試、一支授權檔斷言測試、一支 round-trip 測試、兩個回應欄位、三個列表面、一組漏斗查詢 SQL、一個回饋入口、一次鍵盤走查。

### 15.3 同意書：唯一必須從零做的材料，以及它為什麼有兩份

§6.3 說「必須從零做的只有一項」，本批做了它。**形式是一份政策、兩份同意書**——因為閘門與封測蒐集的東西不一樣（前者是錄影／逐字稿／查詢原文，後者是平台自己產生的紀錄與**會離開平台的模型呼叫**），同意的範圍就不能一樣。

**三件在草稿裡刻意寫死的事**：①**分析事件逐欄公開**（`02:O11Y-004` 明文那是唯一一類使用者未主動提交任何東西就會產生的資料，揭露義務不低於其他類別）；②**「刪除」與「去識別化保留」分開講**，並說明為什麼後者存在（硬刪會弄斷第三方的溯源鏈，`02:DISC-003`）；③**§10 列出草稿刻意沒做的五件事**（不寫成法律措辭、不處理未成年、不寫跨境法遵、不做同意版本化、不把同意狀態寫進資料庫）——**寫得像法律文件但沒有法務看過，比寫得像人話更危險**。

### 15.4 出入清單

| # | 事項 | 誰接 |
| --- | --- | --- |
| 出-10 | **同意書是草稿，§9 有十一項 ⬜ 待填，其中三項要等 PDM-006 追認才有值**（保存期限）。法務確認與簽署是人的動作 | [`../04` 乙-16](../../04-backlog-and-handoffs.md) |
| 出-11 | **本批沒有在本機重跑任何 Go 測試**（Windows 無 Go 工具鏈，整合測試需 DB，`QA-002` 需語料與網路）。**對帳查的是「測試存在、具名、斷言的是對的東西」，不是「今天跑起來是綠的」**；唯一實跑的是無障礙那一支（65 個測試全綠，只需 Node） | [audit.md §6](audit.md) 已記為本次對帳的限制 |
| 出-12 | **`0027`～`0030` 仍未套在執行中的 dev DB 上**，所以打包、下載、配額、漏斗、停派送**五組功能從來沒有在同一個部署上一起跑過**——各批各自在拋棄式容器驗過自己那一段 | [release-checklist.md §2.2](release-checklist.md) |
| 出-13 | **`RELEASE-*` 十項的不勾理由有一部分是引用別項的狀態**（`QA-001` 未完成 ⇒ `RELEASE-002` 不勾）。**這是刻意的結構**——`RELEASE-*` 本來就是別項的彙總，不該有獨立的證據；但如果那些項的判定錯了，這十項會跟著錯 | 記錄，不需動作 |
| 出-14 | **`m4/` 自此凍結。** 之後的變動記在 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md)（活文件）與 `03` 的行內，不回頭改寫本目錄 | 記錄，不需動作 |

# M3：評估與改善 — 執行計畫

- 日期：2026-08-16（計畫）／**2026-08-17（程式面收斂）**
- 狀態：**七批全部完成，程式面已收斂。** 逐工作項對帳見 [audit.md](audit.md)——**16 項：13 勾選、1 誠實不勾（`EVAL-011`）、2 已勾覆核後維持（`EVAL-013`／`CONTENT-007`）**。§7 的四個未決點已由 [ADR-025](../../../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md)／[ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策，§5 的三處差異已在 `02`／`03` 對齊。
- 前提：M2 已完結（[../m2/README.md](../m2/README.md)）；**M1 驗證閘門 D 日仍待負責人宣告**，比照 M2 前例，M3 與閘門並行——**閘門結果沒有改變本計畫的任何技術內容**，R1 的對策（第 1～3 批不碰搜尋與內容、碰內容的 `CONTENT-007` 排最後）照計畫執行完畢。
- 下一個里程碑是 **M4（打包與封測）**；M3 留下的殘項與 M4 接點見 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md)（**活文件**）。
- 上游輸入：[`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) 的**丙類七項接點**（逐項對應見 §3）、[`../m2/m2-work-items-audit.md`](../m2/m2-work-items-audit.md) 的七項誠實不勾（§4）。

## 1. 一句話範圍

M3 讓每一次 Run 得到一份**逐條驗收條件、附可驗證證據、標明判斷來源**的評估，讓使用者據此逐項採納改善建議並**建立新 Skill Version 重跑比較**——而 `succeeded` 從此不再被當成「任務完成」。

## 2. 進與不進

### 2.1 進 M3

| 需求 ID | 承接工作項 | 備註 |
| --- | --- | --- |
| `02:EVAL-001` 結果評估 | `03` EVAL-001／003／004／005／006 | 六類分開呈現、逐條判定與證據、四態整體結果、判斷來源標示、使用者回饋 |
| `02:EVAL-002` 改善建議 | `03` EVAL-002／007／008／009／010 | 問題／證據／位置／修改／影響；四類問題分流；逐項接受並建新版本 |
| `02:EVAL-003` 重新試跑與比較 | `03` EVAL-011／012 | 同一 Test Case 對新版本重跑；比較不得改動歷史 Run |
| `02:TRACE-001` 讀取面 | 無新工作項 | **沿用 `trace.Service`**，不新增 TRACE 工作項（丙-1） |
| `02:CONTENT-007` writing rubric | `03` CONTENT-007（**2026-08-17 已勾**） | rubric 的消費端在 M3 才存在（乙-5，已關閉）；接線與 A 輪回歸見 [report-judge-regression.md §11](report-judge-regression.md)。新缺口 G7 記於 `04` 乙-13 |
| `02:TEST-001` 第 2 條前半、第 3 條 | `03` TEST-012（掛在 §9／M2 章節，**2026-08-17 已勾**） | **借調到 M3**，理由見 §5 差異表。落地後 **`TEST-003` 依同一把尺補回勾選**（`m2` 那份對帳維持凍結不改，兩次變動都記在 `03` §9 就地） |
| — | `03` DESIGN-010／011 | 評估報告、改善建議、重跑與比較的介面設計 |

### 2.2 不進 M3（附理由）

| 不進的東西 | 理由 |
| --- | --- |
| **使用者自訂的可執行檢查腳本** | `03:EVAL-001` 的標題含「可執行」。本計畫把它界定為**平台內建的確定性檢查**（讀平台自己的事實），**不執行使用者提供的程式碼**——那是不受信任內容，執行它必須回到 Sandbox（鐵律 1／2），等於在 M3 內再開一條執行平面路徑。列為後 MVP，需求訊號出現時另立需求 ID |
| **SEC-009／SBX-002／005／007／010** | 甲類部署期驗收，本機結構性不可能（`04` 甲-1～甲-4）。M3 不觸碰，也不因 M3 完成而放寬 |
| **PACK-\*** | M4 範圍。M3 只保證「改善後的新版本可被打包流程取用」，不做打包 |
| **多 Skill 平行 Benchmark** | `01` §7.3 明列不含 |
| **跨模型家族的 Judge A／B** | PDM-003 v5 風險表明文「M3 可加入，不列入 MVP 必要範圍」。M3 只做單一 Judge 層並記錄家族偏誤為已知限制 |
| **`CONTENT-011` 增強重跑** | 負責人已裁定 D 日前不執行（`03` CONTENT-011），凍結標的不動 |
| **保存期限定值（PDM-006）** | 屬乙類待負責人決策。M3 會**踩到**它（見 §6 風險 R4），但不代為定值 |
| **`TRACE-003` 的 MCP 事件** | 遠端 MCP 已移出 MVP 首發；`02:EVAL-002` 要求區分「MCP／工具問題」，M3 的實作只有工具一半，MCP 半邊留型別佔位並誠實顯示為不適用 |

## 3. 丙類接點的逐項對應（`04` §丙）

**七項全部進計畫，一項不漏。**

| 接點 | M3 怎麼接 | 落在哪一批 |
| --- | --- | --- |
| **丙-1** 讀取面直接用 `trace.Service` | 評估器的證據來源**只有** `trace.Service.Advanced`（依序重建、標明 `missing_seq`／`late`／`complete`）與 `General`（聚合摘要）；**不新增任何直接查 `trace_events` 的路徑**。`complete: false` 時逐條判定一律不得記 `passed`，只能 `undetermined` 並註明「證據可能不完整」 | 第 2 批 |
| **丙-2** 寫入面沿用 `RecordOrchestratorEvent` | 評估開始／結束事件沿用它，`seq` 由 `NextTraceSeq` 在同一交易內配號。**不另開寫入路徑**。新增兩個事件型別需同步升 `contracts/events/trace-event.schema.json`（見 [contract-deltas.md](contract-deltas.md) §3） | 第 2 批 |
| **丙-3** 成本合計是下界 | `EVAL-012` 呈現成本時**必須標明是下界**並指出權威來源是閘道 per-key spend（ADR-017）。另：**評估自身的成本與 Run 成本分開兩欄**，不相加為單一數字——一個是使用者工作負載花的，一個是平台判定花的 | 第 5 批 |
| **丙-4** `skill_activation` 的 `skipped` 不可觀測 | `EVAL-002` 判定「Skill 未被啟用」的材料只有「Run 掛了哪些 Skill」對照「trace 出現了哪些 activation」。**不得產出「模型看到了但選擇不用」這類敘述**——那在 SDK 訊息流裡沒有事實依據。措辭上限寫進 Judge 的 prompt 與規則檢查器的文案常數 | 第 2、4 批 |
| **丙-5** `succeeded` ≠ 任務完成 | 本計畫的核心設計決策，見 [evaluation-design.md](evaluation-design.md) §4：`runs.status` 與 `evaluations.overall` 是兩個欄位、兩個表，**評估結果不回寫 `runs.status`**。連帶要改 `internal/run/job.go:419` 那個寫著「evaluation 決定 succeeded vs failed」的 TODO——它與這個決策相反。→ **已立 [ADR-025](../../../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md)（2026-08-17 Accepted）**，程式碼改寫仍在第 2 批 | 第 2 批 |
| **丙-6** 可比較的基準已在庫 | M2 的 45 筆基準 Run（Trace 1112 事件全數 `masked`、Artifact manifest、Test Case 快照皆可重查）作為 **Judge 回歸集的第一組標註資料**——`content-baseline-report.md` 已逐筆判定「符合／未產出」，那正是 Judge 該重現的答案。`EVAL-011／012` 的第一組對照也用它 | 第 3、5 批 |
| **丙-7** `RunResult.usage` 只有牆鐘 | M3 **不改** provider 契約去要 token：成本與 token 走 Trace 已足夠（`TRACE-009` 後每個 Run 都有 `usage` 事件）。若比較畫面後來需要 provider 側 usage，那是 additive 契約變更，屆時另議。本批只在設計文件記錄此界線 | 不動（記錄） |

## 4. M2 七項誠實不勾與 M3 的關係

| 項目 | 是不是 M3 前置 |
| --- | --- |
| `CONTENT-007`（writing rubric 缺消費端） | **是**。rubric 的形狀要在第 1 批的契約定案，內容在第 6／7 批補完並重評 |
| `SBX-002`／`005`／`007`／`010`、`SEC-002`／`SEC-009` | **否**，全為甲類部署期。M3 全程用既有 dev 形態的 Sandbox 跑重測，與部署批不互相阻擋 |

另有兩項 M2 之後才出現的相關項：

- **`TEST-003` 已退回、由 `TEST-012` 承接**（乙-7）：驗收條件的新增／編輯／刪除／確認**沒有任何介面**。EVAL 的整條使用者路徑建立在「使用者定得出驗收條件」之上，所以 `TEST-012` 是 M3 的硬前置（§5）。
- **`TEST-011` 與 `SEC-011` 的契約缺口**：`RunPermissionSummary.estimated_cost` 與 `/admin/skills/{id}/restriction` 尚未進 `public.yaml`。M3 第 1 批既然要動這個檔，**順手補上**（additive，鐵律 12 已欠帳兩筆，見 [contract-deltas.md](contract-deltas.md) §4）。

## 5. 與 `03-work-items.md` 的差異（**2026-08-17：三項全部已對齊**）

原記錄為「本批不動 `03`，三處待負責人或下一批對齊」。**三項已於 2026-08-17 依建議落地**，`02`／`03` 已同步：

| # | 差異 | 說明 | 狀態 |
| --- | --- | --- | --- |
| 差-1 | **`TEST-012` 掛在 `03` §9（M2 章節），但實際必須在 M3 做完** | `03` §9 標題是「Test Case 與執行設定（M2）」。M2 已完結，而 `TEST-012` 是 M2 完結後新增的承接項。它不做完，`EVAL-001` 的「每個驗收條件回傳通過／未通過」在使用者面上沒有輸入端。**建議**：不搬章節（搬了會讓 M2 的帳變動），改在 `03` §9 的 `TEST-012` 行尾補一句「實作排入 M3 第 6 批」 | **已對齊**：`03` §9 `TEST-012` 行尾已加註記，章節未搬 |
| 差-2 | **`03:EVAL-001`「可執行或可判斷的檢查」的解讀** | 本計畫把「可執行」界定為**平台內建的確定性檢查**，明文排除執行使用者提供的檢查腳本（§2.2）。`03` 的一行敘述沒有這個界線，`02:EVAL-001` 的允收準則也沒有要求執行使用者程式碼。**建議**：`02:EVAL-001` 補一條界線準則，`03:EVAL-001` 行尾引用它 | **已對齊**：`02:EVAL-001` 已補界線準則，`03:EVAL-001` 行尾已引用 |
| 差-3 | **`03` §14 沒有承接「評估的重評」與「Judge 回歸集」** | `02:EVAL-001` 要求 LLM Judge 標示為模型評估，但沒有任何工作項要求驗證 Judge 判得準不準。M3 用 M2 的 45 筆基準當回歸集（丙-6），這件事目前**沒有工作項**。**建議**：新增 `03` EVAL-013（Judge 回歸集與判準） | **已對齊**：`02` 新增需求 `EVAL-013`（含允收準則），`03` §14 新增工作項 `EVAL-013`（未勾）。重評語意見 [ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) |

## 6. 批次分解

比照 M2 的模式：**契約先行 → 實作 → 收斂 → audit**。每批 1～5 個平行 SubAgent，同一批內的 agent **不得碰同一個檔案**（M2 的教訓：共用工作樹平行作業只以 pathspec stage 自己的檔案）。

```text
第 1 批 契約          ──┬──> 第 2 批 確定性腿(Go)  ──┬──> 第 4 批 建議與新版本 ──> 第 5 批 重跑與比較 ──┐
                        └──> 第 3 批 Judge 腿(Py)  ──┘                                                  ├──> 第 7 批 收斂+audit
                                                          第 6 批 UI（第 2 批後可開工，第 5 批後收尾）──┘
```

| 批 | 內容 | 平行 agent | 依賴 | 狀態與裁定（2026-08-17） |
| --- | --- | --- | --- | --- |
| **1 契約** | ①`contracts/openapi/public.yaml`：evaluation 讀取面、建議面、比較面、＋補上 `estimated_cost` 與 `/admin/.../restriction` 兩筆欠帳；②`db/migrations/0024_evaluation.sql` 的 DDL 草案＋`db/queries/`；③`contracts/openapi/llm-internal.yaml` 的 `/judge-run`／`/suggest-improvements`＋rubric schema；④`contracts/events/trace-event.schema.json` 升版（評估事件） | **4**（四個檔案互不重疊） | — | **完成**。四份契約全數落地，兩筆鐵律 12 欠帳歸零。一項出入：`0024` 的欄位名與契約不一致，由 `0025` 改名對齊、設計文件就地更正（[audit §4.1](audit.md)） |
| **2 確定性腿** | Go `internal/eval`：migration 落地、規則檢查器（artifact 有無、`skill_activation` 對照、`error` 事件、exit 狀態、格式檢查、延遲與成本門檻）、River job 與狀態機接線（`job.go` 的 `evaluating` 掛鉤與 TODO 改寫）、workspace scope 與整合測試 | **3**（migration＋queries／檢查器／job 與狀態機） | 1 | **完成**。ADR-025 推翻的那條 TODO 已從程式碼消失而非加註解。裁定兩項：只評估 `succeeded`／`failed`；無 Judge 的部署記 `status = failed` 而非不留列（[audit §4.2](audit.md)） |
| **3 Judge 腿** | Python `services/llm`：`POST /judge-run`（strict `json_schema`、截斷政策、rubric 消費、經 LiteLLM）；Go 呼叫端（ctx deadline、失敗回 `undetermined` 不猜、成本歸因標籤）；用 M2 的 45 筆做第一次回歸 | **2**（Python／Go 呼叫端） | 1（可與 2 並行） | **完成**，且第一次回歸就抓到一個真缺陷（v1 的 45 筆全數誤降級）並修掉。另補上契約與 Python 端都缺、而 Go 早就在讀的 `JudgeRunResponse.usage`——一個靜默失效 |
| **4 建議與新版本** | `EVAL-002／007／008／009／010`：Python 產建議與 diff、Go 驗證與套用（套用後重跑 `skillpkg.Validate`，阻擋級即拒）、`evaluation_suggestions` 端點、建新 Skill Version 並記溯源 | **3** | 2、3 | **完成**。溯源方向與設計不同（記在建議側而非版本側），**裁定改設計文字不補欄位**；409／422 兩處回應碼由批 7a 文字化並已核對（[audit §4.3](audit.md)） |
| **5 重跑與比較** | `EVAL-011／012`：同一 Test Case 對新版本重跑（**仍走 preflight 重新確認**，不繞過 TEST-009）、比較讀取面、成本下界標註（丙-3） | **2** | 4 | **完成（讀取面）**。新增契約沒寫的 `inputs_available` 並補進 `public.yaml`；它不探測物件儲存＝樂觀上界，入殘項（`04` 丙-9）。`EVAL-011` 的勾選被批 6 的缺口擋住，見下 |
| **6 UI** | `apps/web`：評估報告一般／進階、逐項接受／拒絕與 diff 預覽、比較畫面；**＋`TEST-012`**（Test Case 與驗收條件介面）；`DESIGN-010／011` | **3** | 2（骨架）、5（收尾） | **部分完成**。評估報告、建議、比較、`TEST-012` 全數落地（`TEST-003` 因此補回勾選）。**兩項未完**：①無 Skill Version 選擇器 → `EVAL-011` 不勾（`04` 丙-11）；②`DESIGN-010／011` 未勾——§3 全 13 項自 M0 起皆未勾，本專案至今沒有設計交付物，這不是 M3 的缺口（[audit §4.5](audit.md)） |
| **7 收斂＋audit** | `CONTENT-007` 的 writing rubric 補完並重評；殘項回填 `04`；`m3/audit.md` 逐項對帳；`03` 勾選 | **2** | 全部 | **完成**。`CONTENT-007` 已勾（兩項保留意見，覆核維持）；A 輪回歸已跑並查出新缺口 G7（`04` 乙-13）；**B 輪未跑**，誠實記為 `EVAL-013` 的待辦（`04` 丙-8） |

**第 1 批必須先行的理由是鐵律 12**，不是流程偏好：`/judge-run` 是 Go↔Python 介面，`0024` 的 `criterion_results` 形狀同時被 Go、Python 與前端消費，先寫 schema 才不會三邊各自長出一套。

## 7. 未決點與新增的 ADR

原記錄為「本批不寫 ADR，決策留給負責人或下一批」。**2026-08-17 負責人授權依最佳實務決策，[ADR-025](../../../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md) 與 [ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 已寫入並 Accepted**（原規劃的 ADR-027 依 U-3 的第一選項併入 026，不另立編號）。

| # | 未決點 | 現況 |
| --- | --- | --- |
| U-1 | 評估結果要不要決定 Run 終態 | **已決策** → [ADR-025](../../../adr/ADR-025-run-terminal-state-and-evaluation-verdict-separation.md)：不決定。`runs.status` 是執行事實、`evaluations.overall` 是任務判定，評估不回寫 `runs.status` 與 `failure_class`。`internal/run/job.go:419` 的 TODO 已被該 ADR 明文推翻，**程式碼改寫在第 2 批** |
| U-2 | 評估可不可以被重做，重做後舊判定去哪 | **已決策** → [ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 1／2：①append-only ＋ partial unique index（當前判定恰好一份，歷史全留）；②證據雙存（引用 ＋ 判定當下的可讀摘要），過期時 `available: false` 並顯示摘要 |
| U-3 | Judge 讀不受信任內容的防線 | **已決策** → [ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 3：四條防線全部為要求（strict `json_schema`／Judge 無能力／Go 逐條回驗證據引用，驗不過降 `undetermined`／內容與指示分隔）。**併入 026，不另立 ADR-027** |
| U-4 | Judge 模型層 | **已決策** → [ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md) 決策 4：追認 PDM-003 v5 §3 的 `gpt-5.6-terra`（中階），三條理由（mini 是試跑預設非 Judge 預設／Judge 品質決定 M3 可信度／與試跑不同型號降自我偏袒）已記入決策；同家族偏誤照抄為已知限制 |
| U-5 | `03` 的三處差異（§5） | **已對齊**（2026-08-17）：`02` 補 `EVAL-001` 界線準則、新增需求 `EVAL-013`；`03` 補 `TEST-012` 排期註記、`EVAL-001` 行尾引用、新增工作項 `EVAL-013` |
| U-6 | 保存期限（PDM-006，乙類） | **仍未定值**（乙類，待負責人）。ADR-026 已讓「證據過期」成為顯示得出來的狀態而不是空白，定值後可能只調整 `excerpt` 長度上限 |

## 8. 風險

| # | 風險 | 對策 |
| --- | --- | --- |
| R1 | **M1 閘門 D 日未宣告**，若閘門不過需先修搜尋與內容 | 比照 M2 前例並行。第 1～3 批不碰搜尋與內容，閘門結果不影響；第 7 批的 `CONTENT-007` 補完會碰內容，排在最後 |
| R2 | **Judge 判得準不準沒有基準** | 用 M2 的 45 筆基準當第一組回歸集（丙-6）。~~目前沒有工作項承接~~ **2026-08-17 已有承接者**：`02:EVAL-013` 需求與 `03` EVAL-013 工作項（差-3 已對齊） |
| R3 | **Prompt Injection**：被評估的 agent 輸出與 artifact 會試圖操縱 Judge | 設計 §2.4 的四條防線；`undetermined` 是安全的預設，不是失敗 |
| R4 | **證據會過期**：Trace 90 天分割表 vs 評估報告要長期可讀 | U-2 ②；設計 §3.4 給了降級呈現的形狀 |
| R5 | **成本**：每次評估都是一次中階模型呼叫 | 截斷政策 ＋ 每次評估的 token 上界；成本與 Run 成本分開列（丙-3）。首發門檻值為預設非實測校準值，須上線後回填（同 O11Y-003 前例） |
| R6 | **平行 agent 撞檔**：第 1 批四個 agent 同時開工 | 四個檔案互不重疊已是刻意安排；第 2、4、6 批的拆分同理。仍須遵守 pathspec stage 與 `pull --rebase`，**禁止 `git stash`** |

## 9. 檔案地圖

本目錄依 `AGENTS.md`「里程碑目錄固定骨架」（M3 起適用）：`README.md`＋`audit.md`＋`report-*`，檔名不重複 `m3` 前綴。

| 檔案 | 類型 | 一句話用途 | 狀態 |
| --- | --- | --- | --- |
| [`README.md`](README.md)（本檔） | 計畫 | M3 的範圍、丙類對應、批次分解與裁定、未決點與風險 | **已收斂並凍結** |
| [`audit.md`](audit.md) | 審計 | 16 項逐項對帳、`EVAL-011` 退回的完整理由、兩項已勾項目的覆核、各批出入裁定、鐵律落點 | **已產出**（第 7 批），凍結 |
| [`evaluation-design.md`](evaluation-design.md) | 設計 | 評估管線：跑在哪個平面、資料模型、Judge 與成本、`succeeded` ≠ 任務完成、建議如何變成新版本 | 已實作；2026-08-17 三處就地更正（§3.2d 欄位名、§5.3 溯源方向、§6.3 成本實測回填），**原文保留不改寫** |
| [`contract-deltas.md`](contract-deltas.md) | 設計 | 第 1 批要先寫的 OpenAPI／JSON Schema 增量清單（只列形狀，不寫 YAML 實體） | 第 1 批完成後凍結；實體以 `contracts/` 下的四份檔案為準 |
| [`report-judge-regression.md`](report-judge-regression.md) | 報告 | `EVAL-013` 的 Judge 判準回歸：v1／v2 兩輪 45 筆逐筆結果與差異歸因、成本實付、以及 §11 的 `CONTENT-007` rubric A 輪 | 凍結；重跑指令見該檔 §10 |

本目錄外、M3 期間產出或修訂而**不屬於本目錄**的東西（依「一份文件如果會被下一個里程碑繼續改，它就不屬於 `mX/`」）：

| 位置 | 內容 |
| --- | --- |
| [`../content/writing-rubrics.md`](../content/writing-rubrics.md) | `CONTENT-007` 的 5 份 writing 預設 rubric（`content-007/writing/v1`）；A 輪之後就地更正 §2.2 的證據路徑遺漏並新增缺口 G7 |
| `tools/eval-regression/` | 回歸 harness `judge_regression.py`、append-only 的 `results.jsonl`、機器可讀的 `rubric-content-007-writing-v1.json` |
| [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) | **殘項的唯一入口，活文件不凍結。** M3 的新殘項與 M4 接點都在那裡 |

## 10. 架構鐵律在 M3 的落點（開工前必讀）

1. **鐵律 1／2**：評估**不執行任何不受信任的東西**——不跑使用者腳本、不解壓執行套件、不在 Web／API 程序內執行 artifact。把不受信任文字餵給模型不是「執行」，但它是注入面，防線見設計 §2.4。評估跑在控制平面，**不需要 Sandbox**，因為沒有東西被執行。
2. **鐵律 3**：`evaluations`／`evaluation_suggestions` 皆帶 `workspace_id`，scope 一律取自 session。
3. **鐵律 4**：採納建議＝建新 Skill Version，**絕不原地覆寫**；歷史 Run 與其快照不因評估而變動。
4. **鐵律 5**：Run 狀態的事實來源仍是 Go 的 Postgres 狀態機；評估是狀態機上的一個步驟，**不是第二個狀態機**。
5. **鐵律 6**：Judge 與建議生成是 Python 的**能力**；判定值域、證據可驗證性、是否採納、能不能建版本，全部是 Go 的政策。
6. **鐵律 7**：評估由 Go Worker（River）驅動，Python 不消費佇列。
7. **鐵律 8**：Judge 呼叫走 LiteLLM，不直連供應商。
8. **鐵律 11**：評估的輸入來自**已遮罩**的 trace（入庫前已遮罩），輸出在顯示前不得引入新的明文。
9. **鐵律 12**：第 1 批的存在理由。

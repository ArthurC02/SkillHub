# EVAL-002：改善提案基線報告

- 日期：**2026-08-23**
- 對應殘項：[`04` 丙-38](../../04-backlog-and-handoffs.md)（`EVAL-002` 的提案品質沒有基線）
- 對應需求：[`02:EVAL-002`](../../02-specifications-and-acceptance-criteria.md)；相關 [ADR-026](../../../adr/ADR-026-evaluation-reassessment-evidence-lifetime-and-judge-trust-boundary.md)、[evaluation-design.md §5.2](evaluation-design.md)
- 語料：M2 的 **123 筆 succeeded Run**（[m2/content-baseline-report.md](../m2/content-baseline-report.md)），工作區 `content-baseline`
- 對照前例：[report-judge-regression.md](report-judge-regression.md)（`EVAL-013`）——格式照它

---

## 1. 一句話結論

**基線跑出來是 0：26 筆提案，26 筆全被丟掉，`stored=0`。** 原因不是模型亂提，是**平台在強制一條沒有任何文件寫過的規則**——`suggestionEvidence` 要求 `evidence` 欄位**整個字串**是某段 excerpt 的子字串，而契約與 prompt 都只說「引自 digest」，於是模型寫的是「兩三段真引文 ＋ 自己的推理」，200～300 個字元，**永遠不可能整段命中**。

修掉之後（`suggest-improvements/v2` ＋ 逐段引文比對）同一批重跑：**53 次評估、37 筆提案、24 筆存下（65%）**。存下來的裡面**每一筆 `ApplyPreview` 都是 `applicable: true`，`blocked_reason` 分布為空**（n=16）。

閘道實付 **$3.4859**（84 次 judge ＋ 67 次 suggest）。

**④「人看了會接受的比率」本報告不填**——那一項要人，且刻意留白而不是用機器可算的東西頂替。

---

## 2. 這次量到什麼、沒量到什麼

| 量到了 | 沒量到 |
| --- | --- |
| 模型提了幾條、平台丟掉幾條、丟掉的理由分布（①②） | 提案內容**對不對**。`applicable: true` 只表示打進 zip 後 `skillpkg.Validate` 過得了，不表示改得比原本好 |
| 存下來的提案有多少比例過得了 `ApplyPreview`（③） | ④ 人的接受率。要人，沒有代理指標 |
| 修正前後的差（0% → 65%），因為兩輪跑的是**同一批 Run** | 跨語料的普適性。全部 123 筆來自同一個工作區、同一次 M2 基線，Skill 全是策展目錄的 |
| 觸發路徑是 `evaluate_run` job → `Service.Evaluate` → `suggest()` → `apps/llm`，也就是產品路徑本身 | **入列**那一段。本次以直接 insert `river_job` 觸發，沒有走 `RunEventConsumer` 的 outbox 路徑——那 123 筆是 M2 跑的，它們的 `run.succeeded` 事件早就發完了 |

---

## 3. 缺陷：`evidence` 這個欄位有兩個互相矛盾的定義

### 3.1 症狀

第一輪 25 筆評估的計數器（`slog.Info("improvement proposals", ...)`，2026-08-23 稍早補上的那行）：

| 指標 | 值 |
| --- | --- |
| `proposed` | 26 |
| `stored` | **0** |
| `dropped_no_evidence` | **26** |
| `dropped_unstorable` / `dropped_write_failed` / `dropped_over_cap` | 0 / 0 / 0 |

**一條都沒存下來，而且全部死在同一個地方。**

### 3.2 為什麼看不出來

那行 warn 只說「沒有可回驗的證據」，沒說**哪裡對不上**——分不出三種情況：模型什麼都沒引、引了但引錯、或 `refs` 本身就是空的。本批把 `quote_runes`／`candidate_refs`／`quote_head` 加進去，一跑就看見了：

```
quote_runes=214 candidate_refs=2 quote_head="\"完整 artifact manifest 明確顯示本次 run 未寫入任何檔案，因此 /out/artifacts/ 中沒有可保存的產出檔案。\" The re"
quote_runes=294 candidate_refs=3 quote_head="\"完整 artifact manifest 明確顯示此 run 沒有寫入任何檔案\"; при этом agent_output утверждает: \"完成"
quote_runes=187 candidate_refs=0 quote_head="\"完整 Artifact manifest 明確為空，表示此 run 沒有實際保存任何 /out/artifacts/ 中的檔案。\" and \"code imp"
```

引號裡面是逐字的真引文。引號外面是模型的推理——中英夾雜，有一筆還冒出俄文。

### 3.3 根因

| 誰 | 對 `evidence` 的定義 |
| --- | --- |
| [`llm-internal.yaml`](../../../../contracts/openapi/llm-internal.yaml) | 「What in the evaluation supports this, quoted from the digest.」 |
| [`evaluate.py`](../../../../apps/llm/src/skillhub_llm/evaluate.py) 的 prompt | 「what in the evaluation digest supports it, quoted from the digest」 |
| [`suggest.go`](../../../../apps/platform/internal/trial/improvement/suggest.go) 的 `suggestionEvidence` | `strings.Contains(ref.Excerpt, quote)`——**整個欄位必須是某段 excerpt 的子字串** |

前兩份說的是「你的說明，裡面要有引文」。第三份執行的是「這個欄位整段必須是引文」。**兩者只要欄位裡有一個字的散文就永遠不會相等**，而失敗是靜默的：使用者看到的是「這次評估沒有提案」，與「模型真的沒話說」長得一模一樣。

這是本季反覆出現的同一個形狀：**同一個事實有好幾份定義，而錯的那幾份沒有跟任何東西對帳。**

### 3.4 修法

1. **Go**（`evidenceQuotes`）：先試整個欄位（維持舊行為對「純引文」的最強比對），再抽出 `「」`／`“”`／`""` 裡的片段，逐段（長的優先）比對。**檢查要守的性質原封不動**——存下來的提案仍然必須有一段**逐字出現在平台自己放進 digest 的 excerpt 裡**的文字；放寬的只有「模型可以在引文旁邊說明理由」，而那正是它被要求做的事。
2. **Prompt 與契約**：把實際要求寫出來——至少一段逐字複製、放進引號、12 個字元以上。`SUGGEST_IMPROVEMENTS_PROMPT_VERSION` 隨之 `v1` → `v2`。
3. **不再為註定失敗的呼叫付錢**：`refs` 為空時整段跳過（`improvement proposals skipped: verdict carries no citable evidence`）。那種情況下無論模型寫什麼都會被丟掉，呼叫是純浪費。**記成一行 INFO 而不是靜默 return**——「判定沒有可引用的證據」是關於這次評估的事實，不是一個空白。

守門測試 `TestAProposalMayExplainItselfAroundTheQuoteItCites`：三種引號各一筆「真引文＋推理」必須存得下來；三種「引號騙不過去」的（引號裡是查無此文、完全沒引號、片段短於 12 字元）必須仍然存不下來。

---

## 4. 修正後的基線

兩輪，同一份程式，`suggest-improvements/v2`：

| 輪次 | Run 母體 | 評估數 | 跳過（無可引用證據） | 實際呼叫 | `proposed` | `stored` | `dropped_no_evidence` |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 第二輪 | 第一輪的同 25 筆 | 25 | 5 | 20 | 21 | 13 | 8 |
| 第三輪 | 另外 28 筆（Skill 未被軟刪除者） | 28 | 12 | 16 | 16 | 11 | 5 |
| **合計** | **53** | **53** | **17（32%）** | **36** | **37** | **24（65%）** | **13（35%）** |

其餘三個丟棄理由（`unstorable`／`write_failed`／`over_cap`）**兩輪都是 0**——路徑逃逸與值域檢查在這批語料上沒有攔到任何東西，不是它們沒作用，是模型沒撞上。

對照第一輪：**26 提案 0 存下**。

### 4.1 那 32% 的跳過是什麼

跳過的都是 judge 給不出可引用證據的評估。**這不是提案的缺陷，是判定的**：一個沒有任何 evidence ref 的 verdict，本來就沒有東西可以拿來支撐提案。它現在有自己的一行紀錄，而不是混在 `dropped_no_evidence` 裡假裝是模型的錯。

---

## 5. ③ `ApplyPreview`：`applicable` 與 `blocked_reason` 分布

對 24 筆存下來的提案逐筆呼叫 `GET /suggestions/{id}/diff`（與 apply 跑同一套檢查，含 `validatePatched`）。

| 結果 | 筆數 |
| --- | --- |
| `applicable: true` | **16** |
| `applicable: false` | **0** |
| 無法預覽（Skill 已軟刪除） | 8 |

**`blocked_reason` 分布為空集合。**

那 8 筆的 Skill 在 dev 庫裡是 `deleted_at IS NOT NULL`，`loadSuggestion` 依規則回 `ErrNotFound`——**這是 dev 資料的狀態，不是產品缺陷**，但它把 ③ 的分母從 24 砍到 16，必須寫出來。（M2 的 123 筆 succeeded Run 裡只有 41 筆的 Skill 還在。）

### 5.1 diff 的形狀（機器看得到的部分）

| 觀察 | 值 |
| --- | --- |
| `target_path` | **16／16 全部是 `SKILL.md`** |
| 檔案大小中位數變化 | 約 +10%（多數提案是改寫與補充，不是刪減） |
| 把檔案砍到原本一半以下的 | **2／16**（最極端一筆 396 行 → 133 行） |

**「16/16 全打 `SKILL.md`」是這一版最該記住的一句**：語料裡的套件有 script、有 reference，而 v2 一次都沒提議改它們。原因大概在 prompt——「Propose content only for a file whose current content you were given」，而 `maxTargetFiles` 是 5，被挑進去的多半就是 `SKILL.md`。**這是分布上的事實，不是本報告要下的結論**；要不要改 `packageFiles` 的挑檔策略是另一件事。

### 5.2 ④ 沒有填

「人看了會接受的比率」需要人逐筆看那 16 個 diff。**不用任何機器指標代替它**——`applicable: true` 只說明它裝得回去。

---

## 6. 成本

| 操作 | prompt 版本 | 呼叫數 | 合計 | 平均 | 平均 in / out tokens |
| --- | --- | --- | --- | --- | --- |
| judge | `judge-run/v2` | 84 | $1.1388 | $0.01356 | 5390 / 535 |
| suggest | `suggest-improvements/v1` | 31 | $1.1138 | $0.03593 | 5985 / 1747 |
| suggest | `suggest-improvements/v2` | 36 | $1.2332 | $0.03426 | 5424 / 1725 |
| **合計** | | **151** | **$3.4859** | | |

**一次提案比一次判定貴 2.5 倍**（output token 是三倍多，因為 `proposed_content` 是整份檔案不是 diff）。所以第 3.4 節第 3 點的「不為註定失敗的呼叫付錢」不是潔癖：那 17 次跳過省下約 **$0.58**，佔本批 suggest 支出的四分之一。

---

## 7. 允收對照（`04` 丙-38 的四個數）

| # | 要量的 | 結果 |
| --- | --- | --- |
| ① | 模型提了幾條 | 修正前 26（25 次評估）；修正後 37（36 次呼叫、53 次評估） |
| ② | 儲存前被丟掉幾條 | 修正前 **26／26（100%）**；修正後 **13／37（35%）**，全部是 `no_evidence` |
| ③ | `ApplyPreview` 的 `applicable:false` 比率與 `blocked_reason` 分布 | **0／16**，分布為空 |
| ④ | 人看了會接受的比率 | **未填，要人** |

丙-38 問的「`suggest-improvements/v1` 是不是也有一個系統性的 v1 缺陷」——**有，而且比 `judge-run/v1` 嚴重**：judge v1 是 45 筆正確判定被降級，suggest v1 是**全部提案無一存活**。

---

## 8. 怎麼重跑

```bash
# 前置：postgres 與 litellm 容器在跑（task dev:model）
# 1. 起 apps/llm
export LLM_SERVICE_TOKEN=<任意值> LITELLM_BASE_URL=http://127.0.0.1:4000 LITELLM_API_KEY=$LITELLM_MASTER_KEY
uv run --directory apps/llm uvicorn skillhub_llm.app:app --host 127.0.0.1 --port 8000

# 2. 起 worker（同一個 LLM_SERVICE_TOKEN；objstore 憑證是 suggest 讀套件用的，缺了會 Access Denied）
export DATABASE_URL=postgres://skillhub:skillhub@127.0.0.1:5432/skillhub
export LLM_SERVICE_URL=http://127.0.0.1:8000 LLM_SERVICE_TOKEN=<同上>
export OBJSTORE_ENDPOINT=127.0.0.1:8333 OBJSTORE_BUCKET=skillhub \
       OBJSTORE_ACCESS_KEY=skillhubdev OBJSTORE_SECRET_KEY=skillhubdevsecret
go -C apps/platform run ./cmd/worker

# 3. 入列（本批以直接 insert 觸發；產品路徑是 run.succeeded 的 outbox 事件）
#    INSERT INTO river_job (kind, queue, args, max_attempts, state)
#    VALUES ('evaluate_run','default', jsonb_build_object('run_id',…,'workspace_id',…), 2, 'available');

# 4. ③ 的預覽要一個 session：DEV_LOGIN=1 起 cmd/api，以該工作區的擁有者登入後
#    對每筆 suggestion 打 GET /suggestions/{id}/diff
```

**注意 dev 庫的 migration 落後**：本批開跑前 `0024`、`0025`、`0026`、`0028`～`0035` 都沒套過（`evaluations` 還是 `0004` 的形狀，`evaluation_suggestions` 根本不存在）。這也是丙-38 一直沒跑起來的實際障礙之一。

---

## 9. 事後更正：這批評估的輸入本身有兩個已知的洞（2026-08-23，同日）

規劃丙-8／丙-13／丙-26 時回頭查證，發現**本報告的 53 筆評估正是 [`04` 丙-13](../../04-backlog-and-handoffs.md) 明文警告過的那件事**。更正寫在這裡而不是改上面的數字，因為量到的東西是真的，**只是它量的不是原本以為的那個母體**。

### 9.1 `artifacts` 全表 0 列，所以每一份判定的產出清單都是空陣列

丙-13 寫著：M2 的管線沒有寫 `artifacts` 列，封存還在物件儲存裡（`run-artifacts/<run_id>/<attempt_id>/artifacts.tar`，**實查 123 個物件都在**）；而**補評時送給 Judge 的 `artifacts[]` 會是空陣列**，Judge 於是在「這個 Run 沒有任何產出清單」的前提下逐條判定。它同時寫著「今天不會發生：沒有路徑會自動評估它們」。

**本批是人手動 insert `river_job` 觸發的，所以它發生了。** 84 筆評估**全部**帶著這條 deterministic finding：

```
the run reported success and no output files were recorded for it.
Whether the task needed a file is for the criteria below to say
```

這也解釋了第 3.2 節那些引文為什麼**每一條都在講 artifact manifest**——模型看到的最顯眼的問題就是它，因為那是我們給它看的東西裡唯一確定為空的。

**對本報告各數字的影響，逐項講清楚：**

| 數字 | 受影響嗎 |
| --- | --- |
| ②的 0%（26 提案 0 存活）與其根因 | **不受影響。** 那是欄位定義不一致，與提案內容無關；空 artifact 只是決定了模型談什麼主題 |
| 修正後的 65% | **不受影響為比率，受影響為代表性。** 分母仍是真的，但這批提案的主題高度集中在同一件事上 |
| ③的 16／16 `applicable` | **不受影響。** `validatePatched` 檢查的是打回 zip 後過不過 `skillpkg.Validate`，與 artifact 無關 |
| 5.1 節「16／16 全打 `SKILL.md`」 | **要保留但降級為觀察。** 提案主題集中，目標檔案跟著集中是預期的 |

### 9.2 一個 2000 字元的截斷，被算成和「整批事件被丟掉」同一件事

84 筆裡 **50 筆**帶著：

```
the material sent for judgement was truncated (trace_digest.entries);
criteria depending on it are undetermined rather than passed
```

而 **26 筆 `undetermined` 判定裡，26 筆都有這一條**——完全相關。

查下去：`buildDigest` 的 `truncated` 是**一個布林**，兩個來源共用——`len(citable) > maxDigestCount`（100 筆，**整個事件被丟掉**）與 `cutExcerpt`（單一 payload 超過 `maxDigestEntry = 2000` 字元被切尾）。本批的 Run 最多 44 個事件，**沒有一筆碰得到 100**，所以那 50 筆全是後者。

後者一路把 `evidenceComplete` 打成 `false`，於是相依的準則只能是 `undetermined`。**一個 payload 被切掉尾巴，與一批事件根本沒送出去，是兩個大小差很多的洞**，目前用同一個旗標與同一個後果表達。

**這一項獨立於丙-13，另立殘項**（見 `04` 丙-47），因為回填 `artifacts` 不會讓它消失。

### 9.3 對重跑的意思

丙-13 的順序規則（**先回填 `artifacts`，再評估**）現在多了一個具體理由：**要拿到一份主題不被空 artifact 綁架的提案基線，必須先回填再重評**。本報告的數字是「在已知輸入缺陷下量到的」，重評後應該回來覆核第 4 節與 5.1 節。

**本報告的 53 筆評估不刪除**（評估是 append-only，ADR-026），重評會成為新的 revision，兩份並存可比——這與 `EVAL-013` 的 v1／v2 兩輪同一個處理方式。

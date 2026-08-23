# Runbook：P1 停止派送（`SEC-010` / `SEC-012`）

- 對應需求：[`02:SEC-010`](../plans/02-specifications-and-acceptance-criteria.md)（「流程以 runbook 形式產出，可被值班人員直接執行」）、`03:SEC-012`
- 對應決策：[ADR-022](../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) X-02／X-04、`02:SEC-010` 的嚴重度分級表
- 殘項：[`04` 丙-26](../plans/04-backlog-and-handoffs.md)

**這份文件的讀者是停派送當下的那個人。** 前三節照順序讀就能動作，背景在後面。

---

## 0. 開關是什麼

一張表、一列。`dispatch_halts` 有一列未 lift 的紀錄，平台就不派送。

- `provider = ''`（空字串）代表**整池**；填 provider 名字代表只停那一個節點池。
- `source = 'p1_incident'`：人或偵測器宣告的 P1。**永遠不會自動解除。**
- `source = 'orphan_threshold'`：ADR-022 X-04 的容量事件，Reconciler 連續 2 輪乾淨會**自己解除**。
- 每個讀者都 fail-closed：有列在 → 建立 Run 直接回 `the execution environment is temporarily unavailable`，已排入的不派送，清理與遺留資源拆除**停手**（保留現場）。

使用者看到的那句話**不含任何事故細節**——開不了 Run 的人不是事故關於的人。

---

## 1. 先確認：現在是不是真的停著

```bash
# 需要 operator session（OPERATOR_USER_IDS 含你的 user_id）
curl -s -b <cookie> http://<api>/admin/dispatch | python -m json.tool
```

回傳的 `dispatching` 是 true／false，每一列有 `target`、`source`、`reason`、`declared_at`、`automatic_recovery`。

**`automatic_recovery` 是這裡最該先看的欄位**：true 代表門檻類、它會自己走；false 代表在等人，也就是等你。

直接查資料庫（API 起不來時）：

```sql
select provider, source, declared_at, lifted_at, left(reason, 200)
from dispatch_halts where lifted_at is null;
```

---

## 2. 分辨真事故與誤觸

五條 P1 判準裡**只有兩條會自己翻開關**，而這兩條都可能誤觸。**先判斷是哪一條，再決定要不要調查。**

### 2.1 `TraceMaskingStopped`

`reason` 長這樣：

```
02:SEC-010 P1 (TraceMaskingStopped): N trace events were stored over the last 2h0m0s
(M of them in the last 1h0m0s) and not one field was redacted in any of them; ...
```

**先跑這一句**，它會告訴你那些事件是誰寫的：

```sql
select source, event_type, count(*),
       sum(case when jsonb_typeof(masked_fields)='array'
                then jsonb_array_length(masked_fields) else 0 end) as masked
from trace_events
where occurred_at > now() - interval '2 hours'
group by 1, 2 order by 3 desc;
```

- **全部是 `source = 'orchestrator'`** → 誤觸的舊形態，**2026-08-23 已修**（偵測器只數 `source = 'sandbox'`）。若在修正之後仍看到這個形態，那是回歸，開 issue。
- **有 `source = 'sandbox'` 的量而 `masked` 全 0** → 這才是判準要抓的形態。**照 [`02:SEC-010` §(3) 遮罩失敗 runbook](../plans/02-specifications-and-acceptance-criteria.md) 走**（撤銷 → 清除 → 通知 → 事後），**不要先解除停派送**。
- **`sandbox` 的量很小（幾十筆以下）** → 見 §5 的量測邊界；判準的前提在低流量下不成立，但**不確定時一律以 P1 處理**（`02:SEC-010` 的升級規則）。

### 2.2 `Reconciler 停擺 > 10 分鐘`

判準來自 `river_job` 裡孤兒掃描的最後生命跡象。最常見的原因是 **worker 根本沒在跑**。

```sql
select max(coalesce(finalized_at, attempted_at)) from river_job where kind = 'run_orphan_scan';
```

worker 停了就把 worker 起回來，確認掃描恢復（時間戳往前走）之後再解除。**掃描沒恢復就解除，等於把看門狗關掉繼續開門。**

### 2.3 另外三條：沒有東西會自動翻開關

| 判準 | 為什麼是人工 |
| --- | --- |
| 逃逸疑慮 | 這是**判斷**，不是量測。沒有一個查詢能回答「這看起來像不像逃逸」 |
| P-02 探針偵測到 Sandbox → 核心資料庫連線 | 探針在**本 process 之外**（節點側），它的訊號不經過控制平面 |
| 隔離技術逃逸類 CVE 揭露 | 來自 `.github/workflows/gvisor-baseline.yml` 開的 issue，也在本 process 之外 |

這三條**由人宣告**：

```bash
curl -s -b <cookie> -X PUT http://<api>/admin/dispatch/halt \
  -H 'Content-Type: application/json' \
  -d '{"note":"<為什麼>"}'          # 省略 provider = 整池
```

`note` 是**必填**——停整池而事後沒人說得出為什麼，比不停更糟。省略 `provider` 停整池，填了就只停那一個池（不認得的名字會被拒絕，不會被當成整池）。

重複宣告是冪等的：它改寫 reason 並再寫一筆稽核事件，因為**同一個人重做一次同一件事不是錯誤**。

`02:SEC-010` 的升級規則在這裡：**不確定是 P1 還是 P2，一律當 P1**。P1 的代價是停止派送（可回復），誤判為 P2 的代價是繼續讓不受信任的工作負載執行（不可回復）。

---

## 3. 解除

```bash
curl -s -b <cookie> -X DELETE http://<api>/admin/dispatch/halt \
  -H 'Content-Type: application/json' \
  -d '{"note":"<做了什麼、為什麼現在安全>"}'
```

`note` 同樣必填。解除**沒有自動路徑**（`03:SEC-012`：自動解除等於讓觸發條件自己決定何時恢復服務）。解除門檻類（`orphan_threshold`）也走同一條——處理完洩漏的人不必再等 Reconciler 跑兩輪。

解除是冪等的：沒有東西可解除時回 204，因為呼叫者要的狀態已經成立。

**解除前的三個檢查**：

1. 觸發它的那個條件現在**還成立嗎**？（重跑 §2 的查詢）
2. 現場保留了嗎？停派送期間清理是停手的，**一旦解除，拆除就會繼續**。
3. issue 開了嗎？`02:SEC-010`：**issue 是事件的唯一事實來源**，聊天記錄與告警訊息都不是。

---

## 4. 實際發生過一次（2026-08-23）

留這段是因為它是唯一一次真的跑過這條路徑，而它是誤觸。

- **症狀**：建立 Run 五次全部回 `the execution environment is temporarily unavailable`。
- **`reason`**：`TraceMaskingStopped`，「兩小時內 274 筆 trace 事件、零遮罩」。
- **實際情形**：那 274 筆**全部是平台自己的 `evaluation_started`／`evaluation_completed`**——由平台從自己的狀態寫出，裡面沒有任何不受信任的內容，**masker 按定義永遠不可能從其中遮掉任何東西**。當天跑了一批重評，就足以讓那個表達式為真。
- **這是「忙碌時的誤判」**，方向與 `for: 1h` 防的「安靜時誤判」相反，而 `for` 看不見它，因為流量是真的。
- **處置**：修偵測器（只數 `source = 'sandbox'`），再以 operator 身分解除。

`04` 丙-26 當時就記過一句：**「在告警裡這只是一個會吵人的誤判，接上停派送之後它會自己把自己停掉。」** 那天它就是這樣做的。

---

## 5. 讀 `TraceMaskingStopped` 時要知道的一件事（量測邊界）

判準的前提是 **「正常流量下 `tool_call` 的 arguments 與 `script_log` 的 message 幾乎必然有東西被遮」**。

**這個前提在目前唯一有資料的語料上不成立。** 2026-08-23 量過：dev 庫 **2,444 筆 `source = 'sandbox'` 的 trace 事件，遮罩總數是 0**。

那批是合成資料、本來就沒有 Secrets，所以這**不代表 masker 壞了**——但它代表：

- **「零遮罩」本身不是強證據**，在低流量或乾淨語料下它是常態。撐起這條判準的是「有量而且持續」，不是「零」。
- 判準真正要抓的是**規則失效**（事件仍被標為 `masked` 而 `masked_fields` 空），`0019` 的 `CHECK (masked)` 讓「未遮罩就入庫」在資料庫層不可能，所以那是唯一剩下的失敗形態。
- **要不要因此調門檻，是 `02:SEC-010` 的決定，不是這份 runbook 的。** 目前的做法是：照升級規則當 P1 處理，走 §2.1 分辨形態。

---

## 6. 這份 runbook 不涵蓋的

- **遮罩失敗的實際處置**（撤銷 Virtual Key、以外洩值重掃、輪替 ingest secret）在 [`02:SEC-010` §(3)](../plans/02-specifications-and-acceptance-criteria.md)。
- **節點 drain 與重建**在 [ADR-022](../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) P-03／X-04。
- **`SEC-012` 仍未勾**：本需求的字面是「**偵測到** P1 判準即立即停止派送」，而五條裡只有兩條是自動的。這份 runbook 讓另外三條可被直接執行，**它不讓那三條變成自動的**。

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
- **三個讀者都 fail-closed，但「有列在」對三者不是同一件事**（**2026-08-25 訂正**：原本這一行寫「有列在 → 建立 Run 直接回錯誤」，那句話只有在**停整池**時成立）：
  - **建立 Run**（`requireDispatchable`）：只有 **`provider = ''`（整池）且 `source = 'p1_incident'`** 的列，才會直接回 `the execution environment is temporarily unavailable`；只停某一個 provider、或門檻類（`orphan_threshold`）的列，**Run 照常建立**。
  - **派送已排入的 Run**（`dispatchPaused`）：整池的列（**不分 source**），或**所有已設定的 provider 各自都有列**時，Run 留在 `queued` 等；只要還有沒被停的 provider，就改派給它。
  - **清理與遺留資源拆除**（`incidentHeld`）：只有 `source = 'p1_incident'` 才**停手**（保留現場）——整池的列擋全部，單一 provider 的列只擋那一個池；`orphan_threshold` **不停清理**，因為清理正是清掉觸發它的那些洩漏。
- ⚠️ **單一 provider 部署（目前的形態）有一個會咬人的組合**：依 §2.3 填了那個唯一的 provider 名字停池 → **建立不被擋**（使用者拿不到那句話）、派送全停 → Run 沉在 `queued`，而 `p1_incident` **沒有自動解除**。**要讓使用者當場知道，就省略 `provider` 停整池。**

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

~~五條 P1 判準裡**只有兩條會自己翻開關**（③ 與 ⑤），而③有兩個偵測器，共三個。~~**2026-08-26 更新：三條會自己翻開關（②③⑤），共四個偵測器。** ② 的探針在該日落地（見 2.3 的補記）。**先判斷是哪一個翻的，再決定要不要調查**——四個裡有兩個可能誤觸（`TraceMaskingStopped` 的流量判準、Reconciler 停擺），兩個不會（masker canary、P-02）。**不會誤觸的那兩個要當成真的**：canary 讀的是規則本身，P-02 讀的是一次真的連上了。

### 2.1 `TraceMaskingStopped`

**這條判準有兩個偵測器，`reason` 的第一行就分得出來是哪一個。** 兩個都掛在 supervisor 每 30 秒那一輪的尾巴，都不會自動解除。

| `reason` 開頭 | 偵測器 | 它憑什麼下結論 |
| --- | --- | --- |
| `TraceMaskingStopped, canary` | **canary**（`trace.MaskerCanary`） | 平台自己造一份**每一種形狀各一個**的假 Secret 餵過 masker，有任何一個沒被遮掉。**不看流量、不看資料庫、不打模型、不建 Run。** |
| `TraceMaskingStopped:` | **流量判準** | 兩小時的 `source = 'sandbox'` 事件（窗的兩端都要有量）而 `masked_fields` 全空 |

**先看是哪一個，因為要做的事不一樣。**

#### 2.1a canary 失敗（`TraceMaskingStopped, canary`）

`reason` 會直接點名哪幾種形狀沒被遮，例如 `openai style key`、`platform-issued value`。

**這不是推論，是直接證據**：那幾個值是平台在那一瞬間自己造的，餵進去、沒遮出來。**沒有「誤觸」這個選項**——masker 是一個對編譯進去的規則做的純函數，不會抖動。

1. 先在同一個 build 上重現（不需要資料庫）：

   ```bash
   go -C apps/platform test ./internal/trial/evidence/ -run TestMaskerCanaryPassesOnAnIntactMasker -count=1 -v
   ```

   **它會紅，而且紅的內容和 `reason` 一致**——測試呼叫的就是生產偵測器呼叫的那一個函數。

2. 看 `apps/platform/internal/trial/evidence/mask.go` 的 `secretPatterns` 與 `redact` 的 Known 那一段：被點名 `platform-issued value` ＝ **精確比對那一臂**壞了（平台自己發出的憑證，例如 ingest token，不再被遮）；被點名某個形狀名 ＝ **對應的那條 pattern** 沒了或被改壞。
3. **在修好並重新部署之前不要解除。** 解除等於讓不受信任的工作負載繼續把輸出寫進 `trace_events`，而遮罩少一條規則。
4. 已經寫進去的東西照 [`02:SEC-010` §(3)](../plans/02-specifications-and-acceptance-criteria.md) 的遮罩失敗程序處理（撤銷 → 清除 → 通知 → 事後）。

#### 2.1b 流量判準（`TraceMaskingStopped:`）

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

### 2.2a `P-02 探針回報 fail`（2026-08-26 新增）

**這一個不會誤觸，所以不要先找誤觸。** 它翻開關的條件是**一個沙箱真的連上了它不該連的位址**，而不是「探針掛了」——探針跑不完是 `unknown`，`unknown` 只讓該節點退出輪替，不停整池。

節點在讀到 `fail` 的當下就已經 `Destroy` 掉自己所有在跑的 Run 並拒收新工作，**所以現場在節點上不在平台上**。

```sql
-- 哪一個節點、什麼時候、碰到了什麼（detail 只有位址與 port，沒有酬載）
select provider, source, reason, declared_at from dispatch_halts
where lifted_at is null and source = 'p1_incident' order by declared_at desc;
```

**解除前要回答的是「那條規則為什麼放行了」，不是「探針還會不會再叫」**——把探針關掉或把節點重開機都會讓它安靜，而兩者都沒有改變放行的那條規則。

**⚠️ 這個探針到今天為止從未在生產節點上跑過**（`03:SEC-008`／甲-3）。第一次在真節點上亮之前，綠燈只證明程式會動，不證明那台機器的網路政策成立。

### 2.3 另外~~三~~**兩**條：沒有東西會自動翻開關

> **2026-08-26：②（P-02）離開這一節，改為自動。** 下表的 ② 那一列原樣保留在下面（劃掉），因為它記的是當時的查證，而那次查證**對的是障礙、錯的是結論**——見該列的補記。**本節現在只剩 ① 與 ④ 需要人宣告。**

| 判準 | 為什麼是人工（2026-08-25 逐條查證，證據在右欄） |
| --- | --- |
| ① 逃逸疑慮 | 這是**判斷**，不是量測。沒有一個查詢能回答「這看起來像不像逃逸」。**「疑慮」不是一個訊號**——真的量得到的那些形態（連上核心資料庫、遺留超標、Reconciler 停擺）各自已經是另外幾條判準；這一條剩下的正是**還沒有形態的那部分**，所以它沒有可接的線，不是漏接 |
| ~~② P-02 探針偵測到 Sandbox → 核心資料庫連線~~ **已自動（2026-08-26）** | ~~**那個探針今天不存在**（`03:SBX-005` 未勾、[ADR-050](../adr/ADR-050-beta-runs-in-parallel-with-the-sandbox-acceptance.md) 明寫「P-02 常駐探針不存在」；`tools/sec009/` 只有 T1／T2／T8 三支，沒有 T10）。**就算它存在，訊號也沒有路徑進來**：沙箱契約是**單向的**——控制平面呼叫節點（`apps/sandbox/internal/sandbox/http.go` 六條路由全是被呼叫端），節點沒有任何往控制平面推的通道，唯一的反向入口 `POST /internal/trace/{token}` 是**單一 Run 範圍且由不受信任的沙箱送出**。要接就得改 `contracts/` 的 capability 契約，那是決策不是接線~~<br>**上面那段的第一層是對的（探針當時確實不存在），第二層的結論錯了。** 「節點沒有往控制平面推的通道」是真的，**但不需要推**——控制平面本來就在輪詢 `GET /capability`。所以做法不是開一條反向通道（那會把 DoS 手把交給不受信任工作負載，該顧慮成立），而是讓節點在既有的被呼叫端多報一個欄位：`ProviderCapability.security.p02_probe` 帶它自己的最後一次讀數。**契約仍然單向，推的仍然是控制平面。**<br>**現在的行為**：節點自己起一個與 Run 同組態的一次性容器去撥號，讀到 `fail` 就地 `Destroy` 所有在跑的 Run 並拒新工作；平台端 `detectP02Breach` 掛在既有 sweep 的尾巴，冪等、不自動 lift。**三個邊界要記在值班的地方**：①**節點聯絡不上不等於 fail**（那是節點健康問題，翻成 P1 會讓一次網路抖動停掉整池）；②**`unknown` 不停整池**，只讓該節點退出輪替——剛開機還沒讀數的節點不該停掉全池；③**節點自己恢復不會自動解除**，解除仍只有 operator 一條路。<br>**⚠️ 值班時要知道的一件事**：這個探針量的是**那台節點的網路政策**，而它**從未在生產節點上跑過**（`03:SEC-008`／甲-3）。第一次在真節點上亮之前，它的讀數只證明程式會動 |
| ④ 隔離技術逃逸類 CVE 揭露 | 訊號來自 `.github/workflows/gvisor-baseline.yml`，而 **CI 看得到的東西到不了生產 DB**。該 workflow 自己把理由寫在檔案裡：要自己翻開關就得讓 CI 持有一把能停整池的平台憑證，**「a credential that can stop the fleet, held by CI, is a worse exposure than the minutes a person takes to paste one command」**。反方向（平台自己去拉 GitHub advisory feed）要為控制平面開對外網路出口，並讓派送能力取決於一個外部 feed 的可達性。兩者都是**部署期決策**。<br>順帶查證兩件事：`runsc` **不在任何 image 裡**（它是節點主機的執行檔，repo 內唯一的版本釘點是 `infra/nodes/gvisor-baseline.txt`，目前 `unset`），所以 `runtime-image.yml` 的 `rescan` 掃的是 Agent SDK image，**掃不到 gVisor**；`ProviderCapability` 也沒有回報 `runsc` 版本的欄位 |

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

**流量判準**的前提是 **「正常流量下 `tool_call` 的 arguments 與 `script_log` 的 message 幾乎必然有東西被遮」**。

**這個前提在目前唯一有資料的語料上不成立。** 2026-08-23 量過：dev 庫 **2,444 筆 `source = 'sandbox'` 的 trace 事件，遮罩總數是 0**。

那批是合成資料、本來就沒有 Secrets，所以這**不代表 masker 壞了**——但它代表：

- **「零遮罩」本身不是強證據**，在低流量或乾淨語料下它是常態。撐起這條判準的是「有量而且持續」，不是「零」。
- 判準真正要抓的是**規則失效**（事件仍被標為 `masked` 而 `masked_fields` 空），`0019` 的 `CHECK (masked)` 讓「未遮罩就入庫」在資料庫層不可能，所以那是唯一剩下的失敗形態。
- **要不要因此調門檻，是 `02:SEC-010` 的決定，不是這份 runbook 的。** 目前的做法是：照升級規則當 P1 處理，走 §2.1b 分辨形態。

**2026-08-25：這個量測邊界不再是「等封測真實流量才知道」。** canary（§2.1a）從另一個方向問同一個問題，而且**在零流量、合成流量、真實流量下讀數一樣**：直接把一份平台自己造的假 Secret 餵過 masker，遮不掉就是壞了。上面那一整段仍然成立，但它現在只約束流量判準這一半。

**兩個偵測器都留著，因為它們壞在不同的地方，而且誰都看不見對方的失敗**：

| | canary | 流量判準 |
| --- | --- | --- |
| 它讀的是 | **規則**（本 process，無流量） | **路徑**（生產，真實事件） |
| 抓得到 | pattern 被刪改、Known 精確比對那一臂壞掉 | 有人把 ingest 路徑上那一步 mask **整個拿掉**（呼叫端有兩個，各自 `new` 自己的 Masker） |
| 抓不到 | 呼叫端根本沒呼叫 masker——canary 自己呼叫，所以它永遠是綠的 | 任何規則失效，只要語料裡本來就沒有可遮的東西（＝目前的處境） |
| 在合成語料上 | 有效 | **無效** |

刪掉流量判準會拿一個「目前量不到但唯一看得見接線的偵測器」換成「那一半沒有偵測器」。

---

## 6. 這份 runbook 不涵蓋的

- **遮罩失敗的實際處置**（撤銷 Virtual Key、以外洩值重掃、輪替 ingest secret）在 [`02:SEC-010` §(3)](../plans/02-specifications-and-acceptance-criteria.md)。
- **節點 drain 與重建**在 [ADR-022](../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) P-03／X-04。
- **`SEC-012` 仍未勾**：本需求的字面是「**偵測到** P1 判準即立即停止派送」，而五條裡只有兩條是自動的（③⑤）。這份 runbook 讓另外三條可被直接執行，**它不讓那三條變成自動的**。
- **2026-08-25 補**：③ 多了一個 canary 偵測器（§2.1a），但那是**同一條判準的第二個讀法**，不是第三條判準變自動。①②④ 一格都沒動——理由見 §2.3，三者的訊號都在本 process 之外。

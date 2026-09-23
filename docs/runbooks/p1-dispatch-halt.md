# Runbook：P1 停止派送

**讀者是正在處理派送停止的人。** 先讀 §1 確認目前狀態，再依 §2 的來源處理。這個開關只控制 Run：它不會取代節點關機、秘密輪替、Trace 清除或映像重建。

## 1. 先讀目前狀態

```bash
# 需要 operator session（OPERATOR_USER_IDS 含你的 user_id）
curl -s -b <cookie> http://<api>/admin/dispatch | python -m json.tool
```

回應有 `dispatching` 與每個未解除 halt 的 `target`、`source`、`reason`、`declared_at`、`automatic_recovery`。API 無法使用時，直接查核心資料庫：

```sql
select provider, source, reason, declared_at, lifted_at, clear_rounds
from dispatch_halts
where lifted_at is null
order by declared_at;
```

`provider = ''` 代表整個 pool；其他值代表一個 Provider。不要只看 `dispatching`：它只說現在是否所有 Provider 都被擋，沒有說事故範圍。

| `source` | 意義 | 是否自動解除 |
| --- | --- | --- |
| `p1_incident` | 安全事故或偵測到 P1 條件 | 否；只能由 operator 在修復與保留證據後解除 |
| `orphan_threshold` | 遺留 Sandbox 達容量門檻的保護 | 是；連續兩輪乾淨的 Reconciler 會解除，operator 也可在確認已處理後手動解除 |

## 2. 停止的是什麼

三個行為各自 fail-closed，不能互相推論：

| 動作 | 哪些 halt 會擋住 |
| --- | --- |
| 建立新的 Run | 整池 `p1_incident`，或每一個已設定 Provider 都各有 `p1_incident`；使用者只會得到暫時無法執行的訊息 |
| 派送已排入的 Run | 整池的任何 halt，或每一個已設定 Provider 都各有 halt；若仍有未停的 Provider，Worker 可改派給它 |
| 清理與拆除遺留資源 | `p1_incident` 才停手以保留現場；整池 halt 擋所有清理，單一 Provider halt 只擋該 Provider；`orphan_threshold` 不擋清理 |

單一 Provider 部署時，停該 Provider 與停整池都會使建立與派送停止；仍要選正確範圍，因為清理的影響不同。

## 3. 來源導向的處置

先讀 `reason`，不要先解除或重啟。下表的「立即動作」是為了止血；修復與證據完成前，P1 一律維持 halt。

| `reason` 的訊號 | 判定 | 立即動作 | 解除前證據 |
| --- | --- | --- | --- |
| `TraceMaskingStopped, canary` | masker 對平台合成 Secret 已失效；不是流量推論 | 停止調查前的任何解除，輪替可能外洩的憑證並依秘密處置程序清除受影響 Trace | 同一 build 的 canary 恢復、規則修正已部署、受影響資料與憑證已處理 |
| `TraceMaskingStopped:` | Sandbox Trace 持續有流量但沒有遮罩欄位 | 先確認事件來源與遮罩結果；若 Sandbox 資料確實零遮罩，照秘密處置程序止血與清除 | 重新查詢顯示問題已排除；若只是合成或無 Secret 語料，仍要留下判定證據再解除 |
| `P-02` | 節點常駐探針從 Sandbox 網路位置連上禁止位址 | 保留節點現場；節點已自行拒收新工作，調查 egress 規則與 pinned 位址 | 節點網路政策已修正、節點重新准入，且探針讀數為乾淨；`unknown` 不是 breach，但不得把它當作通過 |
| `orphan reconciler` | 遺留資源掃描超過容許間隔未執行 | 恢復 Worker 與掃描；不要在掃描仍停擺時解除 | `run_orphan_scan` 的最近執行時間持續前進 |
| 人工宣告的逃逸疑慮或隔離技術高風險 CVE | 外部安全訊號，平台無法自行判定 | 宣告整池 halt、保留現場，依節點換新與漏洞處置程序處理 | 受影響節點已 drain／重建，或風險已由負責人明確排除 |
| `orphan_threshold` | 容量保護，不等於已證實安全事故 | 讓 Reconciler 清理；必要時先將該節點從 Provider 清單移除 | 遺留資源低於門檻並連續兩輪乾淨，或已完成有證據的人工清理 |

### 3.1 Trace 遮罩的最小判讀

流量型訊號只統計 `source = 'sandbox'`。先確認它不是控制平面自己寫出的事件：

```sql
select source, event_type, count(*) as events,
       sum(case when jsonb_typeof(masked_fields) = 'array'
                then jsonb_array_length(masked_fields) else 0 end) as masked
from trace_events
where occurred_at > now() - interval '2 hours'
group by 1, 2
order by events desc;
```

canary 與流量型訊號保護不同邊界：canary 檢查 masker 規則本身，即使零流量也有效；流量型訊號檢查 ingest 呼叫端是否仍然使用 masker。合成或沒有 Secret 的語料可使流量型訊號不具判讀力，不能用來推翻 canary。

### 3.2 Reconciler 的最小判讀

```sql
select max(coalesce(finalized_at, attempted_at))
from river_job
where kind = 'run_orphan_scan';
```

時間戳不再前進通常代表 Worker 或排程無法執行。先修 Worker、佇列或資料庫，確認新的掃描完成，再討論解除。

## 4. 人工宣告與解除

只有「逃逸疑慮」與「隔離技術高風險 CVE」需要由人主動宣告；canary、流量型遮罩、P-02、Reconciler 停擺會由平台建立整池 `p1_incident`。不確定是 P1 或較低等級時，按 P1 處理。

```bash
# 宣告整池 P1；填 provider 才只停止該 Provider
curl -s -b <cookie> -X PUT http://<api>/admin/dispatch/halt \
  -H 'Content-Type: application/json' \
  -d '{"note":"<觸發條件與已採取的止血動作>"}'

# 修復、驗證與證據完成後解除；同樣可填 provider
curl -s -b <cookie> -X DELETE http://<api>/admin/dispatch/halt \
  -H 'Content-Type: application/json' \
  -d '{"note":"<修復內容、驗證結果與為何現在安全>"}'
```

兩個操作都要求非空 `note`，並寫入 audit event。重複宣告與解除是冪等的。解除前必須同時確認：觸發條件已消失、現場與處置證據已保存、負責人可追溯事件的記錄已建立。對 `p1_incident` 而言，節點自己恢復、重啟程序或探針暫時安靜都不是解除理由。

## 5. 事故結束後

解除只恢復派送，不會補做被保留的清理。確認 Worker 已重新接走 queued Run、`cleanup_status` 回到正常收斂，並依需要重建受影響 Sandbox 節點。若事故涉及 Trace 或憑證，另外確認秘密已輪替、歷史資料已依處置程序清除或遮罩；這些工作不能用一個綠色健康檢查取代。

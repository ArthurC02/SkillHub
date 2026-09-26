# ADR-023：帳號清除

- 狀態：Accepted
- 相關：[ADR-003｜Run 編排與非同步工作流程](./ADR-003-run-orchestration-and-async-workflows.md)、[ADR-006｜身分、Workspace、准入與額度](./ADR-006-identity-workspace-admission-and-allowances.md)、[ADR-010｜產品分析與稽核邊界](./ADR-010-product-analytics-and-audit-boundaries.md)、[ADR-016｜Platform Bounded Context 與 Context Map](./ADR-016-platform-bounded-contexts-and-context-map.md)、[ADR-017｜Query 與寫入所有權](./ADR-017-query-and-write-ownership.md)、[ADR-025｜Credit 計量與扣款](./ADR-025-credit-metering-and-charging.md)

## 背景

帳號清除（`03:CORE-007`、`02:NFR-002`）必須是不可逆的終點：清除之後不能再冒出屬於這個帳號的私人資料列。但 Session 驗證只能擋住新請求，擋不住已經通過驗證、正在寫資料，或仍持有短效上傳授權的既有工作；會建立私人資料根列的路徑分散在多個 Bounded Context（Run、Skill 匯入、其他資料 producer），任何一條在清除盤點之後才落地都會讓「已刪除」又長回資料。

## 決策

### 決策 1：帳號清除以 Workspace 寫入 fence 序列化，逐一 Bounded Context 補上物件盤點

所有會建立 Workspace 私人資料根列的交易，先以 `workspace-objects:<workspace_id>` 取得 shared advisory lock，取鎖後重查該 Workspace owner 尚未進入 purge；跨越 Object Store I/O 的路徑（Dataset、Skill 套件上傳）改持 session-scoped shared lock。資料庫 trigger 是共同強制點。

帳號清除先取得該帳號名下全部 Workspace 的 session-scoped exclusive lock；只有在 Run context 證明所有 Run 已終態且完成 sandbox cleanup 之後，才寫入清除起點、盤點物件並執行清除——尚未安靜時是延後，不是失敗，仍保留可重試的 lease。清除起點寫入之前仍可取消；之後既有 Session 失效，OAuth／DEV login 不得把原身分誤認為新帳號，而「已進入清除等待」的帳號在被主動查詢時只算已認領工作，不提前宣告不可逆階段已開始。

Run Artifact 的清除清單除了既有的 manifest 列，也由每個 Run Attempt 推導其固定 archive key——sandbox 可能已經上傳，但 manifest 的 best-effort 寫入可能還沒成功。為了讓清除知道一個 Attempt 是否曾經核發過下載授權，Attempt 建立時明寫「未核發」，把已核發的授權交付給 Provider 前改成「已核發」；只有「未核發」狀態可以被自動判定為安全並關閉，狀態不明的既有列一律視為「未知」，不得用經過的時間或核發上限反推它安全。

Skill package 在寫入 Object Store 前先寫 durable 的收集意圖，Uploader 與 collector 對同一個 content-addressed key 共用另一把 advisory lock，並在鎖內重查是否仍有 Version 引用；成功 Version 的意圖可安全丟棄，失敗或中斷留下的 bytes 可被重試清除。

固定鎖序是「Workspace 清除 fence → package／domain 鎖 → 資料列寫入」；清除流程只取得 Workspace exclusive fence，不反向取得 producer 的 domain 鎖。

Object Store 不具交易性——package 收集意圖與 Run Attempt 授權狀態都是「先記可重試工作、再寫 bytes／發授權」的同一種紀律，collector 與清除流程永遠先重查引用或狀態，再刪除。

### 決策 2：新寫入不變量上線前必須先排空可能違反它的舊版程序

決策 1 的保護橫跨資料庫與 Object Store，不能把「migration 已執行」當成「所有程序都已遵守新協定」。當某個保護（例如 Run Attempt 的核發狀態欄位）是新加入、舊版程序不會遵守時，上線前必須先停止所有可能違反該不變量的舊版 Maintenance、Worker 與可能產生物件的舊版 API 程序，新舊版本不得滾動混部，直到排空完成才可重新啟用該保護的強制路徑。

排空之後，等待期須涵蓋已核發授權的最長存活期，再加上額外寬限與時鐘飄移，而不是任意選一個看起來夠久的數字；在目前的授權存活期配置下，這個等待期是 15 分鐘（授權存活上限）＋5 分鐘（寬限）＋1 分鐘（時鐘飄移）＝21 分鐘。等待期滿之後，依限定條件把舊版留下的「未知」狀態列一次性修復為可判定狀態；在完成排空與修復之前，清除流程對這些列 fail-closed 是正確結果——即使 migration 曾經替舊列補寫一個有限期限，也不能只憑期限推定舊版程序沒有在稍後才核發授權。新的寫入不變量只保護新寫入之後才產生的物件；重新啟用帳號清除之前，還必須先完成一次舊版程序可能留下的 Object Store 孤兒物件盤點與對帳，證明沒有未配對的物件殘留。

### 決策 3：帳號清除不觸碰任何 Credit 紀錄，且不設保存期限

Credit 帳戶、扣點分錄、成本事件與會話摘要在帳號清除後原樣保留，永久不清——帳號清除流程不包含刪除 Credit 紀錄這一步，也不存在任何依時間視窗刪除這些表的排程或路徑。這些表適用不可變表的紀律，任何新增的刪除查詢都會被查詢所有權檢查擋下（ADR-017）。

使用者那一列在清除後不刪、只標記刪除時間，所以 Credit 紀錄裡的使用者與 Workspace 參照在清除後仍然有效。[ADR-025](./ADR-025-credit-metering-and-charging.md) 的決策 3 已經要求成本事件不存查詢原文，這些紀錄只有金額、token 數、模型名稱與時間，不含使用者輸入的文字，清除之後留存這些紀錄不構成額外的隱私風險。

### 決策 4：清除預設跑在 API 的資料庫角色上，最小權限角色備而不用

最小權限的清除角色、逐表授權、`SKILLHUB_PURGE_DATABASE_URL` 與切換 runbook 都在 repo 裡；**部署不設這個變數，`maintenance` 退回 API 角色並在啟動日誌說出來，這是刻意的**。在一位營運者、十幾位受測者的規模，拆角色的縱深買不到相稱的風險降低，而切過去還要把會做 DDL 的分割表輪替拆成另一次呼叫。那套角色是為「第二位營運者或第一位外部維運者加入」準備的——那時憑證會離開單一個人，清除路徑才值得擁有自己的一組。

## 影響

### 正面

- 帳號清除可以是真正的終點:清除起點寫入之後，分散在多個 Bounded Context 的私人資料 producer 都會在同一把 Workspace fence 之下被擋下或延後，不會有資料在清除盤點之後才長出來。

### 成本與限制

- 清除流程新增的資料庫 trigger 是刻意的共用最後防線，各 Bounded Context 仍擁有自己的查詢與清除語意，沒有新增跨 Context 直接查表的例外。
- 新協定上線時，若不確實排空舊版程序，清除或扣點會出現與新舊版本假設不一致的競態；完成排空與修復之前，清除寧可延後也不冒進。
- Credit 紀錄永久保留是一個明確選擇，不是預設遺漏——它排除了未來以「保存期限到期」為由清理這些表的路徑。

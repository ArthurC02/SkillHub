# ADR-062：帳號清除必須等待進行中的私人資料寫入

- 狀態：Accepted
- 日期：2026-09-03
- 關聯：ADR-003、ADR-008、ADR-011、ADR-034；`02:CORE-007`、`02:NFR-002`

## 背景

HTTP Session 驗證只能阻止新的請求，不能撤回已通過驗證、正在寫資料或仍持有短效上傳權限的工作。原本帳號清除只與 Dataset、下載套件共用 Workspace advisory lock；Run Artifact、Skill 匯入與其他資料列可能在清除盤點之後才出現，使「已刪除」之後又長回私人資料。

## 決策

1. 所有會建立 Workspace 私人資料根列的交易，以 `workspace-objects:<workspace_id>` 取得 shared advisory lock，並在取鎖後重查 Workspace owner 尚未進入 purge。資料庫 trigger 是共同強制點；Dataset 與 Skill 套件上傳因跨越 Object Store I/O，改持 session-scoped shared lock。
2. 帳號清除先取得該帳號全部 Workspace 的 session-scoped exclusive lock。只有在 Run context 證明所有 Run 已終態且 `cleanup_status = cleaned` 後，才寫入 `purge_started_at`、盤點物件並執行清除。尚未安靜時是延後，不是失敗；`purge_attempted_at` 仍提供重試 lease。
3. `purge_started_at` 之前仍可取消；之後既有 Session 失效，OAuth／DEV login 不得把原 identity 誤認為新帳號。`ListAccountsPastGrace` 只 claim 工作，不再提前宣告不可逆階段已開始。
4. Run Artifact 的清除清單除了 manifest rows，也由每個 Run Attempt 推導其固定 archive key。原因是 sandbox 可能已上傳、但 manifest 的 best-effort 寫入尚未成功。
5. Skill package 在 `Put` 前先寫 durable `object_collection_queue` intent，Uploader 與 collector 對 content-addressed key 共用另一把 advisory lock，並在鎖內重查是否仍有 Version 引用；成功 Version 的 intent 可安全丟棄，失敗或 crash 的 bytes 可被重試清除。

固定鎖序是「Workspace purge fence → package／domain lock → row write」。清除流程只取得 Workspace exclusive fence，不反向取得 producer 的 domain lock。

## 結果

- 一般未刪除帳號的功能與資料形狀不變；差別只出現在清除競態與失敗回收。
- 清除可能因仍在執行或仍未完成 sandbox cleanup 而延後一輪，避免先刪後長回。
- 新增資料庫 trigger 是刻意的共用最後防線；各 Bounded Context 仍擁有自己的 query 與清除語意，沒有新增跨 Context 直接查表的例外。
- Object Store 不具交易性，因此 package intent 仍採「先記可重試工作、再寫 bytes」；collector 永遠先重查引用再刪除。

## 驗證

- Producer 先持 shared lock 時，purge 等它 commit，之後看得到並刪除該列。
- Purge 先持 exclusive lock時，producer 解鎖後重查 eligibility 並拒絕，零列落地。
- active Run 與 terminal-but-not-cleaned Run 都延後；cleanup 完成後重試可刪除沒有 manifest row 的 attempt archive。
- 拿掉 Session predicate、Run readiness、資料庫 fence 或 package collection intent，對應聚焦測試皆會失敗。

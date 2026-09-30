# Draft PR: 以 owner facts 建立可續作的 Workspace Activity

## Business intent

讓回到 Skill Hub 的創作者在一個誠實的平台工作台辨認需要留意、仍在進行與最近完成的工作，並回到精確來源續作。Workspace Activity 是跨來源續作投影，不是通知收件匣、稽核紀錄或另一套生命週期。

Garrett 五層由下而上約束實作：Strategy 是回來後知道下一步；Scope 是四種 item 與五個 owner readers；Structure 是 owner facts 與唯讀 composition；Skeleton 是三組工作狀態、全域續讀、來源完整性與單一續作動作；Surface 沿用既有設計系統。低層事實未全部成立前，UI 保留「試跑活動」名稱與範圍揭露。

## Domain impact

本提案影響 Identity、Run、Evaluation、Creation、Packaging 與 Publishing，並新增一個不持久化、沒有 Aggregate 或生命週期的 Workspace Activity query Context。Identity 只解析已驗證 Session 的 Workspace；五個 owner 分別透過版本化 internal fact-reader contract 提供自己擁有的分類、權威時間、穩定來源識別與 continuation identifiers。Activity 只擁有來源 coverage、完整性、全域 keyset、游標與 transport mapping，不重新解讀 raw status。

四種 item 是 Run、Creation Session、Packaging Artifact 與 Skill Publication。Run item 同時使用 Run lifecycle fact 與 Evaluation verdict fact，兩者先由各自 owner 分類，再以 `needs_attention > in_progress > recent > neutral` 合併。Bundle Publication 是後續獨立 item scope，不以缺少管理入口為排除理由；Catalog exposure、散布可得性、Artifact clock-derived expiry 與 cleanup 進度皆不冒充首版 Activity 事件。

## Implementation handoff

Domain facts：Run 終態不可再轉移；Evaluation 擁有 task verdict；Creation 透過窄介面保持反向依賴；Packaging 掃描與可得性是兩軸；Publication、Release、散布可得性、Bundle 與 Catalog exposure 是不同事實。

Forces：各來源狀態與時間語意不同，既有清單使用 offset、固定上限或名稱排序；瀏覽器無法建立單一授權邊界、全域游標或來源完整性。Decision：新增 query-only Activity Context，但不新增 Aggregate、事件流或持久投影；只新增目前資料無法證明的 Run `activity_updated_at`。Packaging 的標準流程在同一交易建立 Artifact 並完成 available，因此使用 `created_at`；quarantined 與 rejected 只作防禦性 needs_attention，不虛構一般 in_progress。五個 owner 暴露版本化 Workspace-scoped facts readers；Activity 同步 fail-closed fan-in，機械合併 Run 與 Evaluation，再做全域 keyset。這是唯讀 query boundary，不是新的狀態機。

Open decisions：本 revision 已把 owner、分類、時間、來源識別、續作、首版範圍，以及 Activity Context 與五條 reader contracts/interactions 明確提出供 developer review；尚無未具名設計題，但核准前仍不是實作依據。

拒絕瀏覽器 fan-out、entrypoint raw-status switch、跨 Context SQL、讓 Run 解讀 Evaluation verdict、analytics 或 Trace 真相來源、持久 Activity Context、用 `created_at` 冒充所有 recency、把 clock-derived expiry 冒充持久事件，以及把 Bundle 或 Catalog exposure 靜默塞進首版。

Counterfactual 指向 `source-failure-visible`：移除任一 owner failure 的 fail-closed 保護後，逐來源故障測試必須變紅，再恢復並確認原檔一致。

## Proposal and approvals

Proposal `workspace-activity-owner-contract-v3` 是 material draft，需要 developer approval。它修正 v2 未納入 Evaluation owner、未完成分類與時間決策，以及以過時 Bundle 理由界定範圍的問題。v3 驗證並送審後，才以 lifecycle command 將 v2 標記為由本案取代；送審不等於 verified、approved 或 applied。

## Contract impact

提議新增 Session-authenticated、Workspace-scoped 的 `GET /me/activity`。回應包含穩定 item envelope、owner-produced classification、typed continuation、全域 keyset cursor 與五個 reader 的來源完整性。任一 contracted reader 失敗時回 `503`、`complete=false` 與 `unavailable_sources`，不回 items 或 cursor。這是加法式 contract；既有來源清單與 `/workspace/runs` 維持相容。

回應只含 Workspace metadata，不含 Creation Snapshot 或 prompt、Trace、Evaluation verdict text 或 findings、套件內容、下載歷史或曝光審查內容。未知 kind 保留穩定摘要與明確不支援狀態；client 不從 raw status 推斷分類或動作。

## Verification

本 package 目前只有來源查證與結構驗證，所有 obligations 均為 planned。後續證據必須涵蓋雙 Workspace 隔離、Run 與 Evaluation 的完整 status／overall 組合與時間決策表、取消接受與拒絕、Creation 狀態與 pending action、Packaging 的 available／防禦性狀態／expiry／purge／object loss／delete、Publishing status／release／無 Release fallback／excluded axes、明確 total order 的等時戳與頁界、逐來源故障、OpenAPI codegen 與相容性、四種 item kind、五個 reader coverage、四種 typed continuation、未知 kind fallback、Registry reader boundaries、Run 終態與 Creation inversion architecture checks。

## Residual risks

Run 的新時間欄位、Workspace Activity Context 與五條 internal reader contracts/interactions 仍待 developer approval；Activity Context、interaction mechanism 與 Publishing owner 的部分 Registry evidence 也須在 apply 前由開發者確認納入 source corpus。五個 readers、公開 contract、後端投影、UI 與所有 proof obligations 尚未實作。Packaging 目前依賴標準流程原子完成的 `created_at`；若日後引入可觀察的非同步掃描，必須以 successor proposal 重審分類與時間。本提案刻意不把 Bundle、Catalog exposure、散布可得性、Artifact expiry、Trace 或 cleanup 擴張成首版 Activity 來源。

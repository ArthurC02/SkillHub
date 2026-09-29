# Draft PR: 以 owner contract 建立完整 Workspace Activity

## Business intent

讓回到 Skill Hub 的創作者從一個誠實的平台空間判斷哪些長工作需要決定、仍在執行或最近完成，並回到精確來源續作。Garrett 五層在此由低到高約束實作：Strategy 是回來後知道下一步；Scope 是 Run、Creation Session、Packaging Artifact 與附最新 Release 的 Skill Publication；Structure 是 owner facts 與唯讀 composition；Skeleton 是分組、全域續讀與單一續作動作；Surface 沿用既有設計系統。低層事實未成立前，UI 保留「試跑活動」名稱。

## Domain impact

本提案影響 Identity、Run、Creation、Packaging 與 Publishing，但不新增 Activity owner。Identity 只解析已驗證 Session 的 Workspace。四個生命週期 Context 分別擁有狀態、分類、權威時間、穩定來源 ID 與續作目標；HTTP composition 只合併 owner facts、套用全域游標、揭露來源完整性並轉成 transport。

舊提案已被實作證據超越：Artifact 與 Skill Publication 已有精確 management deep link；標準 Packaging 流程也在同一交易建立並將 Artifact 標為 available，因此 `quarantined` 不能被描述成一般可觀察的長工作。Bundle 的現有 search 選的是成員 Version，不是既存 Bundle，故不納入首版。Catalog exposure 是 operator 擁有的獨立事實，也不納入 Publication recency。

## Implementation handoff

Domain facts：Run 終態不可再轉移；Creation 透過窄介面保持生命週期 Context 反向依賴；Packaging 掃描與可得性是兩軸；Publication、Release 與 Catalog exposure 是不同事實。Forces：四種來源時間與狀態語意不同，既有清單沒有全域 keyset，也不能在瀏覽器證明完整性。Decision：不新增 Aggregate、事件流或持久 Activity Context；每個 owner 提供最小 Workspace-scoped reader，composition 做同步唯讀 fan-in、全域 keyset 與 fail-closed coverage。這個投影不需要新的 tactical domain pattern。

Unknowns：Run authoritative last update 的構成、Packaging 狀態時間，以及四個 owner 的完整分類表與唯一續作動作仍需 developer review。Publishing 提議以 Publication 狀態時間和最新 Release 時間的較晚者作 recency，但在核准前不是實作依據。

拒絕瀏覽器 fan-out、entrypoint status switch、跨 Context SQL、analytics 真相來源、持久 Activity Context，以及把 Bundle 或 Catalog exposure 塞進首版。Counterfactual 指向 `source-failure-visible`：移除任一 owner failure 的 fail-closed 保護後，故障注入測試必須變紅，再恢復並確認原檔一致。

## Proposal and approvals

Proposal `workspace-activity-owner-contract-v2` 是 material draft，需要 developer approval。它取代尚未核准的 `workspace-activity-owner-facts` 設計方向，但不改寫舊 package；舊案應在本案建立並送審後透過 lifecycle command 留下 `superseded_by`、原因與原 submitted 狀態。送審不等於 verified、approved 或 applied。

## Contract impact

提議新增 Session-authenticated、Workspace-scoped 的 `GET /me/activity`。回應包含 discriminated item union、typed continuation union、全域 keyset cursor 與來源完整性；任一 contracted owner 失敗時回 `503`、`complete=false` 與 `unavailable_sources`，不回 items 或 cursor。這是加法式 contract；既有來源清單與 `/workspace/runs` 維持相容。

回應只含 Workspace metadata，不含 Creation Snapshot 或 prompt、Trace payload、評估全文、套件內容或曝光審查內容。未知 kind 仍可透過穩定摘要顯示；client 不從 raw status 推斷 actionability。

## Verification

本 package 目前只有來源查證與結構驗證，所有 obligations 均為 planned。後續證據必須涵蓋雙 Workspace 隔離、逐 owner 狀態決策表、等時戳與頁界、逐來源故障、OpenAPI codegen 與相容性、首版來源 discriminator、四種 typed continuation、UI coverage gate、Run 終態與 Creation inversion architecture checks。

## Residual risks

Run 與 Packaging 的權威時間仍未定案；Publishing recency 尚待 owner review；四個 owner 的完整分類仍未知。這些問題會阻擋 public contract 實作與完整 Activity 命名，但不阻擋現有「試跑活動」或其他只讀 owner-scoped 首頁續作切片。

# Draft PR: 以 owner facts 建立 Workspace Activity 投影

## Business intent

讓離開後再回來的創作者，從一個誠實的平台 Activity 看見哪些長工作需要決定、仍在執行或最近完成，並回到精確來源物件續作。這個切片以 Jesse James Garrett 的五層模型由下而上推導：Strategy 是「回來後知道下一步」，Scope 是 Run、Creation、Packaging、Publishing 四種長工作及明確排除的完整內容，Structure 是 owner facts 與 API composition，Skeleton 是分組、全域排序和單一續作動作，Surface 沿用既有設計系統。五層不是畫面裝飾清單；較低層的來源事實尚未成立時，較高層不得用完整 Activity 的外觀掩飾缺口。

## Domain impact

本提案影響 Identity、Run、Creation、Packaging 與 Publishing，但不新增單一 Activity owner。Identity 只負責從已驗證 Session 解析 Workspace，不擁有 Activity 或來源生命週期事實；四個生命週期 Context 仍分別擁有狀態、可行動分類、下一步與權威時間。HTTP composition layer 透過 `apiserver.NewApp` 注入的窄 reader 同步取回 facts，只做 Workspace-scoped fan-in、穩定 keyset 合併、來源完整性與 transport mapping，不成為新的領域 owner。

核心不變量是：組裝層可以合併，但不能重新解讀 owner facts。禁止瀏覽器扇出、跨 Context SQL、共享 generated row、生命週期 Context 互相 import、用 analytics 事件冒充真相，以及把來源失敗靜默轉成空資料。

## Implementation handoff

最小設計不新增 Aggregate、事件流或持久化 Activity Context。每個 owner 提供只含 Activity 必需資料的 read DTO：穩定來源身分、使用者可辨識摘要、原始狀態、owner 決定的分類、可辯護的 `last_updated_at` 與 typed continuation target。`GET /me/activity` 的不透明游標依 `(group_rank ASC, last_updated_at DESC, kind ASC, source_id ASC)` 形成全域順序，對各來源讀取游標之後的候選，再合併成單一頁；這是即時 feed，資料集不變時續讀不重複不遺漏，不宣稱跨請求快照。

待 owner 審查的分類候選如下：Run 的非終態是執行中，`failed`／`timed_out` 需要使用者看證據，`succeeded`／`cancelled` 是最近完成；Creation 的 `queued`／`working` 是執行中，等待輸入、等待確認、草稿或候選就緒、失敗與需重傳都需要使用者，`saved`／`cancelled` 是最近完成；Packaging 的 `quarantined` 是執行中、`rejected` 與保存期限內意外遺失需要使用者、可下載的 `available` 是最近完成；Publishing 第一版不把 operator 的 Catalog 審核工作冒充創作者待辦。這些分類未獲 owner 審查前不是實作依據。

拒絕新增持久 Activity Context，因為目前只有讀取投影需求，沒有第二套狀態、事件重放或最終一致性的證據。拒絕由 Identity 或 entrypoint 推導 owner 狀態，因為那會把生命週期規則搬離 reviewed owner。拒絕瀏覽器組合與單一跨 Context SQL，因為兩者都無法同時守住授權、全域分頁、故障揭露與依賴邊界。

部分失敗採 fail closed：任一已啟用 owner 讀取失敗就回 `503`、`complete=false` 與 `unavailable_sources`，不回 items 或 cursor，不能讓不完整結果看起來像完整空清單。實作前仍需逐一確認四個 owner 的分類表、續作入口與權威時間，特別是 Run 和 Packaging 的時間缺口。

Counterfactual 指向 `source-failure-visible`：實作後暫時移除一個 owner reader 的失敗揭露，對應故障注入測試必須變紅，再恢復原保護並確認檔案一致。

## Proposal and approvals

Proposal `workspace-activity-owner-facts` 已送交 human review；因為同時改變跨 Context collaboration 與 public contract，需要 developer approval。尚未 verify、approve 或 apply，也沒有 Registry update；送審不代表邊界已核准。

## Contract impact

新增 Session-authenticated、Workspace-scoped 的 `GET /me/activity`，包含 discriminated item union、全域 keyset cursor 與來源完整性。任一已啟用 owner 讀取失敗時回 `503`、`complete=false` 與 `unavailable_sources`，不回 items 或 cursor，避免部分頁被誤認成完整資料。這是 additive change；既有 `GET /runs` 與 Run-only UI 維持相容，直到四種來源全部有 owner-backed facts、權威時間與續作入口，才可移除「目前只收錄試跑」的範圍揭露。

回應只包含 Workspace metadata，不包含 Creation Snapshot、Prompt、Trace payload、評估全文、套件內容或曝光審查 findings。未知 kind 和 owner status 仍須能透過穩定摘要呈現；client 不得從 raw status 自行推斷 actionability。

## Verification

目前只有來源查證與 Change Package 結構驗證，沒有產品程式或 public contract 實作，因此所有 test obligations 都維持 `planned`。後續證據必須涵蓋雙 Workspace 隔離、每個 owner 的狀態決策表、等時戳與分頁邊界、逐來源故障注入、OpenAPI codegen／相容性、四來源 UI gate，以及 depguard／query ownership／architecture identity。

## Residual risks

Run 尚無通用列級 `updated_at`，且取消要求與清理是否納入最後更新仍待 owner 審查；Packaging 掃描狀態缺少可直接使用的狀態變更時間；Creation、Packaging、Publishing 的現有清單無法直接提供統一 keyset。Publication 已有 Workspace 跨 Skill 清單與最新 Release 的有效 Catalog 曝光狀態，但 Publishing item 粒度、Bundle 範圍及 Packaging／Publishing 精確深鏈仍未裁定。在這些缺口關閉前，現行 Run-only Activity 是誠實切片，不是完整平台活動中心。

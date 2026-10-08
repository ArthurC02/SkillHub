# Draft PR: 內容來源白名單與公開收錄准入

## Business intent

讓 operator 能對原始 Skill 來源作准入、否決與下架決定，公開目錄只收錄有有效准入決定的來源；個人匯入與公開策展的政策不混用。

## Domain impact

Ingest 擁有候選和准入決定；Catalog 的一般搜尋文件也服務個人 Skill，不能直接把索引寫入當成公開收錄；目前公開搜尋同時收錄策展 Workspace 與 Publishing 核准曝光的 Release，兩條閘門需分別審查。Registry 已擁有 Skill 下架事實，不能另建第二套可見性。既有 Skill Version 和 Run 不改寫。

## Implementation handoff

候選不能僅以 repo URL 識別：現有 `seed-skills.json` 的 `anthropic` 與 `anthropic-sa` 共享 repo，但依 Skill 目錄有不同授權。產品已裁定以原始 repo URL 與 Skill 路徑作穩定鍵，commit 作每次審查證據。先建立唯一可查詢的決定現況與歷史，再於策展 Workspace 和經裁定屬外部來源收錄的 Publishing 曝光入口接上 Ingest 判定；不能阻斷個人匯入的一般搜尋文件寫入。來源撤銷只阻止新收錄，既有 Skill 由 operator 逐項走 Registry 下架流程，不新增平行旗標。使用者自行上傳後公開分享的分類及其曝光審核與白名單的關係仍待釐清。

## Proposal and approvals

`admin-source-admission` 仍是未提交、未核准的 draft。產品已裁定先做可稽核 operator API／工具操作，Admin 畫面不作首批前置；否決後只有來源或授權證據實質改變才重審。這些裁定尚未等於 Change Package 的設計與證據審查，不得把本包當作可直接實作的已審查規則。

## Contract impact

預計有 operator-only 候選決定與狀態 API、公開收錄的具名拒絕；OpenAPI-first，現有成功回應不刪欄位。沒有已核准的新事件；跨 Context 協作須經 Registry 審查。

## Verification

九項待實作證據與一條會紅的反事實檢查列於 `test-obligations.json`。現有來源巡檢會重抓並比對 Skill 子目錄雜湊，不能稱為 HEAD-only；根授權檔變動的涵蓋範圍仍待實證。本包沒有新測試結果，也未使 `CONTENT-001`、`CONTENT-009` 或 `SEC-011` 完成。

## Residual risks

白名單目前只有策展文件流水帳，沒有可查詢現況或公開收錄閘門。公開曝光由 Publishing 審核，而 Domain Registry 尚無 Ingest 與 Publishing 的已審查互動，須在實作前明確決定與審查。法務未決的來源仍維持既有受限處置，不因准入草案放行。

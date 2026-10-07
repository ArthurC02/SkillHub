# 營運後台完整度評估

本頁是持續更新的判讀入口；需求與勾選狀態仍以[規格](02-specifications-and-acceptance-criteria.md#412-營運後台ops)及[工作清單](03-work-items.md#22-營運後台ops)為準。評估「完善」不只看頁面數量，還要看 operator 能否完成日常工作、敏感動作能否追溯、事故時是否真的能收到訊息並採取動作。

## 結論

**日常營運的既定 OPS 範圍已成形，但整個後台尚不能判為完善。**帳號查找與點數授予、Skill 治理、派送煞車、稽核、成本與趨勢、模型逾時設定都有對應畫面、API 與本機測試；然而安全治理需求中的 Skill Version 停用、來源白名單異動、角色授予的完整追溯仍缺。事故告警的建單與非工作時間送達也未在實際部署證明，P1 自動停派送只接上五類判準中的三類。這些缺口不能用「後台已有頁面」抵銷。

| operator 工作 | 目前判定 | 依據與界線 |
| --- | --- | --- |
| 找帳號、看點數、授予點數 | 既定 OPS 範圍已實作 | `OPS-002`／`003`；查詢留 audit，授予要求理由，帳號 email 不進 URL。 |
| 查 Skill 並設定受限、再散布判定或下架 | 既定 OPS 範圍已實作 | `OPS-004`；下架不可恢復且有二段確認。這**不等於**已具備版本停用或來源准入。 |
| 宣告／解除派送煞車、讀名冊 | 既定 OPS 範圍已實作 | `OPS-005`；名冊唯讀，修改仍屬部署設定。煞車 UI 不是 P1 告警送達的證據。 |
| 追查操作、成本、趨勢與模型逾時 | 既定 OPS 範圍已實作 | `OPS-006`～`011`；趨勢只回彙總，逾時設定受後端上限約束。 |
| 審核發佈物曝光 | 畫面與後端流程已存在 | 此工作由曝光審核需求管理，不能代替來源白名單或逐版本停用。 |
| 完成所有跨 Workspace 治理動作 | **未完成** | [`SEC-011` 工作項](03-work-items.md)仍未勾：版本停用、白名單異動無可驅動機制；角色啟動事件只記生效名冊，無法回答誰在何時授予。 |
| 收到並處理 P1／P2 事故 | **未完成** | [`SEC-010`／`012` 工作項](03-work-items.md)仍未勾；Alertmanager 設定不是事件單、手機通知及送達實證；五類 P1 判準只有三類自動接上，節點探針尚未在生產節點驗證。 |

## 驗證證據與沒有驗證的事

截至 2026-10-07，在 `ui/admin-ux` 的 `bde7306c3ba36ec422ede36f822aaa8deb83391a` 上，`npm --prefix apps/web test -- admin.test.tsx` 回 `exit 0`、`1 passed` test file、`112 passed` tests；這只證明元件與前端狀態處理。先前同分支的管理流程 Playwright 測試在 Chromium、Firefox、WebKit 共九次執行通過，但 `apps/web/e2e/admin-workflow.spec.ts` 以攔截回應模擬 API，**不是**前後端真實串接。後台 API 的具名整合測試曾連獨立 PostgreSQL 執行且未跳過資料庫測試；它不覆蓋缺失的治理機制，也不等於部署驗收。

本工作樹的 `task doctor` 仍回報 `.env` 與 PGlite 套件缺席，因此尚未跑「瀏覽器＋真實 API」的淨測試模式。本分支在 GitHub 尚無可引用的 workflow run；本機測試通過不宣稱 CI 或正式環境通過。正式部署的通知送達、P1 節點探針與值班處置亦無實測證據。

## 下一步與停止線

1. 先裁定版本停用能否恢復、operator 可見的最少量版本資訊；再審查[版本停用提案](../domain-memory/changes/admin-version-disable/draft-pr.md)，才實作契約、交易內准入閘門、API 與 UI。既有 Version 與歷史 Run 不能被改寫。
2. 裁定來源候選的身分鍵、白名單操作介面、來源下架對既有項目的效力與重審條件；再審查[來源准入提案](../domain-memory/changes/admin-source-admission/draft-pr.md)。未核准前不能把文件清單當作公開收錄閘門。
3. 審查[事件通知提案](../domain-memory/changes/sec010-incident-notification/draft-pr.md)，完成控制平面 P1／P2 建單、憑證與非工作時間通知的端到端演練；把 P1 剩餘訊號與實際節點探針的證據接上，同時維持單一派送煞車狀態。
4. 在不使用付費模型的環境補跑 operator 登入、查找、治理、煞車、稽核的一條真實瀏覽器＋API 旅程，並核對 CI 對該 commit 的所有 workflow。未做之前不要把 mock E2E 稱為系統驗收。

[既定 OPS 規格](02-specifications-and-acceptance-criteria.md#412-營運後台ops)刻意不包含編輯 operator／封測名冊、讀取私有 Workspace 資料、個人排行、濫用案件、下架恢復或精選層寫入 UI。它們是產品範圍邊界，不應為了讓後台看似完整而自行加上。

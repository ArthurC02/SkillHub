# 草稿：版本與搜尋內容綁定的公開曝光審核

## 使用者結果與授權

使用者已授權設計與實作內容審核／公開曝光資格，以補強意圖搜尋的反投毒邊界。本提案是[意圖搜尋](../catalog-correctable-intent/draft-pr.md)的安全前置工作，不取代 DISC-005 的七項允收，不代表正式 Registry 核准、功能已實作或投毒量測通過。

訪客只能看到仍符合審核內容的公開 Skill；擁有者仍能在自己的 Workspace 使用未公開內容。`indexed` 與 `curated` 繼續表達品質層級，兩者都可以接受曝光審核。不得用 `curated`、`verified_at`、`enriched` 或 `is_catalog` 代替審核。

## 領域邊界

Catalog 擁有曝光決定與實際被搜尋的內容；Registry 擁有不可變版本、套件身分及生命週期。採 Catalog 的獨立持久審核紀錄，不把可重建 Search Document 當作審核權威，也不把曝光規則塞進既有精選操作。

Catalog 透過組合根注入的 Registry 介面取得版本與存活事實，不直接寫 Registry 的資料。已審查的 Catalog→Registry dependency policy 允許協作，但沒有既有的曝光審核 contract 可以直接宣稱沿用；新操作仍需契約與一致性證據。

## 實作交接

審核對象不是「這個 Workspace」或「這個 Skill 大致沒問題」，而是精確快照：不可變來源版本與套件內容雜湊，加上實際用於搜尋／顯示的名稱、摘要、增強摘要、例句、標籤、限制、掃描資訊、Embedding 身分及相關衍生規則版本。具體 canonical schema 要在契約生成前列完，不能用修改時間代替內容識別；JSON key 排序等無語意差異不得任意使雜湊漂移。

現行程式查證指出一項必要前置修補：增強投影沒有來源版本，寫入時另取最新 listing facts。背景工作必須攜帶實際讀取的來源版本，寫入時核對，不可將晚到結果自動標成最新版本。

背景增強的來源版本前置防護已落實：領取工作時一併取得 `version_id` 與套件 object key，Worker／reindex 保留兩者；增強完成後，Registry 在寫回交易中先鎖 Skill，再以另一筆查詢讀最新版本，核對版本與套件身分。不同版本（包含共用同一 object key）、不同套件、刪除、下架或無版本皆不寫回，計入 failed 而非 done。這仍不涵蓋同版本重新增強的快照身分與完整曝光審核，不得當成公開資格閘門。

| 情境 | 必須觀察到的結果 |
| --- | --- |
| 初次建立／未審核 | 不公開，Owner 仍可存取 |
| Operator 核准當前精確快照 | 可公開，原本的 indexed／curated 不變 |
| 核准 V1 後建立 V2 | V2 不沿用資格；不得因索引尚未更新而公開 V2 |
| 相同版本重新增強且內容改變 | 需要重新審核，舊快照不能核准新內容 |
| V1 增強工作在 V2 建立後才完成 | 拒絕過期結果，不覆盖 V2 投影 |
| 撤銷審核後重送舊核准 | 舊 decision revision 衝突，不得恢復曝光 |
| 索引重建或背景事件重送 | 不創造審核紀錄；只有精確匹配的既有有效決定可恢復曝光 |

Operator 先取得快照與 decision revision，再提交核准／撤銷及理由。寫入以 expected snapshot 與 revision 做 compare-and-set；過期回衝突，不默默核准使用者沒有看過的內容。每個 Skill 的鎖序一致：先 Registry identity，再 Catalog 審核／投影；決定、稽核與 outbox 同交易。不得在持鎖交易內呼叫模型或等待人工作業。

公共讀取必須在同一資料庫讀取快照中核對現在的版本與資格，不能等非同步投影刷新才撤下。審核不代表執行安全或授權可散布；既有 takedown、存活、Workspace Scope、授權及增強門檻仍全部適用。

落實順序是：列全公開讀取面與 owner facts → 確定快照契約及來源版本傳遞 → 持久決定與 Operator 操作 → 所有曝光面共用一致資格 → 舊資料審核與 rollout → 品質／投毒及回歸。不能先改搜尋 SQL 就宣稱封住公開曝光。

## 提案與核准

`catalog-exposure-review` 維持 draft；使用者授權已收到，不再列為等待事項。來源版本前置修補已提交並推送為 `ccfdc84f20d7de128ecd71e949c4f5189c93cfa2`，不等於本提案完成或正式核准；`approvals` 與 `registry_updates` 保持空白，不建立虛構審查人或變更 reviewed records。

## 契約與遷移

新增 Operator-only 快照讀取及核准／撤銷命令。公開 API 會拒絕原先可見但未審核的內容，屬於行為相容性變更，不宣稱只是 additive response。

公開路徑清點至少涵蓋搜尋的所有腿與降級、瀏覽、分類數量、直接 detail／version／package 存取、創作參考與採用；也要驗證在列表取得 ID 後資格失效時，下游不能繞過。Owner-scoped 的私人路徑單獨驗證，不套用公開曝光限制。

遷移預設未審核；先提供 Operator 檢視與逐筆審核能力，明確處理現有 Catalog 後才切換公共資格 enforcement。不得因既有資料會消失而自動核准，也不得留下永久 fail-open 開關。實際 rollout 步驟與復原方式待整合驗證後納入 Provision runbook。

## 驗證

六項義務見 `test-obligations.json`，全部仍是 planned，沒有因部分前置測試通過而勾選。需要權限、版本／增強變更、重送、撤銷、併發、rollback、重建、Owner 非回歸及各公開路徑的具體斷言。分別移除來源版本核對、快照核對與 direct-ID guard，觀察對應測試變紅，再恢復變綠。

前置防護的本機證據：`TestBackfillDiscardsPackagesThatAreNoLongerCurrent` 覆蓋 current／replaced／same-package-new-version／deleted／taken-down／no-version；`TestBackfillDiscardsAnUnidentifiedSkill` 覆蓋缺少 Skill 身分。以獨立 `_test` PostgreSQL、`SKILLHUB_REQUIRE_DB=1` 執行。移除寫回前的 `!current` 攔截後，負面情境因錯誤呼叫 projection writer 失敗；另外單獨移除版本 ID 比對，same-package-new-version 因錯誤寫入而失敗。`TestEnrichmentClaimsTheExactVersionAndLeavesVersionlessSkillsUnclaimed` 驗證 Catalog 回傳精確版本且不領取無版本項目；把版本映射改成空值後失敗。上述突變皆 exit 1，恢復後通過。

`TestCurrentPackageRemainsLockedUntilProjectionTransactionEnds` 用第二筆交易的 `FOR UPDATE NOWAIT` 確認目前 Skill 仍被鎖住（SQLSTATE `55P03`），再確認原交易提交後能建立新版本，且舊套件不再符合。`TestCurrentPackageReadFailureIsNotAContentDecision` 以已關閉交易確認讀取錯誤回傳 `ErrTxClosed`，不是當成一般不符合或允許寫回。把鎖定讀取改成普通讀取、把讀取錯誤改成 nil，兩條測試分別因鎖可取得及錯誤被吞而失敗，exit 1；恢復後通過。恢復前後產品檔案 blob hash 一致。這證明持鎖期間的排他性，不宣稱已覆盖完整曝光審核的併發行為。

受影響套件 admission／library／discovery 的完整回歸為 420 pass、2 skip（含子測試，exit 0）；跳過的是未提供 `QA002_CORPUS` 的破損套件語料與開發目錄量測，資料庫回歸未跳過。API 領取版本測試另行通過，零跳過。本次未呼叫付費模型，模型 HTTP 使用替身。原始本機輸出在 scratchpad，不是已提交的正式 attestation。

投毒報告必須分開保留兩個問題：已被放進候選集的毒文件如何排名，以及未審核／被替換的內容能否取得曝光。新資格閘門只直接保證後者，不是語意毒性辨識器；Operator 若誤核准毒文件，前者仍可能失敗。原有已准入投毒 Top-3 失敗不改寫為通過，不以全部不核准而得到空頁充數。正面語料的審核需有實際內容證據，不能按測試標籤或檔名決定。

## 剩餘缺口

尚須完成公開讀取面的逐條盤點、canonical schema、跨 Context facts 操作與 rollout 的實測設計，再落實完整資格控制。現行已准入投毒紅線仍未過；如安全決策需明確區分准入控制與已准入排序，必須直接寫清威脅與限制，不偷換驗收語意。

環境權限阻礙已解除；釘選 Node 版本的 doctor、`task gen:sql`、`task gen:check`、受影響套件 lint 與 automation-check 均已成功。SQL 產物經正式生成，未手改。前置修補已 Commit／Push；完整曝光功能仍未完成，遠端 CI 結果須以該完整 SHA 的所有 workflow 即時查證，不沿用本機通過作結論。

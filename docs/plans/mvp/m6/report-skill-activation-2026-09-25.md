# Skill 啟用 Demo：真實事件與未啟用對照

## 結論與界線

丙-107 的可重建素材、真實啟用事件、版本關聯與一般／進階介面核對已補齊。三次 Run 全部輸出 BETA、執行成功、任務判定符合；只有前兩次記錄技能啟用。**輸出正確不能證明使用了 Skill**。

這是使用者授權的 Agent 代操作、真實付費模型驗收，不是真人採用或封測數字。環境是 Windows 淨測試模式，`isolation_strength=none`，沒有驗證 gVisor 隔離、正式部署或任意不受信任套件。未變更產品程式、公開入口或模型政策。

## 可重建的輸入

完整 `SKILL.md`、允收條件與操作流程見 [Demo Runbook](../../../runbooks/skill-activation-demo.md)。本次使用純文字 BETA 版本，不含 Script，zip 根目錄只有 `SKILL.md`。先匯入 ALPHA 版本，再建立 BETA v2；以下三次都掛載 v2。

- 程式 commit：`cfc66a52ef784467b103172b2938173997a97243`。
- Skill：`8cf869a1-6887-4fcb-8a1c-97163f7e4f45`。
- Skill Version：`9ca0e0ed-ea5c-4841-9361-360278fced32`。
- 套件 content hash：`3790b3b4b05df604af14567ef2eee551942cf1f4432ee9f37b730fa1ce72582a`。
- Test Case：`fa5a9956-d0e8-47b1-bada-650bd4765600`；允收條件 `最終輸出必須恰好為 BETA。` 已確認。
- Run 模型：`gpt-5.4-mini`；Judge：`gpt-5.6-terra`、`judge-run/v2`；runtime：`claude_agent_sdk`、`0.3.233`。模型一律經本機 LiteLLM 閘道。
- 每次均取得並確認 preflight；摘要 hash 為 `cc2f7c2f109947fb314c06f752a4f8de63003e51cbdf42d3257638907f69b59c`。暫時放行只限上述已審閱版本，驗收後撤除。

| 案例 | 完整 user_prompt | Run ID | 不可變 snapshot ID |
| --- | --- | --- | --- |
| 未點名 | `請依技能規則輸出指定的單字。` | `82744653-02a2-4daa-8f5f-0a88d7556a8b` | `94d9363e-4bae-4a9b-9529-df37dd1a13de` |
| 明確點名 | `請使用 run-comparison-acceptance 技能，依照 SKILL.md 回覆。要輸出的單字是 BETA，最終回覆只有 BETA，不要解釋。` | `8f6d7fca-12c7-46d0-84b0-c6cc0076190f` | `af1e67d4-ec18-4ef6-842f-c0036a3b5690` |
| 明確不啟用 | `這是未啟用對照：不要使用任何 Skill，不要呼叫任何工具，也不要讀取檔案。只回覆 BETA。` | `beb55c80-a14f-4856-b2fe-d114168dcf05` | `88341e90-791a-4628-9134-5e093e8b2471` |

未點名案例這次也啟用了 Skill，與先前比較報告的取樣不同，因此不再把「未點名」當作負向條件。新增明確不啟用案例保留實際對照；三次結果全部保存，不挑選成功結果隱藏其他觀測。即使明確要求，也不承諾模型每次選擇相同。

## 原始 API 證據

每份 JSON 保存四個 HTTP 回應的 status 與完整解析後 body：`GET /runs/{id}`、`GET /runs/{id}/trace`、`GET /runs/{id}/trace?mode=advanced`、`GET /runs/{id}/evaluation`。皆為 HTTP 200、Run `succeeded`、清理 `cleaned`、evaluation `completed/met`；沒有保存認證標頭、環境設定或帶簽章網址。

| 案例與原始回應 | 啟用事件 | 完整事件數 | Run 用量／評估點數 |
| --- | --- | --- | --- |
| [未點名](report-activation-unqualified-2026-09-25.json) | `3fd7c176-ace3-4890-8548-57c9bb05736e`，`2026-09-25T15:33:37.212Z` | 7 | 36／8 |
| [明確點名](report-activation-explicit-2026-09-25.json) | `ec94ff13-0e28-4ffc-beac-99b78fc2f969`，`2026-09-25T15:34:28.775Z` | 7 | 11／5 |
| [明確不啟用](report-activation-control-2026-09-25.json) | 無；一般摘要 `skills_total=0`、工具呼叫 0 次 | 5 | 8／4 |

兩筆 `skill_activation` 的 `decision=activated`、`skill_name=run-comparison-acceptance`，`skill_version_id` 均與對應 Run 完全一致。三份進階 Trace 都是 `complete=true`、`has_more=false`，各 stream 的 `missing_count=0`；不是只看第一頁就判斷沒有啟用。評估事件是遲到事件，但已收齊並包含在上述數量內。點數分開列，Run Trace 的用量為下界，不宣稱是帳務總額。

## 真實瀏覽器核對

2026-09-25 以瀏覽器登入本機驗收帳號，在 `/runs/{Run ID}` 逐頁讀取 UI，並實際切換一般／進階模式。以下是瀏覽器 accessibility tree 的**人工轉錄節錄**，不是由 API 產生的假畫面，也不是 PNG 截圖檔；一般頁的視覺呈現另在本次會話內檢視。可攜證據以這份轉錄、URL 與原始 JSON 相互核對；本機暫存資料庫消失後，URL 不能當作永久可開啟的證據。

| 畫面區域 | 明確點名 Run | 明確不啟用 Run |
| --- | --- | --- |
| 任務判定 | `任務判定：符合` | `任務判定：符合` |
| 執行狀態 | `執行完成（succeeded）`，並說明不是任務達成與否 | 同左 |
| 一般模式／使用的 Skill | `run-comparison-acceptance（已啟用）` | `Skill 啟用事件 0 筆。` |
| 一般模式／最終輸出 | `BETA` | `BETA` |
| 一般模式／工作內容 | 工具呼叫 1 次、成功 1 次，工具 `Skill` | 工具呼叫 0 次 |
| 評估／啟用 | `the skill version mounted for this run was activated`，附原始事件引文與版本 ID | `no activation event appeared for the skill version mounted for this run`，警告不推斷模型是否看過技能 |
| 進階模式 | 第 1 頁、沒有更多事件；`skill_activation` payload 的版本 ID 為 `9ca0e0ed-ea5c-4841-9361-360278fced32` | 第 1 頁、沒有更多事件；3 筆 sandbox 與 2 筆 orchestrator 事件，無啟用事件 |

未點名 Run 也在一般模式確認為已啟用、輸出 BETA、判定符合。這三份結果足以示範啟用、輸出與任務判定的差異，不代表技能有效性或採用率。

## 環境限制

`task doctor` 明確報出 Node `v25.0.0` 不符合 repo 要求 `v24.21.0`，因此不宣稱本機環境基線全綠。這不改寫上面的實跑觀測；提交品質另以生成漂移、文件檢查與該提交的遠端 CI 判定。本項只補驗收素材與證據，沒有宣稱修復產品 bug，不適用程式修補的突變證明。

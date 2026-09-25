# Runbook：示範真實 Skill 啟用

## 驗收的是什麼

本手冊用已知純文字 Skill 驗證「模型有啟用掛載的版本」，不是只驗證它能輸出指定單字。輸出正確、Run 執行成功、技能啟用是三個獨立事實，不能互相代替。這也不是模型品質、真人採用率或正式 Sandbox 隔離的驗收。

先依[整體 Provision](provisioning.md)建立可執行付費 Run 的環境，確認 API、Worker、模型閘道、Sandbox 與評估服務就緒。模型費用須有授權；不要為此公開生成入口。淨模式只能使用已審閱的內容，每次重建都重新核對版本 ID，不沿用舊資料庫的放行。

## 固定素材

建立 UTF-8、LF 換行的 `SKILL.md`，檔尾保留一個換行，放在 zip 根目錄；套件不包含其他檔案：

```markdown
---
name: run-comparison-acceptance
description: A pure text fixed response for run comparison acceptance.
license: MIT
---

# Response

For every input return exactly BETA and nothing else. Do not use tools.
```

這是先前實跑的版本二內容，保留原樣；「Do not use tools」是啟用後的工作指示，不是要求測試工具捏造啟用事件。若調整正文或名稱，必須建立新版本與新證據，不再宣稱沿用同一份素材。

固定允收條件：`最終輸出必須恰好為 BETA。` 建立 Test Case 後明確確認該條件。

| 案例 | 完整 user_prompt | 如何解讀 |
| --- | --- | --- |
| 未點名對照 | `請依技能規則輸出指定的單字。` | 先前未啟用並反問；這不是保證永不啟用的負向測試，每次保留實際結果 |
| 明確點名 | `請使用 run-comparison-acceptance 技能，依照 SKILL.md 回覆。要輸出的單字是 BETA，最終回覆只有 BETA，不要解釋。` | 先前有啟用事件並輸出 BETA；prompt 本身含答案，所以 BETA 不能證明有使用技能 |
| 明確不啟用 | `這是未啟用對照：不要使用任何 Skill，不要呼叫任何工具，也不要讀取檔案。只回覆 BETA。` | 補驗時無啟用事件但仍輸出 BETA；每次以實際 Trace 為準 |

前次觀測在[Run 比較報告](../plans/mvp/m5/report-run-comparison-2026-09-25.md)。這份手冊保存可重建的輸入，不保證模型每次做相同選擇。

## 執行與保存

1. 匯入套件，記錄新環境回傳的 Skill ID、版本 ID 與 content hash；檢視套件內容確認沒有 Script。不得把檔案雜湊和套件 content hash 混為一談。
2. 使用同一版本跑明確點名與明確不啟用案例；未點名案例可用來觀察模型選擇，但不能當作必然不啟用的條件。各案例可使用不同 Test Case，或保留同一 Test Case 的不可變快照；保存各自的完整 prompt 與 snapshot ID。
3. 每次先檢視並確認執行前權限與費用摘要，再送出 Run。不得繞過併發上限或重送已成功的建立請求。為本機純文字素材設定的臨時放行在驗收後撤除。
4. 等待 Run 終態與獨立評估完成。保存平台回傳的 Run、一般 Trace、進階 Trace 與評估；進階 Trace 如有分頁，要沿回傳的分頁資訊取完，不以第一頁沒有事件判定未啟用。
5. 在真實瀏覽器開啟 Run 結果頁，核對一般模式的「使用的 Skill」、最終輸出與任務判定；切進階模式核對原始啟用事件。保存畫面與 API 證據的對照表。

證據只存已遮罩的回應，不能存 Cookie、Authorization、Virtual Key、帶簽章的物件網址或整份環境設定。若回應仍有秘密，停止公開保存，先處理遮罩缺口。

## 通過與不通過

| 要證明的事 | 必須看到的證據 | 不能當成證據 |
| --- | --- | --- |
| 啟用的是這次掛載版本 | 真實 `skill_activation` 事件，decision 為 activated，版本 ID 與 Run 的 skill_version_id 相同 | 模型自述、檔案名稱、最終輸出 BETA |
| 介面沒有把兩種結果混在一起 | 一般模式顯示實際啟用狀態；執行狀態與任務判定分開；與 API 一致 | 只有固定 fixture 的元件測試 |
| 對照確實沒有記錄啟用 | 完整 Trace 中無對應啟用事件，介面保留未啟用／零筆的揭露 | 把沒事件說成「模型一定沒讀過內容」 |
| 結果可追查 | commit、模型與 runtime 版本、Skill Version、Test Case snapshot、Run ID、事件 ID 與畫面對得起來 | 只有一段成功摘要 |

若明確點名後仍無事件，記錄為沒有觀察到啟用，不改 Trace、不把 BETA 當替代品。若對照也啟用，照實保留，不能改標為未啟用；另用已保存的歷史未啟用案例說明差異。現場 Demo 也必須如實解釋這些結果。

## 已保存的驗收證據

[啟用 Demo 驗收報告](../plans/mvp/m6/report-skill-activation-2026-09-25.md)保存三次真實 Run 的完整 API 回應、事件與版本關聯，以及瀏覽器一般／進階模式的核對轉錄。明確點名與明確不啟用均輸出 BETA，但只有前者有啟用事件。未點名案例在這次也啟用了，與先前取樣不同；這正是不能把 prompt 寫法當成啟用證據的原因。資料庫為暫存，重新 Demo 應重建素材並保存新結果，不依賴歷史 localhost URL。

# 互動創作 enrichment：代理協助的真實模型驗證

## 驗證範圍

本次使用者授權代理決定缺值處理與後續修訂方向，由代理透過產品 API 回答問題、確認需求、保存候選、建立 Run、帶回評估。這是**受委託代決的功能驗證，不是真人自行操作或願意採用的量測**，不填 `kept_by_owner`，也不代表 GEN-012 的分布門檻重新通過。

環境依 [Provision runbook](../../../../runbooks/provisioning.md) 的獨立本機驗證路徑：真實 Go API／Worker、Python 能力服務、LiteLLM、執行代理與 Judge；資料庫是 clean mode 的單連線 PGlite，物件儲存為程序內實作。Sandbox 宣告 `isolation=none`，只執行經檢閱的純文字 Skill，沒有 Script、附檔或要求連網；**不構成 gVisor、正式物件儲存或正式部署的驗證**。生成入口只在本機開放，未改封測曝光政策。

作者與執行模型為 `gpt-5.4-mini`，Judge 為 `gpt-5.6-terra`。會話預算 1300 credits、上限 24 步／8 次工具；金鑰不保存於本報告。這一場沒有要求 `fetch_url`，不把其他測試的 SSRF／同意證據算成本場實測。

## 任務與決策

任務是把 CSV 銷售資料整理成繁體中文摘要。缺值不補 0、不計入銷售總額與排名；同品項先合計有效數字，最高額相同列出全部品項。全數缺值時明說無有效數字、無法計算總額與無法判定最高品項。

首次樣本包含蘋果 120、香蕉缺值、蘋果 80、橘子 200、鳳梨缺值。它能驗有效總額 400、蘋果與橘子並列 200、缺值 2 列，卻不能驗「全數缺值」。後續另用蘋果與香蕉都缺值的樣本，保留第一場不可變的 Run；不把 `undetermined` 改寫成技能錯誤，也不把兩份樣本混成同一次量測。

## 識別與觀察

會話：`d91b7943-b1ac-4615-bf35-5b0bd126ef32`；Skill：`11e71526-f382-42d7-8fe5-2f3d01e98186`。識別值屬本機暫存環境，不是正式站可永久開啟的連結。

| 場次 | Run | 版本 | 觀察 |
| --- | --- | --- | --- |
| 混合資料 | `c6ab7237-93fe-4437-9885-281fe98ae95c` | `3d7f969e-4b42-41b4-8a87-f89770fb99cd` | succeeded、cleaned；Skill 啟用有 Trace；5 passed、1 undetermined，overall=partially_met |
| 全數缺值 | `44823b4d-b41f-4f9b-8b11-276d4c10363e` | `39265e5f-80f3-4f89-a7f3-8723b021c3c5` | succeeded、cleaned；5 passed、1 failed，overall=partially_met；沒有 Skill 啟用事件 |
| 明確要求使用 Skill | `7dfeeeb1-1cc7-42e3-905d-e1ab65f66bac` | 同上 | succeeded、cleaned；Skill 啟用有 Trace；6 passed、0 failed、0 undetermined，overall=met |

第一場 Judge 正確指出全缺值情境在樣本中不存在。`attach_run` 使會話在 revision 17 進入 `waiting_input`，Go 產生指明該條件與「這份樣本驗不到」的追問；沒有在代理回答之前擅自修改草稿。其工具觀察與追問保留同一時間戳 `2026-09-25T09:51:09.2285656Z`。

第二場 Judge 指出沒有明確的列數統計；雖然執行成功，沒有啟用事件，所以不能宣稱這場使用了掛載的 Skill。第三場維持六條允收條件，只把樣本請求改為明確點名 Skill。模型曾宣稱補入統計欄位但正文未變，因此第三場實際沿用第二版內容，**不能說統計模板已完成修訂**。

兩個版本的內容 hash 分別為 `e87d6debadf11c3f84490183b69578dd19282c923e302b766c25e1db86189aa5`、`8e406de6fa0a9db4d901344426154ad4b0e9044c2906cce3f81c5feadc14e616`；第二版確實新增全缺值不得填 0 的正文規則。保存相同內容仍回同一版本，另建新的 Test Case，不覆寫歷史 Run。

第三場啟用事件為 `aaddac15-d912-4d70-8847-ea27379509a8`，評估 `3fb5df57-0f9e-4409-ba1b-1c9ff1e58f4c` 在 `2026-09-25T10:02:36Z` 完成，`evidence_complete=true`。實際輸出如下：

```text
資料摘要：共有 2 筆資料；品項為蘋果、香蕉；銷售數字皆缺值。

有效銷售額總和：無有效銷售數字，無法計算

缺值列數：2

受影響品項：蘋果、香蕉

最高銷售品項：無法判定，因為全部列皆缺值
```

最終 `attach_run` → 模型檢視 → `finalize` 成功，revision 55、state=`saved`，保留上述版本、第三場 Run 與 Test Case `869efbb1-4bf4-4bef-913c-92135a774dd7` 的連結。共 18 個模型步驟、4 次工具意圖，會話列報 277 credits，reserved=0、usage_unknown=false；三次 Judge 分別列報 26／21／21 credits。後兩場執行 Trace 成本為 unreported，不是零，因此本報告**不加總成全程實付**，也不把會話花費當作包含 Run 與 Judge 的總額。

## 實跑揭露的工程缺陷與反證

保存前查重、跨 context 讀取與修訂時 enrichment 曾持有交易再向同一連線池取第二條連線。已將外部準備／模型呼叫移出寫交易，最後鎖定會話重新驗證 revision、內容 hash 與狀態，再原子保存。新建、既有 Skill 修訂與 `attach_run` 已在這場單連線環境成功完成；取消競態仍由回歸測試守住，沒有以加大 pool 掩蓋問題。

另發現 `validate_draft` 接受不同內容後保留前一版的 `RunUnmet`，下一步因與剛驗證的草稿相同而誤稱「與試跑那份相同」。修正只在 hash 改變時清除舊判定，不宣稱新稿已試跑通過。這場執行中的 API 未重啟以保留暫存會話；此項新修正由下面測試驗證，不冒稱本場已執行新版二進位。

| 回歸測試 | 條件與反證 |
| --- | --- |
| `TestValidateDraftRecordsThePreviousDraftWhenTheHashChanges` | 新 hash 保留舊稿、清除舊候選與舊判定；隨後接受相同新稿不誤追問。移除清除判定那行 → FAIL：新稿繼承舊判定 |
| `TestValidateDraftDoesNotShortCircuitWhenTheStoredDraftIsBlocked` | 相同 hash 的重新靜態驗證保留 Run 判定。改為無條件清除 → FAIL：相同內容遺失判定 |

兩次突變均已還原；creation 套件 315 passed、1 skipped、0 failed，跳過的是需 `SKILLHUB_LIVE_FETCH=1` 的網路量測。`automation-check` 成功，四類生成檔皆 current，comment lint 與 diff check 結束碼 0。以完整 SHA `89e7e67388fda0cf0fe22e3540901a474e9919b7` 查詢，1 個 workflow：[CI completed/success](https://github.com/ArthurC02/SkillHub/actions/runs/36121658573)。該次未執行 web-browser、sandbox-nodocker、web、tools-pglite、qa002、sandbox、llm、deploy-config、sandbox-windows，不拿其綠燈宣稱這些區域重新驗過。

## 證據限制與尚缺

本次 API 觀察能驗證每輪訊息、時間戳、候選與 Run 的連結，沒有新增瀏覽器畫面驗證或真人可理解性評分。模型仍可能只在訊息裡說改好了、正文不變；應檢查實際內容與 Run，不能以回覆語氣代替證據。

真人答問、可理解性與願意採用仍待收集；本機功能路徑也不能代替正式隔離部署驗收。歷史報告與分布數字不回溯修改，當前剩餘工作以 `04` 為準。

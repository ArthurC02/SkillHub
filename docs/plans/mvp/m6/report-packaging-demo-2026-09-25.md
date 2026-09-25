# 可攜套件 Demo：授權、Fork 與下載驗收

## 結論

丙-105 的 Demo 下載阻擋已用獨立、有來源說明的固定素材解除；沒有改寫既有量測語料、沒有批次放行第三方種子。安全攔截仍有效：未知授權 422、非 operator 404、授權證據不符 400、跨 Workspace 的 artifact 與內容讀取 404。

使用者授權 Agent 代操作，不是真人採用。驗收使用 Windows 淨模式的暫存 PGlite／記憶體物件儲存，不驗證正式儲存、gVisor、簽章或下游 Agent 安裝。沒有產品程式修改；此報告是既有流程的實跑證據，不宣稱新增自動回歸測試或修復程式 bug。

## 素材與環境

程式 commit：`e98ed5e87e37e8cd4a52bece3cee94198b86812b`。完整素材與重建程序在 [Runbook](../../../runbooks/portable-demo.md)。素材只有新撰寫的固定回應，MIT 由 manifest 宣告；不以別人的 repo 根 License 替其內容背書。

API `127.0.0.1:9090`、能力服務 `127.0.0.1:8000`，經既有本機模型閘道完成匯入增強。生成與互動創作入口關閉，沒有啟動 Run。增強回報模型 `gpt-5.6-sol`、提示 `enrich-skill/v7`；這是既有產品模型設定，不是子代理派工。`task doctor` 仍指出本機 Node 25 不符規定的 24，不能宣稱本機環境基線全綠。

第一次打包回 503「沒有設定套件的保存期限」。補上本機 `DOWNLOAD_ARTIFACT_RETENTION=720h`，停止自己啟動的行程並重建淨模式後重驗；下表全部是重建後同一次環境的 ID。最初的 503 另保存在原始證據，沒有當成授權拒絕。

| 物件 | ID／值 |
| --- | --- |
| 原始 catalog Skill | `3d30b6b2-3c2a-4109-b600-37735b445936` |
| 原始 Version | `588e82fe-3c2b-48a6-b118-ea92b23758ff` |
| Fork Skill | `c48d7082-786c-4c59-a605-546f2215bc5e` |
| Fork Version | `917d29c4-5a60-4a0d-afcc-f54964974cda` |
| 兩個版本的來源 content hash | `ff371dd68314d1ca38bbdbe13ec80d09bc6f7b2b2d1256db5ac5b7882b3b4c03` |
| Download Artifact | `56774250-5c25-4491-9c7c-7e03ee00f7b4` |
| ZIP SHA-256 | `3ea320cd6d35aa43589c977054e63986d316301ab32be22908ab6951dbf601f4` |
| manifest hash | `d6be954cb4486f4c170205b97305770f57bd44457e4fe4c599ca68c61ec728f8` |

## API 與稽核

[完整選定回應與 ZIP 內容](report-packaging-demo-2026-09-25.json)包含 HTTP 方法、路徑、status、完整 body；未保存認證標頭。`seed` 是匯入回應，`gates` 是順序驗收，`fork` 是瀏覽器 Fork 後讀回的結果，下載與 archive 欄位記錄後續核對。不是所有網路流量的封包錄製。

| 條件 | 實際結果 |
| --- | --- |
| operator 身分 | `/me` 的 operator 為 true，一般使用者為 false，Workspace 不同 |
| 原始版本宣告 MIT，但可散布性 unknown | 打包 `422`、`blocked_reason=license_unknown` |
| 一般使用者以正確證據嘗試放行 | `404 not found` |
| operator 提交 Apache-2.0，版本實際為 MIT | `400` 並說明快照是 `MIT from manifest` |
| 兩次拒絕後 | 狀態仍 unknown；operator audit events 為空 |
| operator 提交相符 MIT／manifest 與理由 | `200`、`previous_value=unknown`、新值 allowed |
| 放行稽核 | `2026-09-25T15:47:50Z`，一筆 `skill.redistribution_set`，before／after、actor、來源 Workspace、授權證據與理由齊全 |
| 一般使用者 Fork | private scope、allowed，來源 Skill／Version 與上表一致，內容 hash 未變 |
| 下載 | `200 application/zip`、4941 bytes、檔名 `portable-demo-fork-v1-standard.zip` |
| 第三個 Workspace 查 artifact 與 content | 兩者均 `404 download not found` |

完整下載位元組另存為 [原始 ZIP](report-packaging-demo-2026-09-25.zip)，可直接重新計算 SHA-256，不需重建當時的暫存服務。只需離線讀取，不執行套件內容。

## 真實 UI 與內容核對

以瀏覽器登入 `packaging-demo-user`，在目錄 Skill 頁確認「可再散布」及「要先 Fork 一份」，實際按 Fork，再從新 Skill 的打包入口建立 standard 套件；沒有用 operator 工作區的打包來冒充一般使用者旅程。

瀏覽器 accessibility tree 的人工轉錄：預覽顯示「這些設定可以打包」、阻擋級錯誤 0；建立後「套件已建立」、「狀態：可下載」、保存 30 天。按下載後 API 紀錄出現 `2026-09-25T15:48:28Z / packaging-demo-user`。另以同一使用者 API 下載保存 ZIP 供檢查，新增第二筆 `15:48:58Z`；下載紀錄 UI 顯示 2 次，展開後兩筆操作者相同。**兩次下載是 UI 一次＋內容核對一次，不是兩名使用者。**

ZIP 的 SHA-256 與 artifact 回報相同；三個 entry 恰為 `INSTALL.md`、`SKILL.md`、`skillhub-manifest.json`。`SKILL.md` 與匯入素材逐字一致（素材為 ASCII，因此 UTF-8 位元組亦一致）；manifest 的 Fork／upstream 版本、來源 hash 正確，validation 無阻擋、無錯誤。能力與行為仍是 `unverified`，不因下載成功變成通過。原始 archive 欄位保存三份完整文字，可離線查閱，不依賴已消失的 localhost URL。

本機核對命令明確輸出 `PASS: archive hash, exact source text, three files, fork/upstream version linkage and validation; skipped 0`，exit 0。文件與生成漂移檢查及遠端 CI 在提交流程另行核對；這些不能替代上述實際下載證據。

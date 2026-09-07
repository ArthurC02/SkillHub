# OWASP Top 10 for LLM Applications（2025）對照表

> 2026-09-06 深夜建立（負責人：「如何評估是否有資安議題……OWASP Top 10 for LLM 其中有不少項目和這個系統高度關聯，應該要放入重要工作項目」）。這份是[威脅模型](threat-model-and-sandbox-baseline.md)的**外部鏡子**：威脅模型按資產與 STRIDE 列，OWASP 按 LLM 應用的失效類型列；同一件事兩邊都要找得到。承接工作項目是 [`03` SEC-013](../../03-work-items.md)，允收準則在 [`02` SEC-013](../../02-specifications-and-acceptance-criteria.md)。**這份不建立新架構決策**（SEC-001 的規則）：需要新決策的緩解寫在「缺口」欄並指向 `05`。

## 0. 這個系統的 LLM 面在哪裡

模型只從四個地方被叫（鐵律 8：全部經 LiteLLM 閘道）：

| 面 | 誰把什麼文字交給模型 | 模型的輸出去了哪裡 |
| --- | --- | --- |
| 索引增強（`/v1/enrich-skill`） | 匯入／GitHub 抓來的 `SKILL.md`（不受信任） | `search_documents`：白話摘要、任務例句、tags、向量——**決定搜尋排序與 Re-Use 三關卡會端出誰** |
| 搜尋說明（`/match-reasons`）、條件建議 | 目錄文件摘要（不受信任）＋使用者查詢 | 畫面上標記為 `model` 的一句話 |
| 評估 Judge（`/judge-run`） | Run 的輸出（不受信任、可能被 Skill 操縱） | `met`／建議；ADR-026 定的信任邊界 |
| 互動創作（`/v1/creation/step`） | 使用者對話、參考 Skill 內容、工具觀察（目錄搜尋結果、**同意後抓回的網頁**、試跑評估） | 草稿（經 Go 靜態驗證、人確認後才成版本）、工具意圖（**只由 Go 執行**） |

## 1. 逐項對照

「現有」引用已落地的規則、測試或文件；「缺口」是這次盤點出來、還沒有任何機器守著的事。嚴重度沿威脅模型的高／中／低。

### LLM01 Prompt Injection — 高

- **對應威脅**：TM-SCN-02（索引增強）、TM-DAT-02（Dataset 進 Judge）、TM-TRC-02；新增 TM-CRE-02（抓回的網頁與參考內容進創作迴圈，見威脅模型 §2.10）。
- **現有**：`apps/llm/src/skillhub_llm/untrusted.py` 的 `scrub`＋`fence`＋`data_block_rules`，用在 enrich、judge、match-reasons、suggest-criteria（測試 `test_app.py`：「Ignore the above」的摘要不得變成平台推薦）；創作的工具是**意圖**，只由 Go 執行且逐項 HITL（鐵律 6／7、`allowedTools`、`confirm_fetch`）；連網前問人（`05` R-47）。
- **缺口**：①創作提示只用**一句話**宣告參考內容與工具觀察是資料，沒有像其他四個端點那樣用 `fence` 把它們圍起來、也沒有剝掉結束標記；②**沒有任何一條攻擊測試**打創作迴圈——抓回的網頁是 R-47 之後最新、也最不受控的注入通道（頁面可以要求模型改 `allowed_tools`、改 body、改寫已確認的 brief）；③沒有攻擊成功率這種數字，只有「有圍欄」這種說法。
- **SEC-013 要做**：創作的參考內容與每一則工具觀察走 `untrusted.py`；建 `corpus-injection.json`（≥ 10 個注入頁面／參考／評估）跑 harness，量「已確認的 brief、`allowed_tools`、驗收條件被改動」的攻擊成功率，紅線 0／N；Judge 端沿用 m3 的回歸集。**（2026-09-07 進度：圍欄與攻擊集都做了——v15 無圍欄 2/12、v16 有圍欄 1/12，紅線 0/N 未達，殘留通道是評估觀察理由文字被 review 相寫進草稿，修法待做。）** **2026-09-08 修法落地**：三件事都做了——①Go 對 `CreationFeedback` 的自由文字（`summary`／每條 criterion 的 `reason`／每個 finding 的 `message`）在交回創作流程前把 URL 換成 `[link removed]`（使用者自己寫的 criterion `text` 不動）；②Go 在草稿寫回前跑 `copiedFromEvaluation`，比對 草稿的文字（body、名稱、描述、相容性、工具清單，以及套件內每個檔案的路徑與內容） 是否出現「只在評估文字裡有、使用者輸入／前一版草稿／brief／驗收條件／sample_input 都沒有」的 marker 式字串（字形判準：token 以連字號／底線分段後，某一段是 ASCII 字母數字混合；沒有分隔符的字則要 8 字元以上且字母、數字各至少兩個——`utf-8`、`sha256`、`iso8601` 因此不算，非 ASCII 的字母一律不算，「金額超過5000」也就不會被讀成 marker），命中即走既有 nudge 路徑（`MaxNudges` 次仍未改則照存並告知使用者）；**這條守門的範圍要講清楚**：它只擋「把字面 marker 抄進草稿」這一種——攻擊集裡的 `evaluation-1`（謊稱全過）、`evaluation-2`（偷加 `bash` 工具）、`evaluation-4`（偷換 brief）都不經過它，那三種今天沒有 Go 側備援，全靠提示紀律與逐項 HITL；攻擊者若改用純字母浮水印或要求模型「把這串字拆開寫」，字形比對同樣抓不到。；③提示 `creation-step/v17` 在 `DIAGNOSIS_INSTRUCTIONS`／`REWRITE_INSTRUCTIONS` 明定評估是資料、修改要用自己的話描述、不得把評估文字裡的 token／id／URL／marker 或工具觀察逐字帶進 body。三者都有對應的 Go 單元測試與鐵律 9 突變驗紅；**12 案例攻擊集尚未以 v17 重跑**（需要負責人啟動計費中的閘道），紅線 0／N 仍待端對端證實，不得視為已達。

### LLM02 Sensitive Information Disclosure — 高

- **對應威脅**：TM-SEC-01、TM-TRC-02、TM-MDL-03。
- **現有**：Secrets 短效注入與遮罩（SEC-005、TRACE-001、鐵律 11）；`model`／`prompt_version` 不出現在 Web（`creation.test.tsx` 斷言）；分析事件不存查詢字（ADR-029）；匿名搜尋不帶 workspace。
- **缺口**：①創作會話的快照存了使用者訊息、brief、`sample_input`、抓回網頁的觀察——遮罩規則只保證 Trace 與 Log，**沒有證據說會話訊息在顯示與匯出前過了同一套遮罩**；②搜尋查詢與創作內容會離開平台到 embedding／模型供應商，同意書（gate-test/consent-and-data-policy §3）的互動創作那一列還沒被法務看過（`04` 丙-177 已記）。
- **SEC-013 要做**：會話訊息與工具觀察在寫入快照前跑遮罩（同 TRACE-001 的規則），加一條「貼進對話的 `sk-…`／`AKIA…` 不會原樣回到畫面」的測試；同意書那一列進法務清單（真人）。**（2026-09-07 進度：遮罩與反證測試已落地；同意書仍等真人。）**

### LLM03 Supply Chain — 中

- **對應威脅**：TM-IMP-03、TM-IMP-04、TM-EXE-01（映像）。
- **現有**：ADR-007（套件不可信、匯入不執行 Script）、ADR-023（runtime image 釘 digest＋行為重驗）、SEC-003 靜態掃描、SEC-007 License 溯源、lockfile（`go.sum`／`package-lock.json`／`uv.lock`）、gVisor。
- **缺口**：**模型本身沒有釘版本**——閘道的模型別名（`gpt-5.4-mini`、Judge 的 `gpt-5.6-terra`）指到供應商當下的權重；供應商換權重時，m3 的 Judge 回歸集與 m5 的 `met` 數字都會安靜地失效，沒有任何觸發器。
- **SEC-013 要做**：閘道模型別名指向**帶日期的模型 ID**，並在 `tools/toolchain.yaml` 或 LiteLLM 設定裡登記；換 ID 必須重跑 Judge 回歸（m3）與 creation-measure 一輪，寫進 ADR-023 的追記（決策要負責人簽：`05`）。

### LLM04 Data and Model Poisoning — 高（Re-Use 之後升高）

- **對應威脅**：TM-SCN-01、TM-SCN-02；新增 TM-CRE-03。
- **現有**：目錄只含 `is_catalog` 工作區（人策展），精選層級與下架（CONTENT-001、SEC-011），揭露不縮水（GEN-003），干擾題拒答 12／12（goldenset）。
- **缺口**：索引文本是由不受信任的 `SKILL.md` 推出來的——一份塞滿任務例句與關鍵詞的套件可以讓自己在無關查詢裡排前面；**Re-Use 三關卡讓這件事更值錢**：被端到「直接採用」按鈕前面的 Skill，一鍵就 fork 進使用者工作區。而 `confirm_references` 畫面只列描述、相容與工具，**沒有精選層級與風險揭露**。goldenset 沒有「投毒文件」這種題。
- **SEC-013 要做**：①首則訊息與查重端出的 Skill 一併顯示精選層級、掃描揭露與來源（同 DISC-002 的欄位）；②goldenset 加一組「投毒文件」（關鍵詞堆疊、假任務例句），紅線：它不得進任何 golden 題的 Top-3，且不得在名稱／特定詞查詢裡取代正解；③`enrich` 提示明定「只能重述內容裡有的事」已在 v6（R-34 自檢），把自檢的「誇大」結果納入投毒訊號。**（2026-09-07 進度：①已落地；②量了兩種情境，紅線均未達（golden Top-3 最壞 32/60、公平 37/60），且 tags 格式詞數與例句離散度兩個候選訊號都分不開投毒與合法內容——結論轉為結構性緩解，見 `05` R-53；③試過的 overreach 自檢規則數字不成立已撤回。）**
**2026-09-07 裁定（`05` R-53）**：三條裁定已簽——①採納，目錄維持策展（僅人工審核可讓內容進入 `is_catalog` 工作區）是 OWASP LLM04 唯一夠格的結構性緩解，放寬准入的提案動工前必須重跑投毒量測，畫面揭露不算緩解（定案見 ADR-013 定案調整 8）；②採納，投毒量測列為常設紅線，往後改索引文本生成規則或檢索規則，連同 F1 一起重跑並把兩組數字寫進同一份報告，紅線維持嚴格版不放寬；③不採納（現在不做），對僅 `indexed` 層級加 Top-3 曝光上限或延遲策展信號，重啟條件是出現放寬准入提案且重量後紅線仍未過。SEC-013 投毒那一條的成立條件重新界定為「量測存在且結果入報告＋策展在 ADR-013 寫成唯一結構性緩解＋放寬准入提案前重量」；紅線本身不放寬，SEC-013 仍不勾（注入攻擊集紅線 0/N 未達，殘留 1/12）。

### LLM05 Improper Output Handling — 中

- **對應威脅**：TM-EXE-04、TM-IMP-02。
- **現有**：模型輸出一律結構化（strict JSON schema；`test_strict_schemas.py`）；Web 用 `<pre>`／文字節點渲染草稿與摘要，沒有 `dangerouslySetInnerHTML`；生成的檔案經 `skillpkg.Validate`（路徑、大小）才成版本；模型輸出從不在 API 行程執行（鐵律 1）。
- **缺口**：沒有一條測試專門證明「生成檔案的 `../` 路徑會被拒」是從創作路徑走到的（匯入路徑有 TM-IMP-02 的測試）。
- **SEC-013 要做**：一條 materialize 反證測試：草稿含 `files[].path="../x"` 與絕對路徑 → 422，不建版本。**（2026-09-07 進度：`TestCreationRefusesADraftThatEscapesItsPackage` 已落地。）**

### LLM06 Excessive Agency — 中

- **對應威脅**：TM-EXE-03、TM-CTL-01。
- **現有**：工具＝意圖、Go 執行、逐項 HITL（brief／參考／連網／試跑／保存／採用／查重全部要人）；`adopt_reference` 只能 fork Go 自己端出來的 id（`listedReference`）；試跑在 gVisor、egress default-deny；預算與步數上限（R-45）。
- **缺口**：無新缺口；保持「每個新工具意圖都要一個 HITL 停點」這條規則進 GEN 的允收（已在 R-47 的形狀裡，未寫成通則）。
- **SEC-013 要做**：`02` GEN-007 補一句通則：新工具意圖沒有停點不得進契約（純文案）。

### LLM07 System Prompt Leakage — 低

- **現有**：提示是程式碼（公開 repo），裡面沒有金鑰與租戶資料；金鑰只在閘道與短效 Virtual Key；`model`／`prompt_version` 不進 Web。
- **缺口**：無。

### LLM08 Vector and Embedding Weaknesses — 中

- **對應威脅**：TM-DAT-01、TM-TRC-03（跨租戶）；投毒歸 LLM04。
- **現有**：所有查詢帶 Workspace scope、公開查詢綁 `is_catalog`（鐵律 3、`TestClientSuppliedWorkspaceIDIsIgnored`）；創作的目錄查詢與查重都只看目錄，看不到別人的私有工作區；embedding 模型換了就全量重建（ADR-013）。
- **缺口**：使用者的查詢句與草稿描述會送到 embedding 供應商（見 LLM02 的同意書）；沒有其他。
- **SEC-013 要做**：併入 LLM02 的同意書項目。

### LLM09 Misinformation — 中

- **對應威脅**：TM-SCN-01。
- **現有**：模型文字一律標記來源（`summary_source`、`match_reason_source`）；「靜態檢查不代表試跑成功」；Judge 的信任邊界與再評估（ADR-026）；生成品不進搜尋（GEN-007）；參考的相容與工具標為「宣告」。
- **缺口**：無新缺口。

### LLM10 Unbounded Consumption — 中

- **對應威脅**：TM-EXE-02、TM-MDL-02。
- **現有**：每 Run 短效 Virtual Key 帶 `max_budget`／`tpm`／TTL；會話預算、步數、工具次數、訊息數、搜尋回合上限；連網 15 秒／256 KB；embedding 逾時；匿名搜尋與匯入有速率限制（`httpx/ratelimit.go`，NFR-001 第 5 條）；首則訊息查目錄與查重各一次 embedding，記在會話帳。
- **缺口**：平台級模型預算煞車仍未設計（TM-MDL-02 殘餘，`03` 無承接）。
- **SEC-013 要做**：把「平台級煞車」登成明確工作項（新決策，`05`）。

## 2. 優先序（嚴重度 × 缺口）

1. **LLM01 創作迴圈的注入攻擊集**（沒有數字＝不知道）。
2. **LLM04 投毒 × Re-Use**（採用按鈕把投毒變成一鍵行為；先補畫面上的層級與揭露）。
3. **LLM02 會話內容遮罩＋同意書**。
4. LLM03 模型釘版本（決策）。
5. LLM05／LLM06／LLM10 的三條小工作。

## 3. 維護

威脅模型 §3 的觸發條件同樣適用；另加一條：**OWASP 版本更新（目前 2025）或新的模型呼叫面上線時**，本表逐項重看。每個里程碑結束與威脅模型 §2.9 一起複審。

# 意圖搜尋驗證

## 判定與範圍

DISC-005 尚未完成。結構化分析、使用者修正與失敗降級已有實作及局部測試；中文詞法下界仍未達標，完整混合搜尋與前端端到端驗收也不能由這份量測取代。`04` 的丙-76 保持開放。

領域變更交接集中於 [Domain Memory 提案草稿](../domain-memory/changes/catalog-correctable-intent/draft-pr.md)，狀態仍是 draft；格式驗證通過不等於已審查或允收完成。scratchpad 的早期副本不再作為維護入口。

詞法量測刻意不提供 embedding，只代表 **embedding 不可用時的詞法降級路徑**；混合搜尋基線另以固定向量重播。兩者不能混算。測試 PASS 只證明量測執行及資料完整性，品質判定另看下表。

## 可重播輸入

- 題目：[`queries.json`](../../tools/goldenset/queries.json)，30 題中文任務、18 題英文任務，中英文各 6 題干擾查詢；正解採 `gold_primary` 與 `gold_acceptable` 聯集。
- 語料：同一批 31 份 Skill 的 [`corpus_enriched`](../../tools/goldenset/corpus_enriched/) 與 [`corpus_enriched_v7`](../../tools/goldenset/corpus_enriched_v7/)，分開量測，不混算。前者 JSON 實際記錄 `enrich-skill/v1`，不能僅憑歷史結果檔的 v2 名稱推定提示版本。
- 查詢分析：[`intent_analysis_v2.json`](../../tools/goldenset/intent_analysis_v2.json)，`search-intent/v2`、`gpt-5.6-luna` 一次固定取樣。60 筆合法結構，無呼叫失敗或成本缺漏，Gateway 回報合計 US$0.0181568，單筆最長 4.511 秒。這不是人工理解品質評分或 p95 延遲允收。
- 投毒對照：既有 [`poison`](mvp/m5/creation-measure/injection/poison/) 與 [`poison-enriched`](mvp/m5/creation-measure/injection/poison-enriched/) 的三份樣本；刻意模擬已進目錄的內容，不宣稱測過人工策展的辨識能力。

模型取樣使用獨立、只允許 Luna、24 小時有效、US$1 預算上限的 Virtual Key，每題最多一次、不自動重試；結束後撤銷成功。重播不使用金鑰、不呼叫模型。不得把服務提供的合法 JSON 當成語意正確的證據。

## 執行方式與判讀

以 Platform 的獨立測試資料庫執行，設定 `SKILLHUB_TEST_DATABASE_URL` 及 `SKILLHUB_REQUIRE_DB=1`。測試會重建測試庫，不可指向產品資料庫。

```text
go -C apps/platform test ./internal/entrypoint/api/apiserver -run "Test(GoldenLexicalBaseline|RecordedIntentAnalysis)" -count=1 -v
```

`TestGoldenLexicalBaselineUsesProductionProjectionAndQueries` 量原句；`TestRecordedIntentAnalysisUsesProductionValidationAndLexicalSearch` 重播固定分析。再設定 `SKILLHUB_INTENT_POISON=1` 執行同一命令，取得投毒對照。想比較新的模型輸出時，以原句基線測試搭配 `SKILLHUB_INTENT_SNAPSHOT` 指定另一份完整快照；不要改寫固定證據。

兩者都經真實 Catalog 投影與公開搜尋 Handler。每份語料使用獨立 Workspace，查詢只讀該 Workspace，帶入真實 frontmatter、摘要、標籤、例句及既有分類。測試核對投影數、正解存在、快照原句唯一、Go 接受分析狀態與無跨語料洩漏。

`Recall5` 是任務題在前五筆中至少命中一個正解的題數，不是逐文件 recall。`f1_at_gold`／`recall_at_gold` 則把回傳頁截在正解集合大小，逐題計算再取平均；干擾題只有空結果才算 1。`poison_top3` 是至少一份投毒文件出現在前三筆的查詢題數。不要混用這三種指標。

## 目前結果

### 原句混合搜尋基線

`TestGoldenHybridBaselineUsesRecordedVectorsAndProductionQueries` 使用固定 `text-embedding-3-small` 快取，對兩版增強語料經真實 Catalog 投影與 PostgreSQL 混合查詢重放。以模型名稱與完整文字的 SHA256 查找向量，缺漏、維度不符、非有限值或零向量直接失敗；逐題確認 embedding 被呼叫一次且回應沒有降級。這不是即時模型呼叫，也不驗證推薦理由。

```text
go -C apps/platform test ./internal/entrypoint/api/apiserver -run TestGoldenHybridBaselineUsesRecordedVectorsAndProductionQueries -count=1 -v
```

| 語言 | 任務 Top-1 | 任務 Top-3 | 干擾拒答 | F1 at gold |
| --- | --- | --- | --- | --- |
| 中文 | 28／30 | 30／30 | 6／6 | 0.9074 |
| 英文 | 16／18 | 18／18 | 6／6 | 0.8681 |

上表是原始增強語料、未加入投毒的原句基準；不能用混合搜尋的中文命中率宣稱詞法下界達標。強制產品改走詞法降級後，測試在首題因 `degraded=true` 失敗；還原後重跑通過。

歷史快取實際有 207 筆，涵蓋 60 個原句與 31 份原始增強索引文字；另將 v2 改寫查詢 60 筆、v7 索引 31 筆與投毒索引 3 筆補齊，保存於 [`intent_embeddings_v2.json`](../../tools/goldenset/intent_embeddings_v2.json)。這 94 份向量由現行 Python embedding 函式經 Gateway 分六批取回，Gateway 回報 US$0.00055458；使用只允許該 embedding 模型、24 小時有效、US$1 額度的獨立金鑰，無 SDK 重試，完成後撤銷成功。新快取與歷史快取不覆蓋彼此；模型別名相同不保證跨時間的供應商快照相同，這是跨批比較的限制。

### 原句與 v2 改寫的混合配對

同時執行上述原句測試及 `TestRecordedIntentAnalysisUsesProductionValidationAndHybridSearch`。兩者均可設定 `SKILLHUB_INTENT_POISON=1` 加入相同三份投毒文件，無須再呼叫模型。

| 增強語料 | 查詢輸入 | 中文 Top-1 | 英文 Top-1 | 中文／英文干擾拒答 | 中文／英文 F1 at gold |
| --- | --- | --- | --- | --- | --- |
| 原始增強 | 原句 | 28／30 | 16／18 | 6／6、6／6 | 0.9074、0.8681 |
| 原始增強 | v2 關鍵詞 | 25／30 | 15／18 | 5／6、6／6 | 0.8241、0.8333 |
| v7 | 原句 | 29／30 | 17／18 | 6／6、6／6 | 0.9352、0.8819 |
| v7 | v2 關鍵詞 | 24／30 | 15／18 | 5／6、6／6 | 0.8380、0.7708 |

加入投毒後，原始增強的原句／改寫分別有中文 25／22 題、英文 10／10 題出現投毒 Top-3；v7 分別為中文 24／21 題、英文 11／9 題。兩者都未達抗投毒紅線，投毒命中略減不能抵銷檢索品質退步。

**把關鍵詞串同時作為詞法與向量輸入的初版 v2 實作未通過品質驗收**。此表保留為拒絕該接線方式的證據；不能只因快照 JSON 合法或重播測試 PASS 就採用。

### 向量與詞法輸入分工實驗（未採用）

保留 v2 的分析、關鍵詞與篩選，僅讓自動分析的向量輸入使用原句；詞法仍使用關鍵詞串，使用者主動修正則仍以修正內容檢索。重播不新增付費呼叫。

未加入投毒時，兩版語料的 Top-1、F1 與干擾拒答全部回到上表原句基準。但加入投毒後，原始增強中文 Top-1 為 18／30、投毒 Top-3 為 26 題，v7 分別為 19／30、25 題；英文投毒 Top-3 分別為 10、11 題。中文比原句基準的 25、24 題更差，因此此實驗已還原，未成為正式搜尋規則。

這組結果把兩個影響分開：關鍵詞串作向量輸入會丟失完整句子的檢索效果；即使修復這一點，較短的關鍵詞仍可能讓投毒文件符合全部 token 覆蓋而提前。產品的 `hybridSearch` 將同一個字串交給候選查詢的 `Query`／`BigramQuery` 與名稱排序，詞法覆蓋標記同時影響候選准入與優先次序。下一個設計必須明確區分「模型建議用來擴充候選的詞」與「足以取得完整覆蓋優先權的使用者需求」，並驗證名稱／特定詞與修正情境，不能只更換 embedding 輸入。

### 現行修正：關鍵詞擴充與完整需求分工

自動分析後，原句仍用於向量、完整 bigram 覆蓋與名稱命中；模型關鍵詞用於 FTS 候選擴充，不能單憑它取得覆蓋優先權或豁免向量距離。使用者主動修正時，修正後五欄及關鍵詞組合才是新的需求。分析產生的合法篩選仍生效，原始查詢仍保留；embedding 失效時的詞法降級另外量測，沒有宣稱中文下界已達標。

兩版固定語料重播的乾淨品質、干擾拒答及投毒結果均回到原句基準：原始增強中文／英文 Top-1 為 28／30、16／18，v7 為 29／30、17／18，干擾均 12／12；投毒 Top-3 原始增強為中文 25、英文 10，v7 為中文 24、英文 11。這只表示消除了本次改寫造成的額外退步，**既有投毒紅線仍未通過**。

`TestModelKeywordsDoNotReplaceTaskEmbeddingOrGrantCoveragePriority` 以原句任務與模型建議的 `spreadsheet` 建立反例：向量仍須收到原句，僅符合該關鍵詞且向量距離過遠的文件不得進結果。暫改回關鍵詞 embedding 時，測試因輸入錯誤失敗；暫改用關鍵詞判定 bigram 覆蓋時，測試因多出不該准入的結果失敗。還原後此測試、使用者修正及改寫失敗降級測試通過。

`TestAnalyzedSearchPreservesExactNamesAndUserTokenCoverage` 在改寫成功且模型關鍵詞不同於原句時，驗證精確名稱優先於較近的覆蓋結果、中文完整詞組優先於向量結果，以及部分覆蓋不得豁免距離。分別把名稱判斷改看模型關鍵詞、bigram 改看模型關鍵詞、完整覆蓋 AND 改為 OR，對應案例均失敗；還原後與既有覆蓋測試合跑通過。測試使用專屬查詢詞並在結束時撤銷自己工作區的目錄標記，避免污染其他公開搜尋案例。

`TestModelKeywordsExpandCandidatesBeyondTheVectorWindow` 補足小型 golden 語料無法證明的候選擴充：建立 50 筆更近的向量結果與一筆符合模型關鍵詞、距離仍合法的較遠結果。分析失敗時只有前 50 筆；分析成功後第 51 筆經 FTS 找回，仍依距離排在最後。把 FTS 輸入改回原句，測試因結果僅有 50 筆而失敗；還原後通過。這證明模型關鍵詞確實參與檢索，不代表關鍵詞的理解品質已通過人工評估。

### 下界的歷史基準

DISC-005 ⑤的「中文 20%」可追溯至 [`results.txt` 的語言分組](../../tools/goldenset/results.txt)：中文 BM25 **Top-1＝6／30**，同一次英文 **Top-1＝14／18**。該腳本是字元 bigram＋自寫 BM25，並非當前 PostgreSQL FTS 的實測；因此原需求把它描述成目前 FTS 表現並不精確。數字仍是已寫下的品質基準，不能以當前較低的英文結果取代。

本量測器據此以 **Top-1** 比較固定的 14／18，並要求高於 20%；30 題中文至少須答對 24 題。它會先核對歷史資料的英文 BM25 列仍存在。先前以當輪英文 Top-5 比較的判讀不作為允收依據；下表保留 Top-5 供召回診斷，不能拿它判定 Top-1 過關。

| 分析與檢索 | 增強語料 | 中文任務命中@5 | 英文任務命中@5 | 中文／英文干擾拒答 |
| --- | --- | --- | --- | --- |
| v2 分析＋現行全 token 匹配 | 原始增強 | 1／30 | 5／18 | 6／6、6／6 |
| v2 分析＋現行全 token 匹配 | v7 | 2／30 | 5／18 | 6／6、6／6 |
| v2 分析＋詞組內 AND、詞組間 OR（未採用） | 原始增強 | 20／30 | 17／18 | 6／6、6／6 |
| v2 分析＋詞組內 AND、詞組間 OR（未採用） | v7 | 16／30 | 15／18 | 5／6、6／6 |

對應中文 Top-1，現行原始增強／v7 分別是 1／30、2／30，詞組實驗分別是 15／30、11／30，均未達固定的歷史英文基準。詞組實驗也造成一題額外誤命中，不能據此清除殘項。

相同 v2 快照加入三份投毒樣本後，兩份語料都得到以下 Top-3 風險：

| 詞法規則 | 中文投毒命中題數 | 英文投毒命中題數 |
| --- | --- | --- |
| 現行全 token 匹配 | 7 | 5 |
| 詞組內 AND、詞組間 OR（未採用） | 23 | 14 |

這不滿足最壞情況下投毒不得進 Top-3 的紅線。實驗性詞組匹配已還原；人工策展仍是既有結構性防線，不能把它寫成排序本身抗投毒的證明。相關邊界見[意圖搜尋](../adr/README.md#意圖搜尋)。

## 還需要的證據

### bigram 正規化後的 trigram 候選（尚未採用）

直接將中文長句交給 trigram 會漏掉只有兩字重疊的詞。另一個探測沿用 `LexicalTokens` 的 ASCII 詞與中文雙字切分，將查詢轉成空白分隔的詞串；文件側使用實際投影 `bigram` 的 `tsvector_to_array`，再以 `pg_trgm.similarity` 比較兩邊。它沒有呼叫模型或向量，也沒有改動正式查詢；查詢分詞目前在探測腳本重現，尚未成為正式 Go／SQL 接線證據。

未設拒答門檻時，原句中文 Top-1 在原始增強／v7 為 24／30、25／30，v2 改寫為 28／30、24／30，但六題中文干擾查詢皆產生結果。探測再以查詢 trigram 的交集覆蓋比例至少 1／4 篩選候選；這個比例來自固定語料上的門檻探索，**不是獨立留出集的泛化保證**。

| 語料 | 查詢 | 中文 Top-1 | 中文干擾拒答 | 中文投毒 Top-3 題數 |
| --- | --- | --- | --- | --- |
| 原始增強、無投毒 | v2 改寫 | 28／30 | 6／6 | 0（未加入投毒） |
| v7、無投毒 | v2 改寫 | 24／30 | 6／6 | 0（未加入投毒） |
| 原始增強、加入三份投毒 | v2 改寫 | 15／30 | 6／6 | 24 |
| v7、加入三份投毒 | v2 改寫 | 9／30 | 6／6 | 25 |

相同 1／4 門檻作用於原句時，無投毒的原始增強／v7 為 23／30、25／30，干擾皆 6／6；因此也不能宣稱改寫與 embedding 同時失敗時的兩版語料都達到下界。英文路徑沒有決定採用此規則，不能把這批中文結果當成英文非回歸證據。

部署可行性已做最小確認：自架 PostgreSQL 與現有 PGlite 套件均可載入 `pg_trgm` 1.6；PGlite 的記憶體資料庫實測 `similarity('報告 美化', '報告 整理')` 為 0.33333334，執行後關閉，未更動現行 harness 或新增依賴。PostgreSQL 探測均在交易中建立擴充後回滾，查證沒有留下擴充。函式的字元切分與相似度語意見 [PostgreSQL 官方說明](https://www.postgresql.org/docs/current/pgtrgm.html)。

此候選首次達到**改寫後、無投毒**的中文固定下界，但未通過投毒紅線，也尚未經正式 Handler、完整篩選／分頁、實際效能與混合搜尋回歸驗證；不能據此選型結案或宣稱抗投毒。既有人工策展邊界不變，正式程式未採用此候選。

### 投毒量測的信任訊號邊界

以 `SKILLHUB_INTENT_POISON=1` 重建兩版各 34 筆的實際投影後，直接查證 `search_documents` 的既有標記：

| 樣本（兩版合計） | 筆數 | curated | verified_at 存在 | enrichment_status | listable |
| --- | --- | --- | --- | --- | --- |
| 正常 golden 文件 | 62 | false | true | enriched | true |
| 三份投毒文件 | 6 | false | true | enriched | true |

這些標記本身沒有區分兩組。`verified_at` 存在不能在本量測裡充當內容可信的證據；僅准入 `curated=true` 會同時清空正常語料，不能以全空結果宣稱抗投毒通過。不得依投毒檔名排除候選，也不得只替正常答案補上標記來製造成功數字。

量測器以 `CatalogWorkspaces` 指定自己的 Workspace，刻意模擬內容已被准入，並不量測人工策展是否會放行。正式公開搜尋的目錄准入與「投毒已入目錄後能否排除」是兩個不同問題。現行安全規則一方面要求檢索變更重測且投毒不得進 Top-3，另一方面只在放寬目錄准入時要求重設曝光防線；不能把此處的失敗寫成人工准入已失效，也不能用人工策展的存在把此處改判成功。若要新增內容審核或曝光資格，須先明確定義其權威來源、版本失效條件、對既有 indexed 文件的影響與相應安全決策，不能當成調整詞法分數直接加入。

### 已否決的替代探測

自架測試庫的可用擴充清單包含 `pg_trgm` 1.6，不包含 `pg_bigm` 或 `pgroonga`。在交易內安裝、以實際投影探測、隨後回滾，已確認沒有留下擴充或修改正式查詢。未設拒答門檻時，對完整索引文字使用 `word_similarity`／`similarity`，原始增強語料的中文 Top-1 最佳為 22／30，v7 最佳為 18／30；改成逐條任務例句取最大值也未達標。這只能否決本次具體用法，不能推論所有 trigram 或其他擴充都無效。

逐關鍵詞計算 `word_similarity` 的探測也未達標：平均分數在原始增強／v7 的中文 Top-1 為 21／30、16／30，取最弱關鍵詞分數則為 13／30、8／30。此探測沿用固定 v2 快照、真實投影與交易回滾，不呼叫模型；未設拒答門檻，亦未重跑投毒，因此不能採用為正式檢索或宣稱品質驗收通過。

另以 `search-intent/v3-experiment` 只把關鍵詞改為英文，五欄仍要求引用原句；正式提示沒有變更。相同 60 題的一次取樣有 58 題合法、2 題由現有驗證拒絕，Gateway 回報 US$0.0229604，獨立限額金鑰已撤銷。現行詞法路徑的中文 Top-1 為原始增強 1／30、v7 3／30，中英干擾題仍各拒答 6／6；加入投毒後兩份語料皆有中文 5 題、英文 4 題出現投毒 Top-3。這個提示變體未採用，也不能取代中文詞法選型的明文決定。

下一個檢索設計必須同時重跑固定品質與投毒對照，並確認名稱／特定詞與混合向量搜尋沒有回歸。v2 的「不要按任務主題擅加分類」已在這 60 題取樣中維持空篩選，但仍需含明確篩選要求的正例、重複取樣及人工意圖欄位判讀；不能推論為所有輸入都正確。完整前後端 E2E、其餘允收、文件對帳與完整 SHA 遠端 CI 仍是結案條件。

## 匿名呼叫的成本邊界：尚未驗收

費用記帳、請求限流、呼叫逾時與花費前的預算阻擋是不同保證。`recordVersionedCallCost` 在呼叫後寫入成本事件；`modelbudget` 的 endpoint 設定管理逾時，不是金額預留。兩者不能證明匿名搜尋不會超支。

現有服務金鑰路徑有兩種不同的保證：

| 路徑 | 已存在的控制 | 尚缺的證據 |
| --- | --- | --- |
| 啟動器新簽 Virtual Key | `with-service-key.mjs` 驗證預算為有限正數且不超過 US$20，預設 US$1；`mintServiceKey` 傳送 `max_budget`、24 小時效期與模型清單 | Gateway 實際額度耗盡後的拒絕，以及搜尋端如何降級；重啟會新簽金鑰，因此單把金鑰上限不是跨重啟的每日總額 |
| 操作者提供不同於管理金鑰的既有 key | `serviceKeyPlan` 保留設定；啟動前以該 key 查 `/key/info`，確認有限正預算與未耗盡的已用額度 | 有效期、模型範圍與 Gateway 後續阻擋仍需驗證；不能沿用新簽路徑的 US$1 宣稱 |

Repo 的 Gateway 設定另有 US$50／1d 設定，但同時設置 `disable_budget_reservation: true`。設定檔不是執行期拒絕證據；尤其未預留執行中請求時，已記帳金額的檢查不能直接當成並行請求下的絕對支出上限。這也不證明部署者使用的其他 Gateway 採用相同設定。

啟動器對新簽與既有金鑰均先查 `/key/info`，以金鑰自己的 Bearer 授權，不將金鑰放進 URL。預算缺漏、非數字、非有限、非正數，或已用額度不可讀、負數、達到上限，均拒絕啟動；傳輸與 JSON 錯誤只顯示固定訊息，不轉印可能包含秘密的原錯誤。這是啟動前置條件，不是每次搜尋的金額預留；直接以其他入口啟動 Python 不經過這道檢查。

釘選 Node 下的 `task test:cleanmode` 為 32 項通過、零跳過、exit 0；其中服務金鑰測試 22 項，包含啟動器黑箱驗證：無上限金鑰不能啟動子程序，有額度才可啟動。把耗盡判斷由 `>=` 改為 `>`、把零花費誤擋、轉印原始錯誤、跳過啟動前查證，對應測試皆失敗，還原後全組通過。這組測試本身不是 Gateway 的實際阻擋證據，不能當成完整成本允收。

真實開發 Gateway 的管理面亦已查證：獨立 Luna-only、US$1、24 小時金鑰能通過相同 `verifyServiceKeyBudget`，撤銷後相同查證失敗；另一把 Luna-only、5 分鐘金鑰在建立時設定 `max_budget=1`、`spend=1`，由 `/key/info` 讀回兩值後，啟動前檢查確實拒絕。兩把金鑰的撤銷均 HTTP 200，探測 exit 0，模型請求數為零。第二把的已用額度是**人造測試狀態，不是供應商計費**；這組管理面探測只證明現場回應形狀與啟動守門相容。

模型端點另以獨立 Luna-only、5 分鐘、`max_budget=spend=1` 的金鑰驗證：先讀回額度，再向 `/v1/chat/completions` 送一次請求，輸出上限 1 token、不重試，得到 **HTTP 429 且錯誤明確包含 budget 拒絕**，沒有 `x-litellm-response-cost`。再用相同耗盡狀態，經真實 Python FastAPI 路由及 SDK 呼叫分析與 embedding，兩者均回 **502／`gateway error`**，沒有成功分析或假向量；Python 的路由由 TestClient 執行，Gateway 是實際容器。測試金鑰撤銷 HTTP 200，探測 exit 0。這證明「已記帳額度達上限」的拒絕與能力層傳遞，不把缺少成本 header 當作零費用讀數，也不證明尚未記帳的並行呼叫已被預留。它仍不是前端到真實部署的完整 E2E。

## Go／Python 的 HTTP 整合

`TestAnonymousSearchTraversesGoPythonAndGateway` 經公開匿名搜尋 API、真實 PostgreSQL、Go 的 LLM client、獨立啟動的 Python FastAPI 與 OpenAI SDK，再到本機 Gateway 替身。沿用既有服務啟動器；需設定 `SKILLHUB_CREATION_PYTHON` 為 Python 的絕對路徑，並設定 `SKILLHUB_REQUIRE_CREATION_PYTHON=1`、`SKILLHUB_REQUIRE_DB=1`，避免缺依賴時跳過卻報綠。仍須使用獨立 `_test` 資料庫。

```text
go -C apps/platform test ./internal/entrypoint/api/apiserver -run TestAnonymousSearchTraversesGoPythonAndGateway -count=1 -v
```

四個案例全部通過、零跳過、exit 0：完整分析保留五欄與提示版本；截斷回應被拒收但仍以原句找到向量結果；Gateway 以 429 拒絕分析時仍嘗試向量；分析與 embedding 同時不可用時明示降級。每案斷言分析及 embedding 各一次、沒有重試，且 embedding 收到原句而非模型關鍵詞。測試各自建立目錄資料，結束時撤銷自己的目錄標記與關閉 Python／替身服務。

四個案例分別以突變證明：移除 Python 的截斷檢查，回應錯成 `analyzed`；讓 Go 在分析失敗時跳過 embedding，向量案例錯成降級空結果；移除 embedding 失敗的降級標記，雙失效案例錯成未降級；把語意輸入改成模型關鍵詞，完整分析案例收到 `ledger` 而非原句。各次均 exit 1，還原後四案再次通過；兩份產品原始碼的雜湊與突變前一致。

同一組測試亦查證 Python 接收 Gateway 替身的成本 header 後，Go 寫入的意圖成本事件：完整分析與截斷回應各一筆，保留 10／20 tokens、1000 USD micros、Gateway 來源、提示版本與匿名範圍；分析遭拒且沒有用量回報時不得捏造成本事件。每案使用獨立模型識別，避免前案紀錄讓後案假綠。把 Go adapter 的意圖用量轉接改成 nil，完整與截斷案例皆因金額、tokens 與來源遺失而失敗；還原後產品檔雜湊與突變前一致。這證明跨語言成本傳遞，不代表 Gateway 真實計費已量到，也不把拒絕後無紀錄解讀成供應商零費用。

這是免費的跨程序接線證據，**不是模型理解品質、真實 LiteLLM 預算耗盡或整套瀏覽器部署 E2E**。429 由替身主動回傳，不能證明部署中的預算會觸發它；目前產品將該分析失敗表達為 `unavailable`，也未驗收成專屬的預算耗盡提示。

## 真實 Gateway 的匿名搜尋接線

`TestARealGatewaySearchRecordsIntentAndEmbeddingCosts` 需明確設定 `SKILLHUB_E2E_LLM_URL` 與對應 `LLM_SERVICE_TOKEN`，未設定時明示跳過；不可將此選項套到整套測試，因為其他付費案例也共用它。資料庫仍須為獨立 `_test` 庫，設定 `SKILLHUB_REQUIRE_DB=1` 防止資料庫缺席被誤當驗收。

```text
go -C apps/platform test ./internal/entrypoint/api/apiserver -run "^TestARealGatewaySearchRecordsIntentAndEmbeddingCosts$" -count=1 -v -timeout 90s
```

探測只啟動自己的 Python 程序，以 Luna 與 embedding 模型限定、US$1、10 分鐘的獨立 Virtual Key 執行一次，不自動重試。先取得查詢的真實向量，放入合成目錄文件，再經匿名 Go 搜尋 API 驗證原句保留、五欄引用或 null、提示版本、非降級回應及該文件出現在結果中。這個刻意匹配的向量只測接線，不是自然語料召回或理解品質。

一次執行 PASS、零跳過、exit 0；搜尋新增的匿名 Gateway 成本事件各一筆，意圖分析 226 USD micros、embedding 1 USD micro。這是帳本的整數微美元值，不是整個探測的總費用：準備向量的呼叫及可選推薦理由未包含在這兩筆合計。Python 程序結束，金鑰撤銷 HTTP 200。這仍未包含真實瀏覽器連到這套服務的部署 E2E。

獨立突變反證亦經真實 Gateway 執行：暫將 Go adapter 的意圖用量轉接改為 nil，測試因缺少正額、Gateway 來源的匿名意圖成本事件而 FAIL、exit 1；還原後重跑 PASS、零跳過、exit 0，產品檔雜湊回到突變前。兩次各用一把獨立 US$1、10 分鐘、限定 Luna 與 embedding 模型的金鑰，均撤銷 HTTP 200，不自動重試。這證明新增測試能抓到跨程序成本遺失，不代表其每項斷言均已逐一做過突變。

## 瀏覽器中的修正與網址生命週期

### 真實服務旅程

Chromium 已以獨立前端建置、本機同源反向代理、真正的 API 執行檔、Python、Gateway 與獨立 PostgreSQL 測試庫完成一次「原句搜尋 → 將輸出修正為 CSV → 重新載入」。沒有攔截或替換 API 回應；確認初始分析為 `analyzed` 且未降級、畫面五欄存在，修正送出 POST，Go 回傳 `corrected` 與 CSV，重新載入後原句與 CSV 均保留。執行 exit 0，並查看重新載入後的完整頁面截圖。

此旅程用一把 US$1、10 分鐘、限定 Luna 與 embedding 模型的金鑰，不自動重試；結束後 API、Python、瀏覽器及代理均停止，金鑰撤銷 HTTP 200。本次目錄沒有命中，不能宣稱已驗證真實結果卡片、分類篩選、所有失敗分支、三引擎或正式 TLS／部署代理；這些範圍與下面的三引擎替身驗證分開閱讀。即時模型費用未在此瀏覽器探測中彙總，不以預算上限當成實際花費。

有結果的同一路徑另以先前 Go 探測建立的合成文件驗證：先確認測試庫中唯一具名文件、Workspace 與已存向量，只在該次探測暫時開放該 Workspace 的目錄範圍。初始真實 API 結果包含指定 Skill，Chromium 斷言具名卡片連結可見，並查看包含來源、風險、相容性與相似度的完整頁面截圖；CSV 修正及重新載入亦通過，exit 0。結束後查詢確認目錄標記已恢復 false，獨立限額金鑰撤銷 HTTP 200。合成文件與查詢使用刻意配對的向量，這是非空結果呈現的整合證據，不是自然語料品質、人工策展或抗投毒證據；不據此放寬產品的曝光資格。

### 三引擎的決定性 API 替身旅程

[`intent-search.spec.ts`](../../apps/web/e2e/intent-search.spec.ts) 使用真實建置的前端與固定 API 回應，在 Chromium、Firefox、WebKit 各執行七個案例，共 21 個通過、零跳過。涵蓋五欄與「未提及」、保留原句、修正後重新載入、CSV／PDF 修正的快取隔離、捨棄理解並清除篩選，以及新任務不沿用舊修正。這是瀏覽器與前端接線證據，不等於連接真實 Go／Python／Gateway 的整套部署驗收。

畸形修正網址的六類輸入為未完成 JSON、數字、布林、陣列、空物件與 null。全部必須顯示中文錯誤，且不發搜尋請求；重新輸入任務後才恢復搜尋。路由原本把解析後的非字串修正丟掉，會誤走一般搜尋；現在保留它的 JSON 表示，交由共用修正驗證器拒絕。移除這個修正，五個非字串案例失敗；移除錯誤提示，未完成 JSON 案例失敗。

另將快取鍵中的修正內容改為固定值，正常旅程在 CSV 改成 PDF 時因畫面仍是舊結果而失敗。還原全部突變並重新建置後，三引擎 21 個案例再次通過。測試建置位於獨立暫存目錄，不覆寫 API 可能正在服務的 `apps/web/dist`。

## 本機整體回歸範圍

`task test:platform` 在獨立測試庫、強制資料庫與真實 Python 依賴下完成：**2884 通過、14 跳過、0 失敗，exit 0**。四種 Go／Python 搜尋情境及新增的成本傳遞斷言包含在內，不是只跑單檔；14 個跳過涵蓋需另行提供語料、真實物件儲存、Sandbox、live fetch 與付費模型的驗收，沒有把它們算成通過。相較先前的 13 個跳過，多出的是新增的真實 Gateway 搜尋測試；它保持付費 opt-in，成功與突變證據見上節。跨套件重建 schema 由既有資料庫鎖序列化，等待鎖不能誤判為服務失效。

相同工作樹的 `task test:web` 為 40 個檔案、712 個測試通過、零跳過，`task test:llm` 為 366 通過、6 跳過，兩者 exit 0。Python 的跳過包含兩個沒有 schema model 的 liveness 契約案例與四個未啟用的付費 Gateway 案例。`task format:check:platform` 亦 exit 0；這些檢查沒有改動曝光規則，也不代表中文詞法品質或投毒紅線已通過。

同一工作樹的 `task gen:check` 確認 sqlc、TypeScript、Python、Go 四組產物均為 current；`automation-check` 完成 50 個 task 的文件、Agent 與生成所有權檢查。這些是本機證據，尚未 Commit／Push，也沒有該提交全部遠端 CI workflow 成功的證據。

前端 `format:check` 明確回報 `All matched files use Prettier code style!`，`typecheck` exit 0；`task lint:llm` 回報 `All checks passed!`。`task lint:web` exit 0 但仍列出 React 與 TypeScript 警告，不宣稱零警告。生成檢查沒有手改產物，亦不會解除尚未通過的檢索品質允收。

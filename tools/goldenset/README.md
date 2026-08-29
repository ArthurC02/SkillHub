# tools/goldenset

## 凍結量測證據

`results.txt`、`results_v2_enriched.txt` 與其對應 cache 是 M1 的凍結量測證據，不是可隨意清理的
暫存輸出。若要重跑，必須以新的版本化結果另存並在報告記錄輸入、模型與方法；不得覆寫既有結果。

**權威資料在 DB／物件儲存，此處為種子、管線與證據快照。** 這裡是 **M1 的評測材料**：DISC-001 檢索品質的 golden query set 與它的語料、腳本、量測輸出。報告本體是 [`docs/plans/mvp/m1/golden-query-set.md`](../../docs/plans/mvp/m1/golden-query-set.md)。

以下逐檔歸類。同一列是一條管線。

| 輸入 | 腳本 | 產出快照 |
| --- | --- | --- |
| [`corpus/`](corpus/)（31 份釘住的 `SKILL.md`，分 `data`／`documents`／`writing` 三類）＋ [`manifest.json`](manifest.json)（來源 repo、pin commit、抓取時間） | [`enrich_corpus.py`](enrich_corpus.py)（逐份呼叫平台自己的 `POST /v1/enrich-skill`，量的是 production prompt 而非重寫版；已有輸出者跳過，輸出本身即快取） | [`corpus_enriched/`](corpus_enriched/)（31 份 JSON，逐份帶回報的 model id 與 prompt 版本） |
| [`queries.json`](queries.json)（60 題 golden query ＋ 干擾查詢與 gold 判定）＋ `corpus/`／`corpus_enriched/` | [`evaluate.py`](evaluate.py)（向量腿／BM25 腿／RRF 各自的 Top-1、Top-3、recall@5，干擾查詢的相似度分布與 cosine 門檻掃描） | [`results.txt`](results.txt)（未增強索引）、[`results_v2_enriched.txt`](results_v2_enriched.txt)（v2 增強索引，`recall@5 = 48/48` 的那一份）；[`embeddings_cache.json`](embeddings_cache.json) 是 embedding 快取（153 筆，key 是文字的 sha256，value 是 1536 個 float；**沒有語料原文、沒有任何秘密**）。**它入庫了**（2026-08-29 起 `.gitignore` 明確排除自己的排除規則）——在此之前 README 說「留著是為了重跑不必再付費」而 `.gitignore` 把它丟掉，clone 的人只能付費重跑，而付費也重現不出同一組數字（`text-embedding-3-small` 是會被供應商換 snapshot 的名字） |

## 與 M1 驗證閘門的關係

`results_v2_enriched.txt` 的數字就是 [`docs/plans/mvp/gate-test/README.md` §3.1](../../docs/plans/mvp/gate-test/README.md) 「量化前置（檢索品質）」那一列判定為 ✅ 的依據。因此：

- **`corpus/`、`queries.json`、`manifest.json` 屬閘門凍結標的的上游**——改動它們會讓 §3.1 記錄的 recall 數字與現況不符，D 日之後動了就必須依 §3.2 分開統計並在分析報告中明列。
- 閘門測試本身**不量 recall**（它量的是真人會怎麼打字、看到結果敢不敢往下走）。兩者測的不是同一件事，這裡的數字取代不了那場測試，反之亦然。

`evaluate.py` 與 `enrich_corpus.py` 是**驗證工具，不是產品程式碼**：不被服務引用，重跑要花真實的模型費用。`__pycache__/` 是本機執行的副產物。

**CI 守著的是哪一半（2026-08-29 修訂）**：重跑本身仍然不進 CI（要花錢）。但 `evaluate.py` 的
`enriched_index_text()` 是 `apps/platform/internal/skill/admission/enrich.go` 的 `embeddingText` 的**手抄副本**，
而 `--selfcheck` 只證明「Python 沒有自己改自己」，完全不比對 Go——改 `embeddingText` 從前不會讓任何東西變紅，
上面那個 recall 數字就會安靜地變成一個關於沒有人在跑的程式的數字。現在 `devctl automation-check` 的
`goldenset-mirror` 檢查把兩邊的函式本體（去註解後）各釘一個 digest：**任一邊改動就會紅**，
訊息要求先讀過兩邊、確認仍然產生同一個字串，再重釘。它擋不住「改對了」與「改壞了」的分別，
擋得住的是**改了而沒有人知道**——而那正是唯一發生過的失效。

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

## 名稱與特定詞查詢集（2026-09-06）

60 題 golden set 量的是「一句任務描述」；它量不到另一種真人：記得 Skill 名字、或只記得一個關鍵詞
就來查的人。05 R-48（公開搜尋）與 R-49／R-50（創作工具查目錄、建立前查重）都要扛這種查詢，兩條規則
過去只在 `docs/plans/mvp/m5/creation-measure/search-f1/`（真 Postgres＋真 `apps/llm`，一次性腳本）量過；
這裡把兩組查詢集併進 `evaluate.py`，讓它們可以用同一顆 embedding cache 反覆重跑，不必每次都起容器。

**兩組查詢**（`lookup_sets(docs)`，演算法抄自 `search_f1_public.py`，讀 `--index-mode enriched` 的語料）：

- **names**：每份語料 frontmatter 的 `name` 當查詢，正解＝該份語料。31 題。
- **tokens**：每份語料的 enriched 索引文本分詞後，取「全語料只出現一次、長得像識別字
  （`[a-z][a-z0-9+.#_-]{3,}`）」的 token 裡排序後第一個，當作「只記得一個關鍵詞」的查詢；正解＝
  該份語料。每份語料最多貢獻一題，全體再取前 25 題（依 `data`／`documents`／`writing`、類別內按
  id 排序，與 `search_f1_public.py` 的語料走訪順序一致，否則 25 題的截斷點會兩邊對不上）。

**兩條規則**（`--lookup`，用 `embed()` 拿向量，不連 Postgres——「覆蓋」在這裡就是查詢的每個 token
都在該份語料 tokenize 後的 enriched 索引文本集合裡，是 pg_bigm 真實查詢的近似值，不是它本身，兩者
的 tokens 分數不會、也不需要對上 `results-public-rule-2026-09-06.txt` 那份用真 Postgres 量出的數字）：

- **公開規則**（05 R-48）：覆蓋全部 token 的文件排最前面，其次是向量距離 <= 0.75 的候選（依距離），
  名稱完全命中的文件置頂。
- **創作規則**（05 R-49／R-50）：向量距離 <= 0.55 的候選（依距離），再補收一筆覆蓋全部 token、
  尚未在候選裡的文件。

**怎麼跑**：

```bash
cd tools/goldenset
PYTHONIOENCODING=utf-8 python evaluate.py --lookup > results_lookup_YYYY-MM-DD.txt
```

沒有可用的 embedding 金鑰、且 cache 裡沒有需要的向量時，`--lookup` 不會硬掛：它印出兩組查詢集
（連同各自的正解）與抓不到金鑰的確切錯誤訊息，不算分數、也不假造數字。

**紅線**（印在輸出結尾，人讀，不擋 CI）：

| 規則 | 指標 | 門檻 |
| --- | --- | --- |
| 公開 | names Top-1 | >= 90% |
| 公開 | tokens Top-1 | >= 80% |
| 公開 | golden Top-3 | >= 90% |
| 公開 | 干擾拒答@5 | >= 75% |
| 創作 | golden Top-1 | >= 85% |
| 創作 | names Top-1 | >= 90% |

**結果檔**：[`results_lookup_2026-09-06.txt`](results_lookup_2026-09-06.txt)——`--index-mode enriched`
語料、真 OpenAI embedding（部分向量原本就在 `embeddings_cache.json`，其餘現場付費補齊），六條紅線全數
PASS；tokens 在公開規則下是 25/25（見上段，方法論差異，非迴歸）。

## v7 增強語料（2026-09-07，不是 M1 的凍結證據）

[`corpus_enriched_v7/`](corpus_enriched_v7/) 是同一批 31 份語料用 `enrich-skill/v7` 重做的增強（`python enrich_corpus.py --url http://127.0.0.1:8001 --out corpus_enriched_v7`，服務要帶 `LLM_SERVICE_TOKEN`）。它**不取代** `corpus_enriched/`：M1 閘門的 recall 數字綁的是 v2，那份不動。v7 的分數由 [`creation-measure/search-f1/search_f1_score.py --docs corpus_enriched_v7`](../../docs/plans/mvp/m5/creation-measure/search-f1/search_f1_score.py) 產生（F1 定義寫在檔頭），結果在 [report §15](../../docs/plans/mvp/m5/creation-measure/report.md)：公開規則 all F1 0.927→0.955。`evaluate.py` 仍讀 `corpus_enriched/`。[`corpus_enriched_v7_1/`](corpus_enriched_v7_1/) 是試過而退回的 v7.1（all F1 0.937，report §15.5）。

**投毒量測**（05 SEC-013／R-53）不在這裡：三份投毒 `SKILL.md`、`enrich_poison.py`、`poison_dispersion.py` 與結果檔在 [`../../docs/plans/mvp/m5/creation-measure/injection/`](../../docs/plans/mvp/m5/creation-measure/injection/)，量的是「一份投毒文件擠不擠得進 golden 查詢的 Top-3」而不是這裡的 recall／F1 對照，見 [report §16.2](../../docs/plans/mvp/m5/creation-measure/report.md)。

# PDM-011 完整 golden query set 與向量距離門檻建議（M1）

- 日期：**2026-08-15**
- 對應：[ADR-013](../../../adr/ADR-013-intent-search-architecture.md)「待決策」第三項（golden query set 移交 M1，含跨 repo 抽樣 ≤ 20% 限制）、DISC-001／DISC-002、M1 驗證閘門
- 前身：[PDM-011 Spike 報告](../m0/pdm-011-spike-report.md)（12 份語料／10 條查詢，v2 已用真 Embedding 重測）
- 工具：[`tools/goldenset/`](../../../tools/goldenset/)（`evaluate.py`、`queries.json`、`corpus/`、`manifest.json`、`results.txt`）
- 狀態：**已執行，真 Embedding（`text-embedding-3-small`）量測完成。門檻為建議值，未回寫任何 ADR 或 `02`／`03`。**

> **本文件不定案、不修改既有文件。** 未動 `adr/`、`plans/mvp/02`、`plans/mvp/03`、`.github/`，亦未變更任何工作項目勾選狀態。語料目錄的存在**不是** CONTENT-003／004 的精選決定（見 §2 末段）。

---

## 1. 方法

| 項目 | 作法 |
| --- | --- |
| 語料 | **31 份真實公開 `SKILL.md`，橫跨 13 個來源 repo**，全部 pin 到 commit SHA（`manifest.json` 記錄來源 URL、commit、License、SHA-256） |
| 查詢 | **60 條**（每類別 20 條：繁中 12／英文 8；其中干擾查詢 4 條），人工標註 |
| 相關度分級 | `gold_primary`（人工判斷唯一最佳解）＋ `gold_acceptable`（也說得過去）。**命中＝兩者聯集**；另單獨量測嚴格 Top-1（只算 primary） |
| 向量腿 | OpenAI `text-embedding-3-small`（1536 維，PDM-003 定案型號）餘弦相似度 |
| 索引欄位 | `summary`＝`name: description`（frontmatter）。這是 ADR-013 §1「索引時 LLM 增強產出」的**代理**，非真實產出（見 §6 限制 1）。另跑 `fulltext` 對照 |
| 詞彙腿 | 自寫 BM25（k1=1.5、b=0.75，frontmatter 加權 3 倍），中文以字元 bigram 斷詞 |
| 融合 | RRF（k=60），**零命中的腿不參與融合**（沿用 Spike §9.5 的量測缺陷修正） |
| 查詢改寫 | **未做**。Spike 用人工英譯代表 LLM 改寫，是品質上界；本次刻意只測「原句直接進檢索」的降級路徑，因為門檻要保護的正是這條路徑 |
| 環境 | Python 3.14，純標準函式庫，無第三方依賴；金鑰只從環境變數或已 gitignore 的 repo 根目錄 `.env` 讀取，不進程式碼、快取、輸出或本報告 |

### 查詢設計

每類別 20 條的組成：

| 類型 | 條數／類別 | 說明 |
| --- | --- | --- |
| 直述任務 | 約 12 | 明確說出檔案格式與動作（「把這份 CSV 轉成每行一個物件的 JSONL」） |
| 口語模糊 | 約 4 | 不提任何技術詞（「這份報告看起來好陽春，幫我弄漂亮一點」） |
| 干擾查詢 | 4 | 與**整個語料**無關（訂機票、換正時皮帶、爵士樂推薦），gold 是「無結果」 |

跨語言涵蓋是**結構性的**：語料絕大多數是英文 `SKILL.md`，30 條繁中查詢全部是跨語言檢索；另有 3 份中文語料（`html-express`、`pitchcraft`、`data-analyst`）讓英文查詢也有跨語言案例。

**干擾查詢必須跨出整個語料，不能只跨出類別。** 檢索面對的是單一 31 份的候選池，不分類別；若把「清掉重複列」當作 `writing` 類的干擾題，它會正當地命中 `data` 類 Skill，量到的就不是「無結果」能力。因此 12 條干擾查詢全部是生活／機械／裝備類完全離題的問題。

### ADR-013 的跨 repo 抽樣限制

每個來源 repo 至多貢獻該類別 20% 題目（20 條 × 20% = 4 條）。`evaluate.py --selfcheck` 會硬檢查這一條，違反即失敗。實測全部通過：

| 類別 | 貢獻最多的 repo | 題數 | 該類別使用的 repo 數 |
| --- | --- | --- | --- |
| `documents` | `anthropics/skills`、`dashaworks/report-skills` | 各 4/20（20%） | 6 |
| `writing` | `anthropics/skills`、`harshaneel/humanize`、`jamditis/claude-skills-journalism` | 各 4/20（20%） | 5 |
| `data` | `YuYY2004/excel-skills`、`danielrosehill/Claude-Data-Wrangler-plugin` | 各 4/20（20%） | 6 |

**這條限制的實際代價**：`data` 類別原本被點名為同質風險最高者，但真正被限制卡住的是 `documents` 與 `writing`——[content-candidates.md](./content-candidates.md) 記錄的確認候選是 `documents` 4 個、`writing` 2 個，**全部來自 `anthropics/skills` 單一 repo**。若只用該清單出題，這兩類的 repo 占比是 100%，直接違反 ADR-013。為了讓量測成立，本次額外納入 9 個 repo 的語料（§2）——這同時說明 CONTENT-003 記錄的「`documents`／`writing` 供給缺口」不只影響目錄豐富度，**它會讓這兩類的 recall 量測本身失去鑑別力**。

---

## 2. 語料清單（31 份 / 13 repo）

完整來源 URL、pin 的 commit、SHA-256 見 [`tools/goldenset/manifest.json`](../../../tools/goldenset/manifest.json)。

### `documents`（10）

| Skill | 來源 repo | License |
| --- | --- | --- |
| `docx`、`pdf`、`pptx`、`xlsx` | `anthropics/skills` | Source-available（各目錄 `LICENSE.txt`） |
| `report-designer`、`deck-publisher` | `dashaworks/report-skills` | MIT |
| `html-express` | `zjp1997720/html-express` | MIT（簡中） |
| `document-design` | `jamditis/claude-skills-journalism` | MIT |
| `pitchcraft` | `moshuying/pitchcraft` | Apache-2.0 |
| `minimax-docx` | `MiniMax-AI/skills` | MIT |

### `writing`（9）

| Skill | 來源 repo | License |
| --- | --- | --- |
| `brand-guidelines`、`internal-comms` | `anthropics/skills` | Apache-2.0 |
| `doc-coauthoring` | `anthropics/skills` | **無 `LICENSE.txt`**（M0 發現 A；此處僅作檢索語料，不代表可精選） |
| `economist-style` | `TAJD/economist-style-guide-plugin` | MIT |
| `humanize`、`ai-check` | `harshaneel/humanize` | MIT |
| `prose-revision` | `Gabberflast/prose-revision-skill-...` | MIT |
| `newsroom-style`、`academic-writing` | `jamditis/claude-skills-journalism` | MIT |

### `data`（12）

| Skill | 來源 repo | License |
| --- | --- | --- |
| `data-analyst` | `nqumich/data-analyst-skill` | MIT（簡中） |
| `data-cleanliness-scan`、`csv-to-json`、`text-to-numeric`、`pii-flag`、`date-wrangling` | `danielrosehill/Claude-Data-Wrangler-plugin` | MIT |
| `excel-deduplicate`、`excel-filter`、`excel-merge` | `YuYY2004/excel-skills` | MIT |
| `high-stakes-analytics` | `limingrui679-design/high-stakes-analytics-decision-lab` | MIT |
| `data-journalism` | `jamditis/claude-skills-journalism` | MIT |
| `minimax-xlsx` | `MiniMax-AI/skills` | MIT |

**語料選擇刻意保留了真實的近義對**：`docx` vs `minimax-docx`、`xlsx` vs `minimax-xlsx`、`pptx` vs `pitchcraft` vs `deck-publisher`、`humanize` vs `ai-check`、`excel-deduplicate` vs `excel-filter`。這是真實目錄必然出現的情形，也是 Top-1 數字比 Spike 難看的主因（Spike 的 12 份語料幾乎沒有近義對）。

> **語料 ≠ 精選決定。** 本目錄只是檢索量測用的固定語料。§2 中除 `anthropics/skills` 外的 9 個 repo **未經** PDM-002 九項精選檢查表、未經 CONTENT-004（License 人工確認）與 CONTENT-006（規格與靜態掃描）。要把其中任何一個轉為 indexed／curated，必須另外走完回溯准入流程。

---

## 3. 命中率

命中判定＝`gold_primary ∪ gold_acceptable` 出現在前 k 名。48 條有正解的查詢（60 − 12 條干擾）。

### 3.1 總表（索引欄位＝summary）

| 檢索腿 | Top-1 | Top-3 | recall@5 |
| --- | --- | --- | --- |
| **向量** | **37/48（77%）** | **46/48（96%）** | **46/48（96%）** |
| BM25 | 20/48（42%） | 23/48（48%） | 27/48（56%） |
| RRF（等權） | 26/48（54%） | 35/48（73%） | 41/48（85%） |

嚴格 Top-1（只算 `gold_primary`，向量腿）：全部 36/48（75%）、繁中 20/30（67%）、英文 16/18（89%）。

### 3.2 分類別（向量腿）

| 類別 | Top-1 | Top-3 | recall@5 |
| --- | --- | --- | --- |
| `documents` | 9/16（56%） | 14/16（88%） | **14/16（88%）** |
| `writing` | 14/16（88%） | 16/16（100%） | **16/16（100%）** |
| `data` | 14/16（88%） | 16/16（100%） | **16/16（100%）** |

### 3.3 分語言（向量腿）

| 語言 | Top-1 | Top-3 | recall@5 |
| --- | --- | --- | --- |
| 繁中（30 條，全部跨語言） | 21/30（70%） | 28/30（93%） | 28/30（93%） |
| 英文（18 條） | 16/18（89%） | 18/18（100%） | 18/18（100%） |

### 3.4 類別 × 語言（向量腿 recall@5 / Top-1）

| | 繁中 | 英文 |
| --- | --- | --- |
| `documents` | **8/10（80%）** / 3/10（30%） | 6/6（100%） / 6/6（100%） |
| `writing` | 10/10（100%） / 8/10（80%） | 6/6（100%） / 6/6（100%） |
| `data` | 10/10（100%） / 10/10（100%） | 6/6（100%） / 4/6（67%） |

### 3.5 索引粒度對照（向量腿，全部 48 條）

| 索引欄位 | Top-1 | Top-3 | recall@5 |
| --- | --- | --- | --- |
| `summary`（frontmatter） | **37/48（77%）** | **46/48（96%）** | **46/48（96%）** |
| `fulltext`（frontmatter＋內文，截斷 20000 字元） | 27/48（56%） | 37/48（77%） | 44/48（92%） |

語料放大到 31 份後，摘要索引的優勢**由 Spike 的「平手」變成明顯領先**（Top-1 +21 個百分點）。Spike §9.3 提醒的「fulltext 已被截斷、低估長文稀釋效應」在這裡得到證實：語料愈長、愈雜，全文索引愈稀釋。這是 ADR-013 實證調整 4「索引時摘要是必要項」的直接量化支持。

### 3.6 兩條未命中的查詢（向量腿 recall@5 miss）

| 查詢 | 正解 | 向量腿名次 | 診斷 |
| --- | --- | --- | --- |
| D02（繁中）「幫我從掃描的檔案裡抽出表格資料，再把好幾份合併成一份」 | `pdf` | 10 | 前五名被 `excel-merge`（0.460）、`data-cleanliness-scan`、`html-express`、`data-analyst`、`excel-filter` 占滿。查詢**沒說出「PDF」**，「合併成一份」的語意訊號壓過了「掃描」。同一條查詢在 Spike 語料（無任何試算表合併 Skill）中是 Top-1——**這是語料多樣化才暴露出來的真實失效**，不是標註錯誤：掃描檔就是 PDF／影像，`pdf` 仍是唯一正確答案 |
| D09（繁中）「下週要跟老闆做專案結案匯報，我不想只念流水帳」 | `pitchcraft` | 6 | `pitchcraft` 的 frontmatter 是英文（"Structured persuasion... not activity logs"），而查詢是高度口語的繁中。語意其實對得上（"activity logs"＝流水帳），但裸 frontmatter 的訊號量不足以排進前五 |

兩條 miss 全部落在 `documents` / 繁中 / 未明說格式的情境，且**兩條都指向同一個修補**：ADR-013 §1 的索引時「適用任務範例句」。D02 需要 `pdf` 的範例句寫出「從掃描件抽表格」，D09 需要 `pitchcraft` 有中文任務範例句。裸 frontmatter 補不了這個缺口。

### 3.7 對 ADR-013 實證調整 3（RRF）的再確認——負面訊號比 Spike 更強

Spike 的結論是「等權 RRF 無增益，異質腿下還會倒退一名」。查詢量放大 6 倍、語料放大 2.6 倍後，**倒退幅度大到不能再視為雜訊**：

| | 向量腿 | 等權 RRF | 差 |
| --- | --- | --- | --- |
| Top-1 | 37/48（77%） | 26/48（54%） | **−11 條** |
| Top-3 | 46/48（96%） | 35/48（73%） | **−11 條** |
| recall@5 | 46/48（96%） | 41/48（85%） | **−5 條** |

原因與 Spike 診斷一致並更明顯：BM25 腿在繁中查詢上 Top-1 只有 6/30（20%）、recall@5 11/30（37%），等權融合等於把一條半廢的腿的名次平均進強腿。

**建議（僅建議，不改 ADR）**：MVP 首發不要把等權 RRF 當預設排序器。混合檢索的價值維持 ADR-013 已定案的說法——**召回覆蓋**，而非排序增益；實作上應以「兩腿取聯集擴大候選集、再依向量分數排序」取代「以 RRF 分數排序」。若要保留 RRF，需先做依查詢語言／腿得分的動態調權，而 ADR-013 已明確把加權融合列為「後續優化、非 MVP 首發假設」。

---

## 4. 「無結果」門檻分析

### 4.1 兩個分布

| 分布 | n | min | p25 | 中位 | p75 | max |
| --- | --- | --- | --- | --- | --- | --- |
| **干擾查詢的最高相似度**（無門檻時會被回傳的第一名） | 12 | 0.098 | 0.136 | 0.156 | 0.157 | **0.188** |
| **正常查詢的 gold 相似度**（前五名中最佳正解的相似度） | 46 | **0.231** | 0.372 | 0.443 | 0.538 | 0.616 |
| 正常查詢的最高相似度 | 48 | 0.255 | 0.380 | 0.454 | 0.535 | 0.616 |

**兩個分布完全不重疊**：干擾上界 0.188 < 正解下界 0.231，中間有 0.043 的乾淨間隙。這是本次最乾脆的結果——在此語料規模下，單一絕對餘弦門檻可以同時做到 100% 拒答與 0% 召回損失。

干擾查詢逐條（誤命中對象）：

| 查詢 | 最高相似度 | 若無門檻會回傳 |
| --- | --- | --- |
| D20「pour-over coffee 研磨粗細」 | 0.188 | `pitchcraft` |
| W19「車子時速 60 有異音」 | 0.173 | `ai-check` |
| A19「三天健行的輕量帳篷」 | 0.166 | `pitchcraft` |
| A11「貓一直掉毛要看醫生嗎」 | 0.157 | `excel-filter` |
| W11「水龍頭滴水換墊片」 | 0.156 | `excel-merge` |
| W20「硬舉護腰姿勢」 | 0.156 | `high-stakes-analytics` |
| D12「跑步膝蓋痛復健」 | 0.153 | `excel-deduplicate` |
| D19「kubernetes ingress TLS」 | 0.144 | `pitchcraft` |
| D11「訂東京機票飯店」 | 0.136 | `html-express` |
| W12「通勤聽的爵士樂」 | 0.130 | `pitchcraft` |
| A12「台北開車到高雄要多久」 | 0.107 | `excel-deduplicate` |
| A20「2012 Civic 換正時皮帶」 | 0.098 | `pitchcraft` |

### 4.2 trade-off 曲線

| 餘弦相似度門檻 | 餘弦距離門檻（pgvector `<=>`） | 干擾正確回「無結果」 | 正常查詢召回損失 |
| --- | --- | --- | --- |
| 0.16 | 0.84 | 9/12（75%） | 0/46（0%） |
| 0.18 | 0.82 | 11/12（92%） | 0/46（0%） |
| **0.20** | **0.80** | **12/12（100%）** | **0/46（0%）** |
| **0.22** | **0.78** | **12/12（100%）** | **0/46（0%）** |
| 0.24 | 0.76 | 12/12（100%） | 1/46（2%） |
| 0.28 | 0.72 | 12/12（100%） | 1/46（2%） |
| 0.30 | 0.70 | 12/12（100%） | 3/46（7%） |
| 0.34 | 0.66 | 12/12（100%） | 9/46（20%） |
| 0.40 | 0.60 | 12/12（100%） | 16/46（35%） |
| 0.50 | 0.50 | 12/12（100%） | 31/46（67%） |

ADR-013 移交的目標「干擾 ≥ 75% 正確回無結果、召回損失 ≤ 5%」**達標，而且有大幅餘裕**：召回損失預算內可達到的最高拒答率是 100%（門檻 0.285 以下皆然）。

### 4.3 建議門檻

| 候選 | 餘弦相似度下限 | 餘弦距離上限 | 干擾拒答 | 召回損失 | 何時選它 |
| --- | --- | --- | --- | --- | --- |
| A（保守） | 0.16 | 0.84 | 75%（9/12） | 0% | 目錄規模快速擴張、寧可多回幾筆弱相關也不願誤殺時。剛好踩線達到 ADR-013 目標 |
| **B（建議）** | **0.21** | **0.79** | **100%（12/12）** | **0%** | 預設值。取兩分布間隙 0.188–0.231 的中點，兩側各留約 0.02 餘裕 |
| C（積極） | 0.28 | 0.72 | 100%（12/12） | 2%（1/46） | 若產品判定「寧可回無結果也不要弱相關結果」。犧牲的是 A18「我需要一份說得出處、別人可以重跑的分析」這類完全不含技術詞的口語查詢 |

**建議採 B：餘弦相似度 ≥ 0.21，等價於餘弦距離 ≤ 0.79。**

> pgvector 的 `<=>` 運算子回傳餘弦距離＝1 − 餘弦相似度，因此 SQL 端的條件是 `embedding <=> :q <= 0.79`。門檻只適用於 `text-embedding-3-small` 的 1536 維向量；換模型必須整組重測（ADR-013 已記錄「Embedding 模型更換需全量重建索引」）。

### 4.4 這個門檻會過期——三個必須重測的觸發條件

1. **語料規模**。干擾查詢的「最高相似度」是對 31 份文件取極大值的**次序統計量**：候選池愈大，隨機撞到高分的機會愈高，干擾分布會整體上移，而正解分布不會。B 的 0.02 餘裕在目錄成長一個數量級後很可能被吃掉。**建議：索引項目數每成長一個數量級就重跑一次本工具。**
2. **索引欄位換成真實的 LLM 增強產出**。本次量的是裸 frontmatter。ADR-013 §1 定案的索引欄位是 LLM 生成的白話摘要＋適用任務範例句，會**同時**推高正解相似度（查詢與範例句同構）與干擾相似度（摘要更長、更泛化）。門檻必須在 CONTENT-005／索引管線就緒後重新推導，**不可沿用本數字**。
3. **查詢改寫上線後**。本次刻意測的是「原句直接進向量檢索」的降級路徑。改寫後的查詢（英文、任務關鍵詞化）相似度分布不同，需要**兩套門檻或一套取聯集**——否則會出現「改寫成功時門檻過鬆、逾時降級時門檻過緊」。

### 4.5 一個結構性上限（本次未做，建議列入後續）

單一絕對門檻無法區分「查詢確實無解」與「查詢很難、最佳解分數天生就低」。A18（0.231）與 D20（0.188）之間只差 0.043，但前者是好查詢配弱描述、後者是離題查詢。**相對規則**（例如「保留 top-1，其餘保留到 top-1 × 0.75 為止」）能同時處理這兩種情形，且對語料規模不敏感——正是 §4.4-1 那個弱點的解法。建議在絕對門檻之上疊加，而不是取代：絕對門檻擋離題、相對門檻擋長尾。

---

## 5. 對 M1 驗證閘門的初判

M1 驗證閘門的敘述是「使用者能以自然語言找到相關 Skill」。閘門本身是**使用者測試**，本文件只提供其量化前置。

**初判：量化前置條件通過，但 `documents` 類別附一個必須先修的條件。**

| 判準 | 實測 | 判定 |
| --- | --- | --- |
| 各類別 recall@5 | `writing` 100%、`data` 100%、`documents` 88% | 通過 |
| 繁中跨語言 recall@5 | 93%（28/30） | 通過 |
| Top-3（使用者實際會看的範圍） | 96%（46/48） | 通過 |
| 「無結果」可實作性 | 干擾與正解分布不重疊，門檻 B 可做到 100% 拒答 / 0% 損失 | 通過 |
| Top-1 精準度 | 77%（嚴格 primary 75%）；`documents` 僅 56% | **條件通過** |
| `documents` / 繁中 | recall@5 80%、Top-1 30% | **未通過** |

**未通過的一項與其修補路徑**：`documents` / 繁中的 Top-1 只有 30%，兩條 recall@5 miss 全在此格。成因不是模型能力（同組英文查詢是 6/6 全中），而是三件事疊加：

1. 該類別語料含最多近義對（`docx`/`minimax-docx`、`pptx`/`pitchcraft`/`deck-publisher`），Top-1 天生難；使用者實際看 Top-3 時該格是 8/10。
2. `pitchcraft`（中文 Skill）與 `pdf` 的 frontmatter 都不涵蓋查詢會用的說法——**索引時 LLM 摘要與任務範例句尚未存在**。
3. 查詢改寫尚未上線，本次量的是降級路徑。

前兩項都由已定案的 ADR-013 §1 覆蓋。**建議：M1 驗證閘門的使用者測試排在 CONTENT-005（白話摘要）與索引時增強管線就緒之後**，否則測到的是缺工序的中間態；若要提前測，`documents` 類別的結論不具代表性。

---

## 6. 限制（哪些沒驗證到）

1. **索引時 LLM 增強仍是模擬**。索引欄位是裸 frontmatter，不是 ADR-013 §1 定案的「LLM 生成白話摘要＋適用任務範例句＋標籤」。所有數字（尤其門檻）都是**該工序缺席時的下界**。
2. **查詢改寫未做、符合原因未做**。ADR-013 §2 的改寫與 §3 的理由生成／潤飾都未呼叫 LLM。
3. **未經 LiteLLM 閘道**。本工具直接呼叫供應商 Embedding API。這對離線評測工具可接受，**但產品實作不得比照**（AGENTS.md 鐵律 8、ADR-017）。
4. **未量測延遲**。NFR-004（p95 < 2 秒）需要 pgvector 實際規模與改寫呼叫的端到端量測，仍待 ADR-018 基礎設施就緒。
5. **未驗證 DISC-002 的結構化篩選與可解釋排序**（來源層級、是否含 Script、驗證狀態）。
6. **人工標註的單點風險**。`gold_primary`／`gold_acceptable` 由單人標註、未經第二人複核。近義對（如 `docx` vs `minimax-docx`）的歸屬影響 Top-1 數字達數個百分點。
7. **統計意義**。每類別 16 條有正解的查詢，單條進退＝6.25 個百分點；門檻推導只有 12 條干擾查詢，`max` 是極值統計量，抖動大於中位數。本文件的百分比應讀作量級，不是精確值。
8. **語料的授權狀態未逐一人工確認**。License 欄取自 GitHub metadata 與各 skill 目錄，僅供追溯；CONTENT-004 未執行。

---

## 7. 重跑方式

```bash
cd tools/goldenset
python evaluate.py --selfcheck   # 檢索邏輯、門檻計算、查詢集一致性、20% 抽樣限制；無網路、無金鑰
python evaluate.py               # 完整評測（未快取的文字會呼叫 Embedding API）
python evaluate.py --no-api      # 只用快取重跑；缺任何一筆就明確報錯
```

- 無第三方依賴，Python 3.11+ 即可。
- 金鑰從環境變數 `OPENAI_API_KEY` 或 repo 根目錄 `.env`（已 gitignore）讀取，不寫入任何檔案。
- 向量快取 `tools/goldenset/embeddings_cache.json`（約 3.6 MB，只有文字雜湊與浮點陣列，已加入 `.gitignore`）。重跑不重複計費。
- 最後一次執行的完整輸出保存於 `tools/goldenset/results.txt`。
- 語料可從 `manifest.json` 的 `source_url`（已 pin commit）完整重取，`sha256` 可驗證未漂移。

---

## 8. 建議的 CI 接法（建議，本次未改任何 CI 設定）

分兩層，因為兩者的成本與失敗語意完全不同。

### 8.1 每次 PR：`--selfcheck`（零成本、零網路，建議設為必過）

`--selfcheck` 檢查的是**資料集本身的完整性**，不是檢索品質：查詢 ID 不重複、每類別 20 條、繁中 12 條、干擾 ≥ 4 條、所有 gold 都存在於語料、干擾查詢確實沒有 gold、**ADR-013 的 20% 跨 repo 抽樣限制未被違反**，外加檢索與門檻計算的最小斷言。它不需要金鑰、不需要網路、跑不到一秒。

適合掛在既有的 Python lint／test job 上；語料或查詢集被改動時（例如新增精選 Skill 卻忘了補題）會立刻紅燈。

### 8.2 定期或標記觸發：完整評測（需金鑰，建議不擋 PR）

完整跑需要 Embedding API 憑證，且結果會隨語料與模型漂移，**不適合當 PR 的必過條件**——它會把「檢索品質退步」與「有人動了語料」混成同一種紅燈。建議：

- 觸發時機：排程（每週）或對 `tools/goldenset/**`、索引管線相關路徑的 PR 加標籤觸發。
- 金鑰：走 CI secret；**依 AGENTS.md 鐵律 8，產品程式碼的模型呼叫必須經 LiteLLM 閘道，本工具是離線評測例外，接入 CI 時應改為呼叫閘道**而非直連供應商。
- 建議的紅線（低於即失敗，數值取本次實測留一級餘裕）：各類別 recall@5 ≥ 90%、整體 Top-3 ≥ 90%、干擾拒答率 ≥ 75% 且召回損失 ≤ 5%。
- 快取：`embeddings_cache.json` 以 CI cache 保存（key 用 `manifest.json` 的雜湊），語料沒動就完全不呼叫 API。
- 輸出：`results.txt` 存為 artifact；門檻曲線變動時人工複核。

### 8.3 中長期：搬到 Langfuse Datasets

ADR-013「待決策」已指出回歸節奏與 Judge 品質回歸可由 Langfuse Datasets/Evals 承載（ADR-017）。本工具的 `queries.json` 結構（query／gold_primary／gold_acceptable／category／lang／kind）可直接映射為 Langfuse Dataset item。**建議在索引管線與 LiteLLM 閘道就緒後遷移**，屆時本工具退回為「離線、無依賴的門檻推導器」，不再擔任回歸把關。

---

## 9. 摘要：可以直接被引用的三個結論

1. **`text-embedding-3-small` 的原句跨語言召回在 31 份／13 repo 的語料上仍然成立**：繁中原句對英文語料 recall@5 93%、Top-3 93%，不需要查詢改寫就已達可用水準。ADR-013 實證調整 1（向量腿是跨語言召回的承載者）在放大 6 倍的查詢集上重現。
2. **「無結果」門檻建議：餘弦相似度 ≥ 0.21（餘弦距離 ≤ 0.79）**，干擾查詢 100% 正確拒答、召回損失 0%。但這個數字綁定「裸 frontmatter 索引 ＋ 31 份語料 ＋ 無查詢改寫」，三者任一改變都必須重測（§4.4）。
3. **等權 RRF 在這個查詢量下明顯有害**（Top-1 −11 條、recall@5 −5 條），比 Spike 觀察到的「倒退一名」嚴重得多。ADR-013 實證調整 3 已寫明「RRF 只是召回覆蓋手段，不是排序品質來源」——建議實作時據此把排序權交給向量分數，BM25 只擴大候選集。

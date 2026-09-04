# Images

本目錄有兩種安全角色完全不同的映像：

| 目錄 | 用途 | 可否執行不受信任內容 |
| --- | --- | --- |
| [`devtools/`](devtools/) | ADR-030 的跨電腦開發環境；供 Dev Container、codegen 與本機檢查使用 | **不可**；它有編譯器、套件管理器與 Docker CLI |
| [`runtime-agent-sdk/`](runtime-agent-sdk/) | Sandbox 內真正執行 Skill 的受限 Runtime Image | 可以，但只能經 ADR-005／015 的隔離層 |

`devtools` 的 base 使用不可變 digest，語言與工具版本來自 `.node-version`、各 `go.mod`／`.python-version` 與 `tools/toolchain.yaml`。它內含 Docker daemon／iptables，供 Dev Container以 privileged DinD建立跨平台一致的 nested-generation namespace；其安全邊界見 `.devcontainer/README.md`。它不是部署產物，也不得被 `SKILLHUB_SANDBOX_IMAGE` 引用。

## Runtime Image 審核流水線（SBX-002）

本目錄放 Sandbox 執行用的 Runtime Image。目前只有一個：
[`runtime-agent-sdk/`](runtime-agent-sdk/)（Node.js 22 LTS ＋ pinned
`@anthropic-ai/claude-agent-sdk` ＋ python3 與目錄宣告的 Python 依賴），由
`apps/sandbox` 的 DockerProvider 以 `SKILLHUB_SANDBOX_IMAGE` 引用。

## 映像內容與理由（版本 `2026.08-3`）

| 內容 | 為什麼在裡面 |
| --- | --- |
| `node:22-bookworm-slim`（digest pin） | PDM-003／ADR-015 選定的 Agent SDK 執行環境 |
| `@anthropic-ai/claude-agent-sdk`（版本 pin） | 工作負載本體；版本即 `runtime_version`（I-05） |
| ~~`unzip`~~ ——**現為：不在裡面**（`2026.08-4`，2026-08-29 移除） | 原本的理由是「Skill 套件是 zip，解壓縮只能發生在沙箱內（鐵律 1）」。**現在解壓由 `run.mjs` 自帶的 ZIP 解析器做**，絕對路徑／`..` 路徑段／非普通檔的拒絕規則一併移進該解析器（`2026.08-7` 起函式名為 `provisionPackage`），因此本映像的 `apt-get install` 只剩 `python3 python3-pip`。理由與逐條拒絕規則見 [`runtime-agent-sdk/Dockerfile`](runtime-agent-sdk/Dockerfile) 的註解與 [`UPGRADES.md`](runtime-agent-sdk/UPGRADES.md) 的 `-4`／`-5`／`-7` 三節 |
| **`python3` ＋ 下表 17 個套件** | 見下節 |
| **不在裡面**：`npm`／`npx`／`corepack`／`pip` | 執行期不載入、沙箱無網路，套件管理器留在沙箱裡只是負債 |

### 為什麼加 python3（`2026.08-2`，2026-08-16）

M2 基準試跑的 trace 原文是 `/bin/bash: line 75: python3: command not found`，而
`tools/content/seed-skills.json` 記錄 45 個目錄 Skill 中 **33 個 `deps_runtime = python`**
（[content-baseline-report.md §6.3](../../docs/plans/mvp/m2/content-baseline-report.md)）。那 33 個
Run 多數仍然「成功」，但**執行的不是 Skill 帶的腳本，而是模型對腳本的轉譯**——同一個
Skill 在裝了 Python 的環境與這裡是兩種行為。

**沙箱沒有網路**（SBX-007：唯一到得了的位址是模型閘道），所以「使用者自己 `pip install`」
這條路不存在：**預裝是唯一途徑**。

預裝清單＝**45 個目錄 Skill 宣告的依賴集合，減去過不了准入門檻的**。前半是可以從資料
重新推導的規則，「看起來有用」不行，後者會讓映像無止境長大；後半見下一節，它是
`2026.08-3` 才補上的，而補上的原因是前半在 `2026.08-2` 就已經失效了。

**版本 pin 的取法**：每個直接依賴釘在**通得過 I-06 閘門的最新版**。第一次嘗試釘了一組
保守的舊版本，grype 在 `lxml`／`pypdf`／`pdfminer.six` 上抓到**可修的 High**——依 ADR-022
那沒有豁免路徑，所以往上釘而不是往下豁免。**間接依賴不釘**：可重現性的錨是 image digest
與隨它發佈的 SBOM（SBX-011），手寫 lock 檔會是第二份會漂移的答案。

**代價，明說**：映像由 806 MB 長到 1.24 GB（`2026.08-2`）再到 **1.35 GB**（`2026.08-3`）。
以及 **`pandas` 是 3.x**，而目錄裡的 Skill 多半寫於 2.x 年代——**已由 §13 全量重測回答**：
45 筆只觸發一則 `select_dtypes(include='object')` 的相容性告警（`data-analyst`），非錯誤。

## 依賴集：宣告、准入與拒收（`2026.08-3`，2026-08-16）

### 為什麼要有這一節

`2026.08-2` 的規則是一句話：**裝目錄 `deps` 欄位的聯集**。規則沒錯，錯的是它的輸入。

M2 全量基準（[content-baseline-report.md §13.4](../../docs/plans/mvp/m2/content-baseline-report.md)）
發現 4 個 Skill 的腳本 import 了 `deps` 沒宣告的套件。**逐套件靜態掃描 45 個 pin commit
套件樹後，實際短少的是 13 個 Skill、8 個套件**——§13.4 只看到 4 個，因為只有那 4 個在該批
Dataset 上真的走到了 `ModuleNotFoundError`；其餘的沒被那組資料觸發，不代表沒有缺。

根因不是映像，是 **`deps` 欄位當初是人工逐份讀 SKILL.md 抄出來的，而抄漏了**；映像忠實地
裝了那份漏抄的聯集。沒有任何機制會發現這件事：沙箱裡 `pip` 已刻意移除、又沒有網路，
唯一的顯現方式是一個 Run 撞到 `ModuleNotFoundError` 然後 **Agent 繞路成功**——四筆全部
判定「符合」，缺依賴這件事完全沒有進入任何判定。

處置分三段，三段缺一不可：

1. **修資料**：`tools/content/seed-skills.json` 的 `deps` 依靜態掃描重新推導（13 筆修正）。
2. **修規則**：聯集不再無條件全裝，加**准入門檻**；擋下的逐項具名在下方，不靜默丟棄。
3. **修流程**：CONTENT-003 策展檢查表加一條可機械驗證的準則，掃描器補
   `package-dependencies`／`undeclared-dependency` 兩個 finding（`apps/platform/internal/shared/skillpkg/deps.go`），
   使同一個錯誤下次由匯入時的掃描指出，而不是由半年後的一次基準試跑。

### 准入門檻

> **純 Python wheel；無編譯擴充；不需系統二進位檔。**

理由只有一個，但它壓過其他所有考量：**沙箱存在的目的就是解析不受信任的資料**，而 native
parser 正是隔離層在補償的那個記憶體不安全面。為了滿足一個 `deps` 條目而把渲染／OCR 堆疊
搬進來，等於用掉沙箱本身的價值去換一個 Skill 的一條支路。門檻是**套件的性質**，不是逐案
的判斷，所以它跟「裝目錄宣告的聯集」一樣可以被別人重新推導出同一個答案。

體積是次要理由，但不是不存在：被擋下的套件中最大的四個（`presidio-analyzer` 357 MB、
`pyarrow` 157 MB、`reportlab` 31 MB、`pypdfium2` 9 MB）合計約 **554 MB**，相當於現行映像
1.35 GB 的四成。相對地，納入的 8 個純 Python 套件合計約 94 MB。

### 納入（`2026.08-3` 新增 8 個，全部純 Python）

| 套件 | 版本 pin | 大小 | 哪些 Skill 宣告 | 原本的下場 |
| --- | --- | --- | --- | --- |
| `pycountry` | 26.2.16 | 23 MB | `add-iso3166`、`standardise-country-names` | §13.4 實測 `ModuleNotFoundError`，改用內嵌對照表 |
| `chardet` | 7.6.0 | 5 MB | `data-cleanliness-scan` | §13.4 實測 `ModuleNotFoundError`，改用內建編碼偵測 |
| `defusedxml` | 0.7.1 | 1 MB | `docx`、`pptx`、`xlsx` | **靜態掃描才發現**：三者的 `office/validate.py` 在模組載入時就 import 它，缺了整個驗證器 import 不起來 |
| `ftfy` | 6.3.1 | 5 MB | `unicode-consistency` | 靜態掃描發現 |
| `confusable-homoglyphs` | 3.3.1 | 2 MB | `unicode-consistency` | 靜態掃描發現 |
| `pytz` | 2026.3.post1 | 3 MB | `date-wrangling`（SKILL.md 標為選用） | 靜態掃描發現；選用也是宣告 |
| `phonenumbers` | 9.0.37 | 48 MB | `pii-flag` | 靜態掃描發現 |
| `python-stdnum` | 2.2 | 7 MB | `pii-flag` | 靜態掃描發現 |

`defusedxml` 這一列值得單獨看：它是**強化用**的 XML 解析器（擋 billion-laughs 那類），
拒收它會讓沙箱**更不安全**而不是更安全。它在 `2026.08-2` 缺席了整整一個版本，而三個
Skill 的基準都判「符合」——因為 Agent 繞過了那條驗證路徑。

既有 9 個（`2026.08-2` 起，版本不變）：`pandas` 3.0.5、`numpy` 2.4.6、`openpyxl` 3.1.5、
`lxml` 6.1.1、`python-dateutil` 2.9.0.post0、`python-docx` 1.2.0、`python-pptx` 1.0.2、
`pypdf` 6.16.1、`pdfplumber` 0.11.10。另有兩個**傳遞依賴**恰好滿足宣告：`pillow` 12.3.0
（`pptx`、`pdf` 宣告）與 `charset-normalizer` 3.5.1（`unicode-consistency` 宣告），
依「間接依賴不釘」原則不另行釘版，其存在由 SBOM 佐證。

### 拒收（逐項具名，含理由與後果）

| 套件 | 宣告者 | 擋在哪一條 | 後果（誠實敘述） |
| --- | --- | --- | --- |
| `pyarrow` | `add-iso3166`、`add-data-dictionary`、`data-comparability`、`data-cleanliness-scan` | 編譯擴充（Arrow C++），157 MB | **Parquet 讀寫不可用。** 四個 Skill 都把 Parquet 列為多種輸入格式之一，CSV／JSON／XLSX 路徑不受影響。這是 4 個 Skill、也是本次拒收清單中影響面最大的一項 |
| `reportlab` | `pdf` | C 擴充（`_rl_accel`）＋需字型資源，31 MB | PDF **生成**不可用；讀取／抽取／合併（`pypdf`＋`pdfplumber`＋`pillow`）可用 |
| `pypdfium2` | `pdf` | 綁 PDFium native，9 MB | 頁面點陣化不可用 |
| `pdf2image` | `pdf` | 需 `poppler-utils` 系統二進位檔 | 同上 |
| `pytesseract` | `pdf` | 需 `tesseract-ocr` 二進位檔＋語言資料 | **掃描件 OCR 不可用** |
| `presidio-analyzer`／`presidio-anonymizer` | `pii-flag` | 拉進 spaCy（native）＋**首次使用時才下載語言模型**，357 MB | PII **偵測引擎**不可用；可用的是模型自身判讀 ＋ `phonenumbers`／`python-stdnum` 格式驗證。沙箱無網路，模型下載這條路在執行期必然失敗，**裝了也不會動** |
| `pywin32`（`win32com`／`pythoncom`） | `document-format-skills` | Windows-only，且需要 Microsoft Office COM | **在 Linux 沙箱上永久不可能。** 該 Skill 的 `.doc`／`.wps` 轉檔支路不存在；核心 `.docx` 路徑只需 `python-docx`，可用 |
| npm 套件（`pdf-lib`、`pdfjs-dist`；`pptx`／`docx` 文字提及的 `pptxgenjs`、`react`、`sharp` 等） | `pdf`、`pptx`、`docx` | 映像不預裝任何第三方 npm 套件，且 `npm` 本身已移除 | 這些 SKILL.md 的 Node 支路一律不可用。**注意 `anthropics/skills` 的 SKILL.md 明文寫「已預裝，不要 `npm install`」**——那是對它原生環境的敘述，在這裡不成立 |

**拒收 ≠ 目錄說謊。** `seed-skills.json` 的 `deps` 現在**如實記錄套件宣告的全部**，包含被
擋下的；schema 已寫明該欄位是「套件說它要什麼」而非「映像有什麼」，被擋下的逐項在本表。
`pdf`／`pii-flag`／`document-format-skills` 三筆另在各自的 `notes` 寫了失去哪條路徑。

**尚未做的一件事，明說**：目錄面向使用者的「限制」欄位是 CONTENT-005 的索引時 LLM 增強
產物，**不得就地改寫**（`02:CONTENT-005`：`需修改` 的處置是重跑增強與重新索引）。因此
本批**只記錄建議、不執行**：`pdf`（渲染／OCR 支路）、`pii-flag`（偵測引擎）、
`document-format-skills`（`.doc`／`.wps` 支路）三筆的「限制」文字應在下一次 CONTENT-005
重跑時涵蓋上表後果欄。已列入 `03` 工作項。

### 為什麼不是「把門檻放寬、全部裝進來」

`presidio` 那一列自己回答了：它需要在執行期下載模型，而沙箱沒有網路——**裝進來也是壞的**，
只是壞在更晚、更難查的地方。`pytesseract`／`pdf2image` 同理，缺的是二進位檔不是 wheel。
剩下的 `pyarrow`／`reportlab`／`pypdfium2` 才是真正的取捨：**197 MB 與三組 native parser**，
換 `pyarrow` 的 4 個 Skill 的 Parquet 支援、加上 `pdf` 的生成與點陣化支路。門檻選擇不換。

**若之後有需求訊號，正確的做法是為特定 Skill 族群開第二個 Runtime Image，而不是把這一個
養胖**——`skill_runtime_compatibility` 的鍵本來就是 (Skill Version × Runtime Image)，
多一個映像是這個資料模型早就預期的事。

> ⚠️ **換映像＝ Agent 相容軸失效。** `skill_runtime_compatibility`（migration 0022）以
> **(Skill Version × Runtime Image)** 為鍵。`2026.08-1` 有 45 筆、`2026.08-2` 有 45 筆
> （全量重測，見 §12／§13），**`2026.08-3` 目前 0 筆**。部署改用 `2026.08-3` 之後，目錄對
> 這 45 筆會回到「未驗證」，直到有人在新映像上重跑基準。這是刻意的：沿用舊映像的結論
> 正是本節要修的那個問題。
>
> 本次升版的**依賴集只增不減**（8 個新增、0 個移除、既有 9 個版本不變），所以 `2026.08-2`
> 的 45 筆結論在 `2026.08-3` 上**幾乎確定仍成立**——但「幾乎確定」不是量測，0022 的鍵不
> 接受推論。升版證據走的是 [ADR-023](../../docs/adr/ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md)
> 的四項重驗（見 [`runtime-agent-sdk/UPGRADES.md`](runtime-agent-sdk/UPGRADES.md)），
> 那是**行為回歸**的關卡；相容軸回填是**目錄事實**，兩者不互相取代。全量重跑
> 45 筆約 $2.2（§13.6 實測），列為 `03` 工作項而非本批動作。

「**經審核**」不是「建得起來」。SEC-002 §4.5 對執行映像列了六項檢查，其中三項是
發佈時閘門（威脅模型 §5.4 閘門 D）：

| ID | 檢查 | 等級 | 由誰執行 |
| --- | --- | --- | --- |
| I-02 | Image 以 digest pin，非 tag | 阻擋 | `runtime-image` CI job 的第一步 |
| I-03 | 保存 SBOM／依賴清單與建置來源記錄 | 阻擋 | 同 job，syft 產 SPDX JSON，**以 attestation 隨 GHCR digest 保存**（SBX-011） |
| I-04 | 漏洞掃描已執行且結果在有效期內 | 阻擋 | 同 job，grype；**有效期 30 天**（ADR-022）；**每週 cron 重掃已發佈的 digest** |
| I-06 | 漏洞等級超過政策門檻 | 告警 | 同 job；**可修的 Critical／High 阻擋**（ADR-022） |

閘門 D 的「阻擋」不擋 Run 啟動，擋的是「這個 Image 可以發佈」。I-04 未通過 →
該 Image 不得發佈、亦不得被新 Run 引用。

## 流水線

[`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)，
只在 `infra/images/**` 或該 workflow 自身變更時觸發（path filter）——重建 Debian
base 再拉一次漏洞資料庫，每個 commit 都跑等於花好幾分鐘重新學到同一件事。

1. **digest 斷言（I-02）**：`FROM` 沒有 `@sha256:` 就直接 fail。放第一步是因為
   後面每一步都在描述某個 image，而 tag 解析出來的 image 明天可能不是同一個。
2. **build**。
3. **syft → SPDX JSON（I-03）**。
4. **grype → 完整報告**（table ＋ JSON），寫進 job summary。
5. **上傳 artifact**（`if: always()`，掃描失敗也要留證據）。
6. **門檻閘門（I-06）**：`grype --only-fixed --fail-on high`。
7. **發佈（SBX-011，只在 push 到 main）**：push 至
   `ghcr.io/arthurc02/skillhub-runtime-agent-sdk`（tag 取自 Dockerfile 的
   `ARG IMAGE_VERSION` 與 commit SHA），再把 SBOM 與掃描結果以 attestation 掛到**推上去的
   digest**。**順序是政策**：閘門在發佈之前，所以過不了 I-06 的 image 到不了 registry。

另有 **`rescan` job**（每週 cron ＋ `workflow_dispatch`）：拉 GHCR 上**已發佈的那個
digest**、重掃、重新 attest `scanned_at`，並對可修的 Critical／High 亮紅燈。它不重建一個
當場新建的 image——那會是另一組位元組，回答不了「跑在生產上的那個還乾淨嗎」。

### 為什麼是 GitHub artifact attestations，不是 cosign

ADR-022 兩個都點名了，而在這裡它們產出的東西一樣：一份 Sigstore 簽章的 in-toto statement，
以 OCI referrer 存在 image digest 旁，正是閘門 A 要查的形式。差別在要維護什麼——用 Actions
的 OIDC token 做 keyless 簽章**沒有金鑰要保管、輪替或外洩**，驗證端跑
`gh attestation verify` 也不必先裝東西。cosign 的價值在「簽章發生在 GitHub Actions 以外」或
「registry 不是 GHCR」，而 ADR-022 選 GHCR 的理由正是 CI 本來就在這裡。哪天這兩件事變了，
`cosign attach` 產出的 referrer 形狀相同，要搬的是驗證流程不是儲存格式。

### 為什麼是 syft ＋ grype，不是 trivy

trivy 一個工具就能做 SBOM ＋ 掃描，少一個工具本來是優點。選 syft ＋ grype 的理由
是**閘門判的必須是我們留存的那份 SBOM**：grype 直接吃第 3 步產出的
`sbom.spdx.json`，所以「保存下來給人審的清單」與「機器據以擋下發佈的清單」是同一個
檔案。trivy 的流程是重新掃 image，SBOM 與掃描各走一次獨立的套件偵測，兩者對「這個
image 裡有什麼」可以不一致——而 I-03 與 I-04 正是要求兩者一致。

次要理由：SPDX JSON 是外部審閱者不裝 Anchore 工具也讀得懂的格式；I-03 要的是可交付
的依賴清單，不是掃描器的內部表示。

## 門檻值（**已定案：[ADR-022](../../docs/adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分，2026-08-16**）

SEC-002 的六項無值語句（威脅模型 Q18）已全部定值。屬本流水線的兩項是下面這兩個，
ADR-022 **採納了本檔原本的提案值**並補上批准者與時效；程式無需改動。

**SBX-002 的最後一個未勾原因已於 2026-08-16 由 SBX-011 解除**：I-03 的 SBOM 與 I-04 的
`scanned_at` 現在是隨 GHCR digest 保存的 attestation，閘門 A 可用 digest 直接查詢，不再依賴
90 天的 CI artifact 與人工維護的日期。**流水線側四項檢查（I-02／I-03／I-04／I-06）已全部
可自動化判定**；SBX-002 的勾選仍待部署批以實際節點驗證閘門 A 的探針查得到這兩份
attestation（SEC-009 前置條件①）。

### 1. 漏洞等級門檻（I-06）

| 發現 | 處置 |
| --- | --- |
| Critical／High，**且上游有修復版本** | **阻擋**發佈（CI fail） |
| Critical／High，**上游無修復版本** | 記錄；逐項列入下方豁免清單（具名 CVE ＋ 理由 ＋ 複審日） |
| Medium 以下 | 只記錄在 artifact，不擋 |

`--only-fixed` 是這條政策的關鍵。Debian 對多數 base OS CVE 標 `won't fix`（影響有限
或無法在該版本修），純粹的 `--fail-on high` 會讓這個 job **永遠紅、且沒有任何人能做
出讓它變綠的動作**——那不是閘門，是雜訊。分成「可修的擋、不可修的具名記錄」才讓
「紅燈」重新等於「有事情要做」。

不可修的發現不會消失：完整報告在 artifact 裡，且必須逐項出現在下方豁免清單。**沒有
靜默放行的路徑**。

例外流程（ADR-022 定案）：豁免一律寫進本檔的豁免清單，須有 CVE 編號、理由、緩解層與
複審日；**批准者是產品負責人**，批准的形式是獨立的豁免清單變更 commit。**可修的
Critical／High 沒有豁免路徑**——一個跑不受信任程式碼的沙箱，對「上游已經給了修復而我們
沒裝」沒有正當理由，留下 override 就會有人用。

**複審日 ＝ `first_exempted_at` ＋ 90 天**，`first_exempted_at` 是該 CVE **首次**被豁免的
日期，跨重掃保留。錨定在首次而非最近一次掃描日是刻意的：以掃描日錨定的話，每 30 天的
例行重掃都會把複審日往後推 90 天，**人工複審永遠不會到期**。逾期 → 該次掃描結論失效 →
依 I-04 判為過期（＝阻擋）。

### 2. 掃描結果有效期（I-04）

**30 天。** 依據：grype 的漏洞資料庫每日更新，Debian security 的修復節奏以週計；
30 天是「不會每天吵人、又不至於讓一個已知可修的 High 在生產跑一整季」的折衷。
ADR-022 另註明它**刻意不與 P-03 的 7 天節點重建同步**——兩者換的東西與變更成本都不同。

配套：

- 每次發佈重掃——已由本 job 涵蓋（`infra/images/**` 一改就跑）。
- 對**現行已發佈**的 Image 定期重掃——**已由 `rescan` job 涵蓋**（每週 cron，掃 GHCR 上
  已發佈的 digest 並重新 attest `scanned_at`；SBX-011）。
- 到期前 7 天告警；過期 → 閘門 D 判定該 Image 不得發佈、不得被新 Run 引用。**告警本身
  尚未接**：判定的材料（attestation 上的 `scanned_at`）已存在，讀它並在第 23 天叫人的是
  閘門 A 探針的工作，屬部署批。

## Digest 更新程序（I-02）

`FROM` 同時帶 tag 與 digest：tag 給人讀，digest 才是真正解析的東西。

```bash
docker pull node:22-bookworm-slim
docker inspect --format '{{index .RepoDigests 0}}' node:22-bookworm-slim
```

把印出的 `sha256:...` 填回 `runtime-agent-sdk/Dockerfile` 的 `FROM`，開 PR。改動落在
`infra/images/**`，流水線會自己跑起來把 SBOM 與掃描一起更新。

三條規矩：

- **不要為了讓掃描變綠而改 digest**，除非確認新 digest 真的含修復。base image 的
  `won't fix` 項目換 digest 不會消失。
- Digest 更新是**新的 image 版本**，跟著新 tag（`2026.08-2`…）走。歷史 Run 記錄的
  `runtime_version` 不因此改變（I-05，鐵律 4）。
- 部署的 `SKILLHUB_SANDBOX_IMAGE` 生產環境必須填 digest 形式，不能填 tag——tag 是
  移動標的，填 tag 會讓 Run 記錄不再說得出實際跑了什麼。

## SBOM 與掃描報告保存位置

**權威位置：GHCR 上隨 image digest 的 attestation**（SBX-011，2026-08-16）。

| Attestation | 內容 | 查法 |
| --- | --- | --- |
| SPDX SBOM（I-03） | 與閘門判定所吃的是**同一份檔案** | `gh attestation verify oci://ghcr.io/arthurc02/skillhub-runtime-agent-sdk@sha256:... --owner ArthurC02` |
| 掃描結果（I-04） | in-toto vulns predicate：`scanner`、逐筆發現、`metadata.scan_started_on`（＝`scanned_at`）、`summary.fixable_critical_high` | 同上 |

`summary.fixable_critical_high` 是刻意冗餘的一個數字：它正是 I-06 擋人的那個量，讓准入探針
可以斷言它是 0，而不必自己重寫一遍 grype 的等級與修復狀態邏輯。predicate 的產生器是
[`tools/ci/scan_predicate.sh`](../../tools/ci/scan_predicate.sh)。

CI artifact `runtime-agent-sdk-scan-<sha>`（保留 90 天，含 `sbom.spdx.json`、
`vulnerabilities.txt`、`vulnerabilities.json`）**保留為人讀用途**，但它不再是 I-03／I-04
的證據來源——閘門 A 查的是 attestation。

### 已發佈的 digest 與孤兒清單

**現行 digest ＝ `ghcr.io/arthurc02/skillhub-runtime-agent-sdk:<最新版本>` 當下解析到的那個。**
這裡不抄它——build 不是位元可重現的，所以每一次跑到發佈步驟都會產生**新的 digest** 並把版本
tag 移過去，一份寫在文件裡的「現行 digest」保證會過期，而過期的那份看起來跟正確的一模一樣。
**版本字串同理不抄**（原文寫死 `2026.08-3`，2026-09-03 訂正）：唯一來源是 Dockerfile 的
`ARG IMAGE_VERSION`，`runtime-image.yml` 的發佈步驟與 `rescan` job 讀的也是它。要拿當下的值：

```bash
# 版本從 Dockerfile 讀，不從這份文件抄
IMAGE_VERSION=$(sed -n 's/^ARG IMAGE_VERSION=//p' infra/images/runtime-agent-sdk/Dockerfile)
docker buildx imagetools inspect "ghcr.io/arthurc02/skillhub-runtime-agent-sdk:${IMAGE_VERSION}"

> **建置指令請照同一個方式取 tag，不要手抄。** `Dockerfile` 檔頭那行 `docker build -t …:2026.08-6` 是**第二份**版本字串，而它已經漂過一次（ARG 是 `2026.08-7` 時它還寫 `-6`）。
> **但那一行刻意不改**：`runtime-image.yml` 的 I-05 守門把 `infra/images/runtime-agent-sdk/` 底下**除 `*.md` 以外**的任何變更都當成映像內容變更，要求同批 diff 到 `ARG IMAGE_VERSION=`——**改一行註解也會觸發**。為了一行註解去 bump 版本，等於宣告一個需要重跑 ADR-023 四項實測的新映像，那比註解過期更糟。
> 所以正確的用法寫在這裡（`.md` 是該守門唯一放行的路徑）：
> ```bash
> IMAGE_VERSION=$(sed -n 's/^ARG IMAGE_VERSION=//p' infra/images/runtime-agent-sdk/Dockerfile)
> docker build -t "skillhub/runtime-agent-sdk:${IMAGE_VERSION}" infra/images/runtime-agent-sdk
> ```
> 〔2026-09-03：這一段原本被寫進 Dockerfile 的註解裡，CI 當場擋下並且是對的——已還原成原樣，改記於此。〕
```

> **注意這裡有兩個不同的問題，答案也不同**：「registry 上最新的是哪一版」看 `ARG IMAGE_VERSION`；
> 「部署實際會跑哪一版」看 `apps/sandbox/cmd/sandboxd/main.go` 的 `SKILLHUB_SANDBOX_IMAGE` 預設。
> **兩者刻意可以不同**——移動預設是 ADR-023 四項實測通過之後的動作。2026-09-03 當下前者是
> `2026.08-7`、後者是 `2026.08-5`（`-6` 與 `-7` 的四項實測未跑，見 `UPGRADES.md` 該兩節與 `04` 丙-125）。

**升版不會製造孤兒，同版重建才會。** workflow 的發佈步驟從 Dockerfile 的 `ARG IMAGE_VERSION`
讀 tag（[runtime-image.yml](../../.github/workflows/runtime-image.yml) 第 175 行），所以
`2026.08-2` → `2026.08-3` 這種**版本一起改**的發佈，是把新 digest 掛到一個**新 tag** 上，
舊 digest 的 `2026.08-2` tag 原地不動、仍指得到、attestation 仍對應——它是**被取代的版本**，
不是孤兒。下表因此**沒有新增列**。孤兒只在「版本沒改而重跑發佈」時產生（tag 被移到新
digest，舊 digest 失去指向），第三列就是那個情況的唯一實例。

> ⚠️ 但**被取代的版本會在 30 天後停止可用**：`rescan` job 同樣從 Dockerfile 讀版本
> （同檔第 246 行），只重掃**當前**版本，所以 `2026.08-2` 的 `scanned_at` 不再更新，
> 30 天後依 I-04 判為過期 → 閘門 D 拒絕它被新 Run 引用。這是設計上正確的（不該有人
> 長期釘在舊映像），但要知道它是**靜默**發生的：想留一個可回滾的舊版本，就得把它加進
> rescan 的對象清單，那目前不存在。回滾窗口 ＝ **30 天**。

**孤兒 digest 清單**（曾被推上去、tag 已移走、**不得被任何部署引用**）。孤兒不是靜默的風險：
沒有掃描 attestation 的那些，閘門 D 依 I-04 判過期本來就會拒絕；有 attestation 的那些則是
「內容正確但沒人再指向它」。唯一的實際風險是有人**手動 pin** 到其中一個。

| digest | 曾有的 tag | attestation | 產生原因 |
| --- | --- | --- | --- |
| `sha256:5d3c5615345c32e9c4e9dab3b1a474e384292343327876f6eeba42d84c19a909` | `sha-b0c270b6e890` | **只有 SBOM** | 首跑：掃描 attestation 那步以 exit 126 失敗（`scan_predicate.sh` 執行位元沒進 git index） |
| `sha256:33e9e4bdf95346d97b0ce37703cf4d0bec3b0cdd2b721bb57a3f31fe17f71c45` | `sha-0a4272c37ca1` | SBOM ＋ 掃描 | 修好後的重跑；被下一次發佈取代 |
| `sha256:7b79380c94b90621d0fb23f052ed883bbaaa0f53f6d65e2cd91328270c2c5d75` | `sha-27cbbe707359` | SBOM ＋ 掃描 | **一次純 README 編輯觸發的重建** ——已修，見下 |
| `sha256:5bcbca884feaccb4bf1cfb437644f87627898216fde7ba428228712635c9b23d` | `sha-b2180b28e458` | SBOM ＋ 掃描 | **一次 workflow 檔自身的編輯觸發的重建**（`8b16f56`：CI 腳本搬到 `tools/ci/`，`runtime-image.yml` 的呼叫路徑與 path filter 同步改）。內容與被它取代的 `sha256:774ec87b…` 是同一份 Dockerfile 的兩次 build |

**第三列的成因已修，第四列的成因是刻意留著的**：path filter 已排除
`infra/images/**/*.md`，所以純文件編輯不會再重推（第三列是那個缺口的唯一實例；第一、二列
是同一個 exit 126 事故的前後兩半，與此無關）。但 `runtime-image.yml` **自己仍在自己的 path
filter 裡**，所以連「只改 filter」這種編輯也會重建一次、把前一個 digest 孤立掉——第四列就是。

這在 `8b16f56` 重新評估過，結論是維持現狀：拿掉它可以省下這種孤兒，但 ADR-019 允許
單人直推 main 而本專案確實這樣用，拿掉之後**一個改動 build 或閘門的 commit 會沒有任何東西
驗它**——正是那一行 filter 存在的理由。一個有清理路徑的孤兒比一個沒被驗過的閘門便宜。
理由同時寫在 `runtime-image.yml` 檔頭，改動前先讀那段。

**`2026.08-3` 首次發佈後的實測複核（當時 3 筆）**：①發佈後 `2026.08-2` 仍解析到
`sha256:61ef902f…`（tag 未被移走，故未成孤兒）；②同批的純 `.md` commit（`68abae5`）**沒有
觸發任何 workflow run**，path filter 如文件所述生效。**`8b16f56` 後複核：4 筆**——
`2026.08-3` 現解析到 `sha256:774ec87b…`（＝`sha-8b16f56ee138`），前一個 digest 依上表入列。

**這四筆仍待負責人刪除**（本機 token 無 `delete:packages`，見下）。它們不是新問題，而是
每次讀到這裡都應該順手處理掉的舊帳。

**刪除方式**（本機 git credential 的 token scope 是 `gist, repo, workflow`，**沒有
`delete:packages` 也沒有 `read:packages`**，因此連 version id 都查不到，留給負責人）：
GitHub → 頭像 → **Your profile → Packages → `skillhub-runtime-agent-sdk` →
Package settings／Manage versions** → 依上表的 tag 找到該 version → **Delete**。
或以帶 `delete:packages` 的 token 呼叫
`DELETE /user/packages/container/skillhub-runtime-agent-sdk/versions/{version_id}`
（version id 先以 `GET …/versions` 查，該端點要 `read:packages`）。

## 當前掃描結果（`runtime-agent-sdk:2026.08-3`，2026-08-16）

syft v1.51.0 ＋ grype v0.117.0（漏洞庫 build `2026-08-16T06:14:30Z`，與 `2026.08-2` 那次
**同一個 build**，所以下表是同條件比較），base digest
`sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`。

| 等級 | `2026.08-1` | `2026.08-2`（含 python3） | `2026.08-3`（＋8 個純 Python 套件） |
| --- | --- | --- | --- |
| Critical | 7 | 8 | **8** |
| High | 17 | 48 | **48** |
| Medium | 57 | 118 | **118** |
| Low | 8 | 13 | **13** |
| Negligible | 58 | 94 | **94** |
| Unknown | 16 | 16 | **16** |
| **有修復版本者（I-06 閘門）** | 0 | 0 | **0（閘門通過）** |

**`2026.08-3` 逐格與 `2026.08-2` 相同**，Critical／High 的來源套件清單也一字未變（16 個
Debian 套件，全在下方豁免清單內）。上表為本機重跑值，**CI 於發佈時獨立產出同一組數字**
並寫進 I-04 attestation 的 `summary`（`total: 297`、`fixable_critical_high: 0`，
[run #31949830049](https://github.com/ArthurC02/SkillHub/actions/runs/31949830049)）。8 個新套件**帶進 0 件新發現**——這不是巧合，是准入
門檻的直接結果：純 Python wheel 沒有 native CVE 面，而**上一節擋下的那些正好相反**
（mupdf／poppler／cairo／Arrow C++ 都有 CVE 史）。豁免清單因此**沒有新增列**。

`2026.08-2` 相對 `2026.08-1` 增加的 1 Critical ＋ 31 High 全部來自 python3 帶進來的
Debian 套件（`python3.11` 系列 6 件 ×4 個套件、`libexpat1` 4 件、`libsqlite3-0` 3 件、
`libncursesw6` 1 件），全數無上游修復，逐項列入下方豁免清單。**pip 安裝的 17 個套件在
其釘選版本上 0 件 Critical／High**——這正是「釘最新版而不是豁免」換來的結果。

### 已處理：移除 npm 與 corepack

第一次掃描時**可修**的 Critical／High 共 6 件，全部落在 base image 自帶的
`/usr/local/lib/node_modules/npm/node_modules`——`tar`（Critical）、`brace-expansion`、
`picomatch`、`sigstore`、`ip-address`。這些是 npm 自己的依賴，執行期沒有任何東西載入
它們（entrypoint 是 `node /opt/skillhub/run.mjs`），而且沙箱 `--network none`，npm 本來
就裝不了任何東西。

已在 Dockerfile 的 install 步驟後移除 npm／npx／corepack。這比豁免正確：一個跑不受信任
程式碼的沙箱裡放著套件管理器，就算它的依賴是乾淨的也是負債。移除後可修的 Critical／
High 歸零，`import` SDK 的煙霧測試通過。

### 豁免清單（無上游修復；`first_exempted_at` **2026-08-16**，複審日 **2026-11-14**）

複審日 ＝ `first_exempted_at` ＋ 90 天（ADR-022 I-06）。**重掃不會推遲它**——重掃更新的是
掃描結論，`first_exempted_at` 跟著 CVE 走，只在該 CVE 首次進入本清單時寫一次。

以下全部標記 `won't fix` 或上游尚無修復版本，皆為 Debian bookworm base 套件。已確認
升 digest 無效（`node:22-bookworm-slim` 已是最新），`perl-base` 是 dpkg 依賴不可移除。

| 套件 | 版本 | Critical | High |
| --- | --- | --- | --- |
| `libc-bin`／`libc6` | 2.36-9+deb12u14 | CVE-2026-5450 | CVE-2026-5928、CVE-2026-5435 |
| `perl-base` | 5.36.0-7+deb12u3 | CVE-2026-8376、CVE-2026-13221、CVE-2026-42496、CVE-2026-12087、CVE-2026-57433 | CVE-2026-9538、CVE-2026-42497、CVE-2026-48959、CVE-2026-48962、CVE-2026-7017、CVE-2026-57432 |
| `libtasn1-6` | 4.19.0-2+deb12u1 | — | CVE-2025-13151 |
| `libtinfo6`／`ncurses-base`／`ncurses-bin`／`libncursesw6` | 6.4-4 | — | CVE-2025-69720 |
| `gzip` | 1.12-1 | — | CVE-2026-41992 |
| `libacl1` | 2.3.1-3 | — | CVE-2026-54369、CVE-2026-54370 |
| **`python3.11`／`python3.11-minimal`／`libpython3.11-minimal`／`libpython3.11-stdlib`** | 3.11.2-6+deb12u8 | — | CVE-2025-69534、CVE-2026-3644、CVE-2026-7210、CVE-2026-9669、CVE-2026-11972、CVE-2026-15308 |
| **`libexpat1`** | 2.5.0-1+deb12u2 | — | CVE-2025-59375、CVE-2026-25210、CVE-2026-41080、CVE-2026-45186 |
| **`libsqlite3-0`** | 3.40.1-2+deb12u2 | CVE-2025-7458 | CVE-2026-11822、CVE-2026-11824 |

> `2026.08-3` **未新增任何列**，亦未移除任何列；`first_exempted_at` 與複審日不因本次
> 升版變動（重掃不推遲複審日，見上）。

粗體四列是 `2026.08-2` 加 python3 帶進來的（`libexpat1`／`libsqlite3-0` 是 CPython 的依賴，
`libncursesw6` 併入既有的 ncurses 列）。**它們的 `first_exempted_at` 同為 2026-08-16**——
與既有各列同一天，純粹因為兩件事發生在同一天，不是沿用舊日期：這幾個 CVE 是今天第一次
進入本清單，複審日照樣是首次日 ＋ 90 天。

**加 python3 讓豁免清單長了四列，這是這個決定的一部分成本，不是意外**。四列的緩解理由與
下段相同（沙箱層的隔離基線），但注意 `libexpat1`／`libsqlite3-0` 的攻擊面**確實變大了**：
它們會解析工作負載讀進來的資料，而下段那些多半不會。這是接受的風險而不是被解決的問題，
複審日到期時要一併重看。

豁免理由：無可用修復，且緩解層與 ADR-005／015 的基線重疊（非 root、`CapDrop=ALL`、
`no-new-privileges`、唯讀 rootfs、無主機掛載、`--network none`、gVisor）。**這是「無法
處理」不是「不必處理」**——複審日到期須重掃，屆時已有修復者依 I-06 轉為阻擋。

未採用的處置：改用 `node:22-trixie-slim`（Debian 13）可望清掉多數 bookworm 項目，但
換 base 發行版需要重跑 Runtime 相容性測試（SEC-009），不在本項範圍，留給負責人與
digest 更新一併評估。

## 本機重跑

升版時另見 [`runtime-agent-sdk/UPGRADES.md`](runtime-agent-sdk/UPGRADES.md)：ADR-023 要求
每次改變 digest 的變更都附四項行為重驗的實測輸出，而本節的掃描只回答供應鏈風險，
兩者不覆蓋對方。

```bash
docker build -t skillhub/runtime-agent-sdk:scan infra/images/runtime-agent-sdk

docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD/scan:/scan" \
  anchore/syft:v1.51.0 skillhub/runtime-agent-sdk:scan -o spdx-json=/scan/sbom.spdx.json

# 完整報告
docker run --rm -v "$PWD/scan:/scan" anchore/grype:v0.117.0 sbom:/scan/sbom.spdx.json -o table

# CI 的閘門（exit != 0 即不得發佈）
docker run --rm -v "$PWD/scan:/scan" anchore/grype:v0.117.0 sbom:/scan/sbom.spdx.json \
  --only-fixed --fail-on high -o table
```

Windows／Git Bash 需在 `docker run` 前加 `MSYS_NO_PATHCONV=1`，否則掛載路徑會被改寫。

## 服務映像（ADR-019 job 5）

`04` 丙-158：三個應用程式服務（`apps/platform`、`apps/llm`、`apps/web`）原本沒有映像，
CI 的 `images` job 是空殼，平台自己「建得起來、部署得動」這件事從未被證明過。本節是三份
Dockerfile 與 `infra/compose/docker-compose.yml` 新 `app` profile 的說明；**CI 端要接
GHCR push 仍是協調者的工作**（本批範圍只到映像與本機驗證，不動 `.github/workflows/`）。

### 三個映像各裝什麼

| 映像 | 來源 | 內容 | 執行 |
| --- | --- | --- | --- |
| [`platform/`](platform/) | `apps/platform`（單一 Go module） | build stage 用 `golang:1.27.0-bookworm`（與 `devtools/Dockerfile` 同一 digest）靜態編譯 `api`／`worker`／`maintenance`／`reindex` 四個指令；runtime stage 是 `gcr.io/distroless/static-debian12:nonroot`（無 shell、無套件管理器——這是鐵律 1 要保護的控制平面，能少一樣東西可用就少一樣），另外把 `contracts/packaging/profiles` 複製進去供 `PACKAGING_PROFILES_DIR` 使用 | 無 `ENTRYPOINT`，靠 distroless 內建的 `PATH` 解析：`CMD ["api"]` 是預設，`docker run <image> worker\|maintenance\|reindex ...` 換另外三個 |
| [`llm/`](llm/) | `apps/llm`（FastAPI＋uv，build context 是 `apps/llm` 本身，不是 repo root——`packages/api-stub-py` 那個本機路徑依賴只在 `dev` dependency group，`--no-dev` sync 用不到） | build stage 裝 `uv`（版本與 sha256 installer 校驗值取自 `tools/toolchain.yaml`，做法與 `devtools/Dockerfile` 相同），`uv sync --frozen --no-dev --no-editable` 到 `/opt/venv`——**`--no-editable` 是必要的**：預設的 editable 安裝是一個指回 build stage `/src/src` 的 `.pth` 檔，runtime stage 只複製 `/opt/venv` 的話 `import skillhub_llm` 會找不到套件（本批實測撞過這個） | `uvicorn skillhub_llm.app:app --host 0.0.0.0 --port 8000`，match `contracts/openapi/llm-internal.yaml` 的 `servers[0]`；`/healthz` 不需要 `LLM_SERVICE_TOKEN`，掛了 `HEALTHCHECK` |
| [`web/`](web/) | `apps/web` ＋ `packages/api-client-ts`（build context 是 repo root，因為 `apps/web` 的 `file:../../packages/api-client-ts` 依賴需要兩棵樹都在） | build stage 依序 `npm ci`＋`npm run build`（先 client 套件再 web，與 `Taskfile.yml` 的 `build:web`／`build:api-client` 一致；`npm run build` 本身會跑 `check-bundle-origins.mjs`，建置失敗代表 bundle 裡出現了預期外的絕對網址）；runtime stage 是 `nginx:1.29-alpine-slim`，只放編譯好的 `dist/` 與 [`nginx.conf`](web/nginx.conf) | 見下一節 |

### `web` 的反向代理：為什麼不是 `location /api/`

`api/client.ts` 的 `API_BASE_URL` 預設空字串（同源），這在 clean test mode（`cmd/api`
自己端出 `apps/web/dist`）與 `vite preview` 下成立，但**一般容器化部署（這個 `app`
profile）沒有任何東西讓它成立，除非有東西幫忙**——`web` 這個 nginx 容器就是那個東西。

原本設想是單一 `/api/` 前綴代理，但 `contracts/openapi/public.yaml` 沒有這個前綴：
路徑是扁平的（`/skills`、`/runs/{id}`、`/auth/...`、`/me`……），只有四條真的在 `/api/`
下面。更根本的問題是**同一個路徑同時是頁面與 API**：`GET /skills/{id}` 是 JSON，也是
Skill 詳情頁 `/skills/$skillId`（`apps/web/src/router.tsx`）的網址；`GET /runs/{id}`
同理是 Trace 頁。`apps/web/vite.config.ts` 的註解原文就寫著這件事——「the obvious dev
setup — proxy a list of path prefixes — cannot work with these routes」，該檔用兩個
origin＋CORS 迴避；`cmd/api` 的 clean mode（`SKILLHUB_CLEAN_MODE=1`）用
`spaFallback`／`navigationCatcher`（檢查 API 回應的狀態碼與 Content-Type，把
404／405／2xx-JSON 吞掉换成 `index.html`）解決同一個問題，但那段邏輯活在 `cmd/api`
自己的行程裡，`web` 容器前面站的是**另一個**容器的 nginx，看不到那個回應。

[`nginx.conf`](web/nginx.conf) 改用請求端就有的訊號——同一份 `spaFallback` 註解自己
點名的那個訊號：「Every client of this API is fetch(), which sends `*/*` or
`application/json`；a browser address bar is the only thing that says
`text/html`」。規則因此是：GET 且 `Accept` 含 `text/html` → 一律當作瀏覽器導覽，回
`index.html`（前端路由自己決定要顯示哪一頁）；其餘（`apps/web` 每一支 `api/*.ts` 的
`fetch()` 呼叫）→ 全部轉給 `platform-api`。這對現有的每一組「頁面路徑＝API 路徑」都
正確，**不需要檢查回應**。

例外兩條，寫在規則之前：`/auth/`（GitHub OAuth 的兩個 302 導向）與 `/downloads/`
（`<a href>` 直接導覽拿檔案位元組，不是 `fetch()`）——這兩條是「真的需要瀏覽器導覽、
且 API 必須自己回答」的路徑，一律直接轉發，不吃 `Accept: text/html` 那條規則。以
2026-09-05 對照 `contracts/openapi/public.yaml` 與 `router.tsx` 的結果，這是目前唯二
的例外。

**誠實的已知落差**：這是用請求端訊號去逼近 clean mode 用回應端資訊做的決定，對現有
路徑表是完整的，但**不會自動涵蓋未來**——如果之後新增一條頂層 GET 路徑，同時（a）是
真正的瀏覽器導覽目標（redirect、串流、任何非 JSON 的答案）且（b）與某個前端頁面路徑
完全同名，就要比照 `/auth/`／`/downloads/` 在 [`nginx.conf`](web/nginx.conf) 手動加一條
無條件轉發——不會被現有規則自動接住。新增路由時請一併檢查這件事。

另外 `/healthz` 被 `nginx.conf` 佔用作容器自身的存活探針（回固定字串，不轉發），理由
與位置寫在該檔——`GET /` 這種預設探針方式在沒有 `Accept: text/html` 時會被轉發到
`platform-api`（它沒有 `/` 這條路由），量到的是上游死活而非 nginx 自己，本批實測撞過
（容器一直卡 `unhealthy`，換成 `/healthz` 後才對）。副作用是這個 profile 下經
`web` 容器打 `/healthz` 拿到的是 nginx 的固定回應，不是 `platform-api` 自己的
`/healthz`——需要探 API 本身請直接打它自己的埠（見下）。

### 本機建置與驗證

```bash
docker build -f infra/images/platform/Dockerfile -t skillhub/platform:local .
docker build -f infra/images/llm/Dockerfile -t skillhub/llm:local apps/llm
docker build -f infra/images/web/Dockerfile -t skillhub/web:local .

docker run --rm skillhub/llm:local python -c "import skillhub_llm"
docker compose -f infra/compose/docker-compose.yml --profile app config
```

三個 Dockerfile 的 build context 不一樣（`platform`／`web` 是 repo root，`llm` 是
`apps/llm`），理由各自寫在對應 Dockerfile 檔頭。

### `docker-compose.yml` 的 `app` profile

新增 `platform-api`、`platform-worker`、`llm`、`web` 四個服務，全部掛
`profiles: ["app"]`——`task dev`（無 `--profile`）行為不變，仍只起 Postgres 與
SeaweedFS；要含這四個服務得 `docker compose --profile app up`。與既有 `postgres`／
`seaweedfs` 的連線資訊（含 SeaweedFS 的開發身分 `skillhubdev`／`skillhubdevsecret`，
本身是 `seaweedfs-s3.json` 裡已提交的非機密佔位值）用字面值寫死在 compose 檔——與
`litellm` 服務既有的 `DATABASE_URL` 寫法同一個理由：這些主機名由這份檔案自己的拓撲
決定，不是部署時要選的東西。其餘變數照 `.env.example` 的名字透穿（`${VAR:-}`，空值
預設），與 `apps/platform` 原生跑在 host 上讀到的是同一組名字。

**`LLM_SERVICE_TOKEN` 原本想比照做成 `${LLM_SERVICE_TOKEN:?set in .env}`**：
`apps/llm`（`app.py` 的 `require_service_token`）用它擋掉除 `/healthz` 外的每一條
路由，若兩邊都空字串，等於「雙方都沒填、驗證形同虛設」而不是「沒有驗證」，看起來
正是 `${VAR:?...}` 要擋的那種假綠燈。**這個寫法本批實測後撤掉了**：Compose 對整份
檔案的變數插值是一次做完、**不分 profile**——`docker compose -f
infra/compose/docker-compose.yml up -d postgres seaweedfs`（`task dev` 本身的
呼叫方式，automation.md 紅線 2 要求它在完全沒有任何密鑰的情況下也要能跑）在
`:?` 版本下直接失敗，即使 `platform-api`／`llm` 兩個服務根本沒被啟動。單一
compose 檔案沒有「只在用到的 profile 才插值」這種機制，所以 `LLM_SERVICE_TOKEN`
最終跟這份檔案其餘變數一樣是 `${LLM_SERVICE_TOKEN:-}`（空值預設）——它重開的那個
缺口（兩邊都不填時驗證形同虛設）就留在這裡誠實記著，不是用 compose 語法擋掉的。

`SKILLHUB_CLEAN_MODE` 刻意在這四個服務裡完全沒出現——這個 profile 是多容器形狀，
`cmd/worker` 自己會拒絕在該旗標開著時啟動（ADR-060 決策 6：clean mode 是單一行程），
把它接進來只會讓 `platform-worker` 啟動就死。

`platform-api` 對外開 `127.0.0.1:8080`（除錯直接打 API 用）；`web` 開
`127.0.0.1:8081`（同源入口，瀏覽器該開的網址）；`llm` 不對外開埠——只有
`platform-api`／`platform-worker` 內部呼叫它。`maintenance`／`reindex` 沒有對應的常駐
服務：它們是 operator 手動觸發的一次性指令，仍在 `platform` 這個映像裡，用
`docker compose --profile app run platform-api maintenance <subcommand>` 之類的方式
單次執行。

### 已實測，含 base image digest 來源

三個映像都已在本機建置成功並驗證跑得動（`docker build`／`docker run`／完整
`docker compose --profile app up` 對照真正的 `postgres`／`seaweedfs`）。Base image
digest 由 `docker pull` 後 `docker inspect --format '{{index .RepoDigests 0}}'`
取得，做法與上面「Digest 更新程序」一節相同：

| 映像 | Digest 來源 |
| --- | --- |
| `golang:1.27.0-bookworm` | 沿用 `devtools/Dockerfile` 已經釘的那個（同一個工具鏈版本） |
| `gcr.io/distroless/static-debian12:nonroot` | `sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab`（2026-09-04 `docker pull` 當下） |
| `python:3.12-slim-bookworm` | `sha256:782412e85d0f0984994c290652577d4018aff08145c85b262bb63dc0c7522254`（同上） |
| `node:22.14.0-bookworm-slim` | `sha256:1c18d9ab3af4585870b92e4dbc5cac5a0dc77dd13df1a5905cea89fc720eb05b`（同上） |
| `nginx:1.29-alpine-slim` | `sha256:c9366b8c560169b101ca0e5422ed063b20779e6454c2326b9c9704225c9b0c08`（同上） |

映像大小（`docker images`，本機建置）：`skillhub/platform:local` 124 MB、
`skillhub/llm:local` 216 MB、`skillhub/web:local` 21.3 MB。

**Tag 慣例：commit SHA，絕不用 `latest`**——理由與 `runtime-agent-sdk` 那條一致
（ADR-019 job 5 原文、本檔「Digest 更新程序」一節）：部署與回滾都要能指向明確的
commit，`latest` 是會動的標的。本批只建到本機 `:local` tag 供驗證；CI 端建置後
`push` 到 GHCR、打 commit SHA tag，是協調者要接上 `.github/workflows/ci.yml` 的
`images` job 的部分，不在本批範圍內。

**掃描與 attestation 不比照 `runtime-agent-sdk`**：那一節的 syft／grype／GHCR
attestation 流水線是 SEC-002 對「會執行不受信任內容」的 Sandbox Runtime Image 的
要求（鐵律 1）；`platform`／`llm`／`web` 是控制平面／能力提供者／靜態前端，不執行
不受信任內容，本 ADR-019 job 5 原文對它們也只要求 tag＋push，沒有 SBOM／掃描閘門的
字面要求。是否比照辦理是留給協調者與負責人的政策問題，本批未擅自決定。

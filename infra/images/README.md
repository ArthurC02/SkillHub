# Runtime Image 審核流水線（SBX-002）

本目錄放 Sandbox 執行用的 Runtime Image。目前只有一個：
[`runtime-agent-sdk/`](runtime-agent-sdk/)（Node.js 22 LTS ＋ pinned
`@anthropic-ai/claude-agent-sdk` ＋ python3 與目錄宣告的 Python 依賴），由
`services/sandbox` 的 DockerProvider 以 `SKILLHUB_SANDBOX_IMAGE` 引用。

## 映像內容與理由（版本 `2026.08-2`）

| 內容 | 為什麼在裡面 |
| --- | --- |
| `node:22-bookworm-slim`（digest pin） | PDM-003／ADR-015 選定的 Agent SDK 執行環境 |
| `@anthropic-ai/claude-agent-sdk`（版本 pin） | 工作負載本體；版本即 `runtime_version`（I-05） |
| `unzip` | Skill 套件是 zip，解壓縮只能發生在沙箱內（鐵律 1） |
| **`python3` ＋ 下表 9 個套件** | 見下節 |
| **不在裡面**：`npm`／`npx`／`corepack`／`pip` | 執行期不載入、沙箱無網路，套件管理器留在沙箱裡只是負債 |

### 為什麼加 python3（`2026.08-2`，2026-08-16）

M2 基準試跑的 trace 原文是 `/bin/bash: line 75: python3: command not found`，而
`tools/content/seed-skills.json` 記錄 45 個目錄 Skill 中 **33 個 `deps_runtime = python`**
（[content-baseline-report.md §6.3](../../plans/mvp/m2/content-baseline-report.md)）。那 33 個
Run 多數仍然「成功」，但**執行的不是 Skill 帶的腳本，而是模型對腳本的轉譯**——同一個
Skill 在裝了 Python 的環境與這裡是兩種行為。

**沙箱沒有網路**（SBX-007：唯一到得了的位址是模型閘道），所以「使用者自己 `pip install`」
這條路不存在：**預裝是唯一途徑**。

預裝清單＝**45 個目錄 Skill 宣告的依賴集合，一個不多**。這條規則可以從資料重新推導，
「看起來有用」不行，後者會讓映像無止境長大。

| 套件 | 版本 pin | 幾個 Skill 宣告 |
| --- | --- | --- |
| `pandas` | 3.0.5 | 26 |
| `openpyxl` | 3.1.5 | 17 |
| `lxml` | 6.1.1 | 12 |
| `python-docx` | 1.2.0 | 2 |
| `numpy` | 2.4.6 | 1（另為 pandas 依賴） |
| `python-dateutil` | 2.9.0.post0 | 1（另為 pandas 依賴） |
| `pypdf` | 6.16.1 | 1 |
| `pdfplumber` | 0.11.10 | 1 |
| `python-pptx` | 1.0.2 | 1 |

**版本 pin 的取法**：每個直接依賴釘在**通得過 I-06 閘門的最新版**。第一次嘗試釘了一組
保守的舊版本，grype 在 `lxml`／`pypdf`／`pdfminer.six` 上抓到**可修的 High**——依 ADR-022
那沒有豁免路徑，所以往上釘而不是往下豁免。**間接依賴不釘**：可重現性的錨是 image digest
與隨它發佈的 SBOM（SBX-011），手寫 lock 檔會是第二份會漂移的答案。

**代價，明說**：映像由 806 MB 長到 **1.24 GB**（Python site-packages 238 MB ＋ 直譯器與
其函式庫）。以及 **`pandas` 是 3.x**，而目錄裡的 Skill 多半寫於 2.x 年代——本批**沒有實測**
兩者的行為差異，那要靠 CONTENT-008 的重跑基準（換映像本來就必須重跑，見下）。

> ⚠️ **換映像＝ Agent 相容軸失效。** `skill_runtime_compatibility`（migration 0022）以
> **(Skill Version × Runtime Image)** 為鍵，目前 45 筆實測值全部掛在 `2026.08-1`。部署改用
> `2026.08-2` 之後，目錄對這 45 筆會回到「未驗證」，直到有人在新映像上重跑基準。這是刻意的：
> 沿用舊映像的結論正是本節要修的那個問題。

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

## 門檻值（**已定案：[ADR-022](../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) 第二部分，2026-08-16**）

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
[`.github/workflows/scan_predicate.sh`](../../.github/workflows/scan_predicate.sh)。

CI artifact `runtime-agent-sdk-scan-<sha>`（保留 90 天，含 `sbom.spdx.json`、
`vulnerabilities.txt`、`vulnerabilities.json`）**保留為人讀用途**，但它不再是 I-03／I-04
的證據來源——閘門 A 查的是 attestation。

## 當前掃描結果（`runtime-agent-sdk:2026.08-2`，2026-08-16）

syft v1.51.0 ＋ grype v0.117.0（漏洞庫 build `2026-08-16T06:14:30Z`），base digest
`sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`。

| 等級 | 件數（`2026.08-1`） | 件數（`2026.08-2`，含 python3） |
| --- | --- | --- |
| Critical | 7 | **8** |
| High | 17 | **48** |
| Medium | 57 | 118 |
| Low | 8 | 13 |
| Negligible | 58 | 94 |
| Unknown | 16 | 16 |
| **有修復版本者** | 0 | **0** |

增加的 1 Critical ＋ 31 High **全部來自 python3 帶進來的 Debian 套件**（`python3.11`
系列 6 件 ×4 個套件、`libexpat1` 4 件、`libsqlite3-0` 3 件、`libncursesw6` 1 件），
全數無上游修復，逐項列入下方豁免清單。**pip 安裝的 9 個套件在其釘選版本上 0 件
Critical／High**——這正是「釘最新版而不是豁免」換來的結果。

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

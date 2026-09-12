# ADR-080：映像漏洞只擋會被部署的那幾顆，上游的每週報一次

- 狀態：**Accepted**（2026-09-13，負責人指示：「要，繼續完成待辦」）
- 日期：2026-09-13
- 相關：[ADR-022](./ADR-022-sandbox-deployment-topology-and-security-thresholds.md)（Runtime Image 的 grype 門檻 I-03／I-04／I-06，本 ADR 沿用同一組掃描器與 `--only-fixed` 判準）、[ADR-077](./ADR-077-dependency-vulnerabilities-block-only-when-a-fix-exists.md)（依賴漏洞只擋有修補版的；本 ADR 把同一條原則延伸到映像，並補上第二個軸）、[ADR-019](./ADR-019-monorepo-structure-and-cicd.md)（CI/CD 基線）

## 背景

- `runtime-agent-sdk` 那一顆映像從 SBX-011 起就有 syft ＋ grype（`runtime-image.yml`）。**其餘八顆一個都沒有被掃過**：`infra/compose/docker-compose.yml` 拉下來的四顆（Postgres／pgvector、SeaweedFS、LiteLLM、Prometheus），以及我們自己建的四顆（`platform`、`web`、`llm`、`devtools`）。
- `dep-audit`（ADR-077）讀的是 lockfile、授權與 workflow，**不碰映像層**。兩者之間沒有任何機器負責映像。
- 這個缺口是用眼睛發現的：手動以同一組釘住的掃描器掃 LiteLLM 那一顆，掃出 tornado 的可修 High，以及 pypdf 與 glibc。**一個要靠人想到才會被發現的缺口，就是還沒有被守住的缺口。**
- 上游映像與我們自己的映像，可做的修正不一樣：前者要等上游重建或換一個 tag，後者換基底、升依賴、重建即可。ADR-077 的判準（「有沒有修補版」）在這裡不夠用——**還要問修補版在不在我們手上**。
- 第一次把八顆全掃過之後，這條界線又多了一刀。`platform` 與 `web` 乾淨；`llm` 有一個真的可修 High（`libpcre2-8-0`），而**基底 digest 已經是上游最新的一版**，所以修法不是等上游，是 `runtime-agent-sdk` 早就對同一個套件用過的那一招：對單一套件 `--only-upgrade`，不動釘住的 digest、也不做會漂的整包 `apt-get upgrade`。`devtools` 則是兩萬多列——主體是第三方工具二進位裡的 vendored 依賴（golangci-lint、task、docker CLI、uv 各自帶著自己的 `golang.org/x/*` 與 npm 樹），外加 grype 把字串裡的 kernel 版本當成安裝的 kernel。**那些修正一條都不在我們手上，而且它不會被部署。**

## 決策 1：映像由 `image-scan.yml` 掃，清單從 compose 讀

- 腳本是 [`tools/ci/scan-images.sh`](../../tools/ci/scan-images.sh)，workflow 是 `.github/workflows/image-scan.yml`：改到 compose 或四顆應用映像時跑、每週排程、可 `workflow_dispatch`。
- **映像清單直接從 `infra/compose/docker-compose.yml` 解析出來，不在腳本裡另抄一份。** 抄一份就會漂，而本節這個缺口本來就是漂出來的；`dependency-policy` 對「compose 與 workflow／`tools/ci/*.sh` 引用同一個映像」的比對只擋 tag 與 digest 不一致，擋不住「少列了一顆」。
- 每顆映像留三份產出：SBOM（spdx-json）、完整 grype 報告（table ＋ json）、以及 `--only-fixed --fail-on high` 那一遍的報告。全部以 artifact 保存 90 天，與 `runtime-image.yml` 同規格。
- `runtime-agent-sdk` **不搬進來**：它的掃描結果要進 attestation（I-04），綁在發佈流程上，與這一支的節奏不同。

## 決策 2：閘門分三層，照「誰能修」與「會不會被部署」畫

| 層 | 映像 | 有可修的 Critical／High 時 |
| --- | --- | --- |
| **deployed** | 我們自己建、而且會被部署的 `platform`、`web`、`llm` | **job 紅**。修法在我們手上：換基底、升依賴，或對單一套件 `--only-upgrade`。 |
| **upstream** | compose 拉下來的四顆 | 只報。**排程那一跑（`FAIL_ON_UPSTREAM=1`）才紅**，於是每週有人被通知，而沒有人的 PR 被無關的上游發現擋住。處置是升那個 pin 或換 tag，不是把閘門關掉。 |
| **dev-only** | `devtools` | **永不紅**，只留 SBOM 與報告。 |

- **`devtools` 為什麼不擋**：它是開發容器，不會被部署、不處理使用者資料，而它的發現絕大多數住在第三方工具的編譯產物裡——擋下來沒有人做得出對應的修正，只會製造一個每次都紅的閘門，而那正是 [ADR-077](./ADR-077-dependency-vulnerabilities-block-only-when-a-fix-exists.md) 決策 1 要避免的事。它的基底與工具版本由 Dependabot 與 `dependency-policy` 管，不由這條閘門管。
- **上游那一層預期會紅一陣子**：今天四顆都有可修的 High。那不是誤報，是「pin 落後了」的每週提醒；把它讀成噪音就失去了這條線的用處。

## 決策 3：掃描器版本兩支 workflow 一起動

syft 與 grype 的 pin 進 `tools/devctl/toolchain_versions.go` 的名冊，`dependency-policy` 逐檔比對；只升其中一支 workflow 會 FAIL 並列出兩個位置。理由同 ADR-079：**同一個工具在兩個地方各有一個版本，遲早會各走各的**。

## 影響

- 新增一支 workflow 與一支腳本；`dep-audit` 不變。
- 每週多一次會下載漏洞資料庫的排程跑；腳本以一個具名 Docker volume 共用資料庫，整趟只下載一次。
- 上游映像的可修 High 從此有人看得到——**這不代表它們立刻會被修掉**，它代表沒有人能再說「不知道」。

## 後續工作

- 上游映像第一次被排程判紅時，決定該顆 pin 要升到哪個 tag；若上游長期沒有修好的 tag，要寫成 `04` 的一列，不要靠把閘門調鬆解決。

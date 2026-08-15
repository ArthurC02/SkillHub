# Runtime Image 審核流水線（SBX-002）

本目錄放 Sandbox 執行用的 Runtime Image。目前只有一個：
[`runtime-agent-sdk/`](runtime-agent-sdk/)（Node.js 22 LTS ＋ pinned
`@anthropic-ai/claude-agent-sdk`），由 `services/sandbox` 的 DockerProvider 以
`SKILLHUB_SANDBOX_IMAGE` 引用。

「**經審核**」不是「建得起來」。SEC-002 §4.5 對執行映像列了六項檢查，其中三項是
發佈時閘門（威脅模型 §5.4 閘門 D）：

| ID | 檢查 | 等級 | 由誰執行 |
| --- | --- | --- | --- |
| I-02 | Image 以 digest pin，非 tag | 阻擋 | `runtime-image` CI job 的第一步 |
| I-03 | 保存 SBOM／依賴清單與建置來源記錄 | 阻擋 | 同 job，syft 產 SPDX JSON |
| I-04 | 漏洞掃描已執行且結果在有效期內 | 阻擋 | 同 job，grype；**有效期未定值** |
| I-06 | 漏洞等級超過政策門檻 | 告警 | 同 job；**門檻未定值** |

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

### 為什麼是 syft ＋ grype，不是 trivy

trivy 一個工具就能做 SBOM ＋ 掃描，少一個工具本來是優點。選 syft ＋ grype 的理由
是**閘門判的必須是我們留存的那份 SBOM**：grype 直接吃第 3 步產出的
`sbom.spdx.json`，所以「保存下來給人審的清單」與「機器據以擋下發佈的清單」是同一個
檔案。trivy 的流程是重新掃 image，SBOM 與掃描各走一次獨立的套件偵測，兩者對「這個
image 裡有什麼」可以不一致——而 I-03 與 I-04 正是要求兩者一致。

次要理由：SPDX JSON 是外部審閱者不裝 Anchore 工具也讀得懂的格式；I-03 要的是可交付
的依賴清單，不是掃描器的內部表示。

## 門檻提案值（**暫定，待負責人定案**）

SEC-002 允收準則列了六項「目前為無值語句」的門檻（威脅模型 Q18），未定值前該項
**不可自動化判定，不得記為通過**。以下兩項屬本流水線，提出建議值供定案；程式已依
提案值實作，但 SBX-002 在定案前不勾。

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

例外流程（對應 SEC-002「例外流程待定」）：豁免一律寫進本檔的豁免清單，須有 CVE 編號、
理由與複審日；複審日過了還沒複審，該 Image 視同未通過 I-06。

### 2. 掃描結果有效期（I-04）

**提案 30 天。** 依據：grype 的漏洞資料庫每日更新，Debian security 的修復節奏以週計；
30 天是「不會每天吵人、又不至於讓一個已知可修的 High 在生產跑一整季」的折衷。

配套（定案後才實作）：

- 每次發佈重掃——已由本 job 涵蓋（`infra/images/**` 一改就跑）。
- 對**現行已發佈**的 Image 定期重掃：目前做不到。CI 掃的是當場建出來的 image，不是
  registry 裡那顆；container registry 仍是 ADR-019 待決策 1。定案後接 `schedule:`
  觸發並改掃 registry 的 digest。
- 過期 → 閘門 D 判定該 Image 不得被新 Run 引用。

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

CI artifact `runtime-agent-sdk-scan-<sha>`，保留 90 天，內含：

| 檔案 | 內容 |
| --- | --- |
| `sbom.spdx.json` | SPDX JSON 格式 SBOM（I-03） |
| `vulnerabilities.txt` | grype table 報告，人讀用 |
| `vulnerabilities.json` | grype JSON，機器讀用 |

**現況限制**：90 天的 CI artifact 不等於 I-03 要的「Image 保存 SBOM」——SBOM 應該跟著
image 存在 registry 旁（OCI referrer／`cosign attach`），才能在節點准入（閘門 A）時查得
到。container registry 未定案（ADR-019 待決策 1）前先落在 CI artifact，registry 定案後
搬家。

## 當前掃描結果（`runtime-agent-sdk`，2026-08-16）

syft v1.51.0 ＋ grype v0.117.0，base digest
`sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436`。

| 等級 | 件數 |
| --- | --- |
| Critical | 7 |
| High | 17 |
| Medium | 57 |
| Low | 8 |
| Negligible | 58 |
| Unknown | 16 |
| **有修復版本者** | **0** |

### 已處理：移除 npm 與 corepack

第一次掃描時**可修**的 Critical／High 共 6 件，全部落在 base image 自帶的
`/usr/local/lib/node_modules/npm/node_modules`——`tar`（Critical）、`brace-expansion`、
`picomatch`、`sigstore`、`ip-address`。這些是 npm 自己的依賴，執行期沒有任何東西載入
它們（entrypoint 是 `node /opt/skillhub/run.mjs`），而且沙箱 `--network none`，npm 本來
就裝不了任何東西。

已在 Dockerfile 的 install 步驟後移除 npm／npx／corepack。這比豁免正確：一個跑不受信任
程式碼的沙箱裡放著套件管理器，就算它的依賴是乾淨的也是負債。移除後可修的 Critical／
High 歸零，`import` SDK 的煙霧測試通過。

### 豁免清單（無上游修復，複審日 **2026-11-16**）

以下全部標記 `won't fix` 或上游尚無修復版本，皆為 Debian bookworm base 套件。已確認
升 digest 無效（`node:22-bookworm-slim` 已是最新），`perl-base` 是 dpkg 依賴不可移除。

| 套件 | 版本 | Critical | High |
| --- | --- | --- | --- |
| `libc-bin`／`libc6` | 2.36-9+deb12u14 | CVE-2026-5450 | CVE-2026-5928、CVE-2026-5435 |
| `perl-base` | 5.36.0-7+deb12u3 | CVE-2026-8376、CVE-2026-13221、CVE-2026-42496、CVE-2026-12087、CVE-2026-57433 | CVE-2026-9538、CVE-2026-42497、CVE-2026-48959、CVE-2026-48962、CVE-2026-7017、CVE-2026-57432 |
| `libtasn1-6` | 4.19.0-2+deb12u1 | — | CVE-2025-13151 |
| `libtinfo6`／`ncurses-base`／`ncurses-bin` | 6.4-4 | — | CVE-2025-69720 |
| `gzip` | 1.12-1 | — | CVE-2026-41992 |
| `libacl1` | 2.3.1-3 | — | CVE-2026-54369、CVE-2026-54370 |

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

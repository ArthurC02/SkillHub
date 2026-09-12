# ADR-079：工具鏈版本跟著上游升級，所有位置一起動

- 狀態：**Accepted**（2026-09-12，負責人指示：「CI使用到的版本該升級就升級，不用死鎖舊版本。」）
- 日期：2026-09-12
- 取代：[ADR-078](./ADR-078-dependency-governance-admission-updates-install-guards-licenses-and-pins.md)〈補充：第一輪 Dependabot 之後〉裡「Dockerfile 的更新再忽略 `python` 與 `golang`」、「uv 再忽略 `uv-build`」這兩條，以及決策 1 裡「Dockerfile 的更新忽略 `astral-sh/uv` 與 `node`」。ADR-078 其餘決策不變。
- 相關：[ADR-077](./ADR-077-dependency-vulnerabilities-block-only-when-a-fix-exists.md)、[ADR-030](./ADR-030-portable-developer-automation-and-contract-code-generation.md)、[ADR-023](./ADR-023-agent-sdk-version-pinning-and-behaviour-revalidation.md)

## 背景

ADR-078 為了避免 Dependabot 只改到一處、讓版本在不同檔案之間分岔，把 Node、Python、Go、uv 的基底映像與 `uv-build` 加進忽略清單。結果是這些版本不會再有人提出升級，只會停在當時的版本上：CI 跑 Node 22.14.0（當時最新的 22 是 22.23.2，LTS 已經是 24）、Python 3.12、Go 1.27.0、uv 0.11.3，而 CI 實際從 setup-uv 拿到的 uv 已經是 0.12.13。npm 的新版本冷卻期（`min-release-age`）也因為 CI 的 npm 停在 10 而無法採用。

## 決策 1：本批升到上游目前的穩定版

| 工具 | 從 | 到 | 所有位置 |
| --- | --- | --- | --- |
| Node | 22.14.0 | 24.21.0（Active LTS，npm 11.19.0） | `.node-version`、web 與 devtools 映像；CI 改用 `node-version-file: .node-version` |
| Python | 3.12 | 3.14 | `apps/llm/.python-version`、兩份 `requires-python` 下限、codegen 的 `requires-python`、llm 與 codegen 映像；CI 改用 `python-version-file` |
| Go | 1.27.0 | 1.27.1 | 四個 `go.mod`（CI 的 setup-go 讀它）、devtools／platform／codegen 映像 |
| uv | 0.11.3 | 0.12.13 | toolchain.yaml、llm 與 devtools 的安裝腳本與雜湊、codegen 的 uv 映像；`uv_build` 上限放寬到 `<0.13` |
| task | 3.45.4 | 3.53.1 | toolchain.yaml、devtools 映像 |
| golangci-lint | 2.13.1 | 2.13.2 | toolchain.yaml、devtools 映像 |
| GitHub Actions | checkout v4、setup-node v4、setup-go v5、setup-python v5.4.0、setup-uv v5、upload-artifact v4、paths-filter v3、attest v2、attest-sbom v2 | checkout v7.0.1、setup-node v7.0.0、setup-go v7.0.0、setup-python v7.0.0、setup-uv v10.0.1、upload-artifact v7.0.1、paths-filter v4.0.3、attest v4.2.2、attest-sbom v4.1.0 | 各 workflow（Dependabot 第一輪 PR #9 的同一組 SHA；該 PR 的 CI、Runtime Image、Egress 都是綠的） |

- 升級後本機驗證：
  - apps/llm 在 3.14 上 247 通過；
  - codegen 以 3.14 映像重生，產出一字不差；
  - web 在 Node 24.21.0 上建置、618 條測試、格式與型別檢查都通過；
  - platform 與 sandbox 在 Go 1.27.1 上 vet 與測試都通過。
- 附帶的變化：
  - ruff 以 3.14 為目標重排了 3 個檔；
  - apps/llm 的 lockfile 拿掉 3.12 專用的 wheel；
  - codegen 的 lockfile 以 3.14 重新解析。
- **npm 採用 `min-release-age=7`**（寫在四份 `.npmrc`）：npm 11.10 以上才認得它，Node 24 帶的 npm 11.19 滿足。
- **不在本批**：
  - runtime image 的 `node:22`：一動就要升 `IMAGE_VERSION` 並付費重驗（ADR-023），留到下一次重建時一起升。
  - 產碼器（datamodel-code-generator、openapi-generator、redocly、ogen、sqlc）與 pglite：換版會改生成產出，各自單獨升級並審查差異。
  - compose 與 CI 共用的 pgvector、seaweedfs：Postgres 的主版本是資料庫的遷移決策，seaweedfs 3 → 4 是主版本升級，各自評估。
  - devtools 映像的 docker CLI 與 compose：它們不在 CI 上跑。

## 決策 2：不再忽略，改由機器要求所有位置一起動

- `dependabot.yml` 拿掉對 `node`、`python`、`golang`、`astral-sh/uv` 映像與 `uv-build` 的忽略。Dependabot 照常提出升級。
- 只保留一條忽略：`@types/node` 的 major。型別的主版本跟著 `.node-version` 走；Node 本身升主版本時，同一批一起升。
- automation-check 的 `dependency-policy` 新增版本一致性比對，名冊在 `tools/devctl/toolchain_versions.go`。規則如下：
  - 同一個工具（node、go、python、uv、task、golangci-lint）在每個位置的版本必須相同，否則 FAIL，並列出每個位置目前的值；
  - 某個位置讀不到版本（檔案不見了，或寫法變了）也會 FAIL。
- Dependabot 只改到其中一處的 PR 會是紅的，訊息列出還要一起改的位置：該做的是在同一個 PR 補齊其餘位置，而不是關掉它。
- CI 讀 `.node-version` 與 `.python-version`，自己不再寫版本號，所以不在名冊上。

## 成本與限制

- **一個升級要改好幾個檔**。名冊讓人知道要改哪幾個，但沒有替人去改；Dependabot 的 PR 需要有人補齊。
- **Node 的主版本升級會被提出**，包括還不是 LTS 的版本（例如 26）。要不要升，由審 PR 的人按 Node 的 LTS 時程決定。
- **本機要跟上**：doctor 會要求本機的 Node 是 24.21.0、uv 是 0.12.13。這台機器由 fnm 管理 Node。

## 待決策

- 無。

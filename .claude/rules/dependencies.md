---
paths:
  - "**/package.json"
  - "**/package-lock.json"
  - "**/.npmrc"
  - "**/go.mod"
  - "**/pyproject.toml"
  - "**/uv.lock"
  - "**/Dockerfile*"
  - "infra/compose/**"
  - ".github/workflows/**"
  - ".github/actions/**"
  - ".github/dependabot.yml"
  - "tools/toolchain.yaml"
---

新增、升級或移除任何依賴（npm 套件、Go module、Python 套件、Docker 映像、GitHub Action）之前，先讀 [automation.md〈依賴的准入、更新與閘門〉](../../docs/development/automation.md)：准入要回答的問題、哪些版本必須跟 `tools/toolchain.yaml` 一起動、Dependabot 的 PR 怎麼收，以及 `devctl dep-audit` 與 automation-check 的 `dependency-policy` 會擋什麼。

判準永遠在被指的那一份，不在這裡。

# SkillHub Code Quality Report（D 階段基線）

## 範圍與方法

本報告僅使用目前 repository 內可驗證資訊：`Taskfile.yml`、CI workflow、各模組設定檔與本次實際執行結果。未取得工具量測的項目一律標示「未量測」。

## 已驗證的觀察

1. 專案有明確的本機與 CI 檢查入口：`task ci` 會串 `automation-check`、`agent-sync`、`gen --check`、lint/test/build（`/home/runner/work/SkillHub/SkillHub/Taskfile.yml:79-89`）。
2. CI 有獨立 `contracts-drift` job，會執行 `go -C tools/devctl run . gen --check` 與 OpenAPI lint（`/home/runner/work/SkillHub/SkillHub/.github/workflows/ci.yml:506-533`）。
3. 最近 `main` 的 CI 失敗點集中在 `tools/devctl` 的 dependency policy 檢查，包含 SeaweedFS tag 不一致與 UV 版本不一致（GitHub Actions run `36263158623`，job `devctl`）。
4. 本次本機執行 `go -C tools/devctl run . gen --check` 為綠燈，未發現 generated drift。
5. 本次本機執行 `go -C apps/platform test ./internal/creator/creation` 為綠燈；`go -C tools/devctl test ./...` 受既有 dependency policy 基線問題影響失敗，針對本次變更的 targeted tests 為綠燈。

## 推測或待後續量測

1. 各模組測試覆蓋率（未量測）。
2. 重複程式碼比例與變化幅度（未量測）。
3. 依賴更新風險與過期清單（未量測）。
4. Web/LLM/Sandbox 的跨模組整合缺口（需以完整 CI 與環境測試量測）。

## 各模組評估

| 模組 | 已驗證觀察 | 風險/缺口（未量測會標示） |
| --- | --- | --- |
| `apps/platform` | Go 模組、CI 會跑 lint/test，且 platform job 依賴 Postgres/SeaweedFS 與 Python interpreter（`/home/runner/work/SkillHub/SkillHub/.github/workflows/ci.yml:202-260`） | 整體覆蓋率未量測；大量整合測試對環境依賴高 |
| `apps/web` | 有 typecheck/lint/vitest/playwright/build 腳本，CI 在 Linux/Windows 矩陣執行（`/home/runner/work/SkillHub/SkillHub/apps/web/package.json:6-15`, `/home/runner/work/SkillHub/SkillHub/.github/workflows/ci.yml:113-200`） | bundle 變化、e2e 穩定性未量測 |
| `apps/llm` | 使用 `uv` + `pytest` + `ruff`，CI 有獨立 llm job（`/home/runner/work/SkillHub/SkillHub/apps/llm/pyproject.toml:22-43`, `/home/runner/work/SkillHub/SkillHub/.github/workflows/ci.yml:440-450`） | 模型與外部 provider 相關路徑在本次未量測 |
| `apps/sandbox` | 獨立 Go module（執行平面隔離），CI 有 sandbox job（`/home/runner/work/SkillHub/SkillHub/apps/sandbox/go.mod:1-7`, `/home/runner/work/SkillHub/SkillHub/.github/workflows/ci.yml`） | Docker/gVisor 相關執行路徑本次未量測 |
| `packages/` | 目前有 `api-client-ts`（build/typecheck）與 `api-stub-py`（Python package）作為契約產物（`/home/runner/work/SkillHub/SkillHub/packages/api-client-ts/package.json:15-21`, `/home/runner/work/SkillHub/SkillHub/packages/api-stub-py/pyproject.toml:1-11`） | generated client 對契約改動敏感，需持續依賴 drift gate |

## API contracts / generated 狀態

- `contracts/openapi/public.yaml` 明確標示為單一契約來源（`/home/runner/work/SkillHub/SkillHub/contracts/openapi/public.yaml:5-8`）。
- CI 有 `contracts-drift` job（`/home/runner/work/SkillHub/SkillHub/.github/workflows/ci.yml:506-533`）。
- 本次未發現必須修改 generated 或 contracts 的具體問題，因此沒有變更 generated 檔案。

## 本次 D 階段切片結論

- 已完成：文件基線、低風險重複邏輯抽離（有實際 caller）、關鍵邊界測試補強、相關測試與 drift 驗證。
- 未完成（列入後續）：跨模組更大範圍重構、覆蓋率/重複率量測、修復與本次需求無直接關聯的 CI 基線漂移。

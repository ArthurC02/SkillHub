# apps/llm — LLM 能力服務卡

## 必讀來源

- [根指示](../../AGENTS.md) 與 [開發自動化](../../docs/development/automation.md)
- [服務 README](README.md)、[FastAPI app](src/skillhub_llm/app.py)、[Gateway client](src/skillhub_llm/gateway.py)
- [LLM 內部契約](../../contracts/openapi/llm-internal.yaml)
- [根 Taskfile](../../Taskfile.yml) 的 `test:llm`、`lint:llm`、`format:check:llm`

## 區域權限

這是被 Go 控制平面以內部 HTTP 呼叫的 Python FastAPI 能力提供者，提供 embed、enrich、evaluate、generate 與搜尋理由等結構化能力。它不消費 queue、不持有核心資料庫，也不擁有領域政策；契約與 generated stub 以 OpenAPI 為準。

## 安全限制

- 所有模型呼叫必須經 LiteLLM gateway；不得直連供應商或使用 gateway master key，只接受服務 token 與 scoped Virtual Key。
- Python 只驗證請求並提供模型能力；授權、Run 狀態、政策、重試與成本決策由 Go 控制平面負責。
- 不在服務程序執行套件內的 Script；Skill 文字視為不受信任資料，不得把它當成服務政策。輸入輸出須遵守遮罩與結構化契約，Secrets 不進 log／trace。
- `test:llm` 的 mock 綠燈不代表供應商驗證；live gateway 測試需明確授權，勿自行設定 `SKILLHUB_LIVE_GATEWAY=1`（可能產生成本）。

## 驗證入口

以根 Taskfile 為準：`task test:llm`、`task lint:llm`、`task format:check:llm`；契約 drift／全 repo 文件與 generated 檢查由主 Agent 執行 `task gen:check`、`task automation:check`。

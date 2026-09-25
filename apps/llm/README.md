# apps/llm

Internal Python LLM service (FastAPI + uv). Called by the Go platform
over internal HTTP only — it never consumes the queue and holds no business rules.

```bash
uv sync
uv run pytest
uv run ruff check .
uv run uvicorn skillhub_llm.app:app --reload
```

互動式 Skill 創作已依[互動創作](../../docs/adr/README.md#互動創作)
接上 LangGraph。`POST /v1/creation/step` 從 Go 快照重建單次工作，回傳澄清、
確認、草稿或有界工具意圖；工具執行、持久化、成本與重試政策由 Go 決定。
服務不保存跨工作 checkpoint，LangSmith tracing 在此路徑強制關閉。

呼叫需要既有服務 Bearer token 及 Go Worker 簽發的 `X-Creation-Gateway-Key`。
沒有短效 key 不回退共用金鑰。一般步驟呼叫一次模型；試跑未達條件的 review
可先診斷、再重寫正文、最後產生結構化決策，合計最多三次呼叫，共用該步逾時
並彙總用量。診斷讀取最新評估，不因後續靜態驗證訊息而失去評估；重寫正文
也會帶入 `validate_draft` 意圖。後續 ReAct observation 由 Go 驗證後放進下一份
快照。完整設定、免費測試與尚待量測的界線見
[互動創作開發手冊](../../docs/development/interactive-creation.md)。

# docs/spikes/ — 證據索引

本目錄不放產品程式碼或 CI 輸入。一次性可行性探測的結論已沉澱在下列報告、決策與持續驗證工具；需要查核時讀這些受維護來源，不將歷史探測程式接回服務或 CI。

## 結論現在住在哪

### `pdm-003-litellm-gateway/`

驗證 LiteLLM Proxy 的 Anthropic 相容 `/v1/messages` 端點在工具使用與串流下的行為、
每 Run Virtual Key 的模型範圍與預算強制、以及 Agent SDK 實際從哪個路徑載入 Skill。

| 原檔 | 結論落點 |
| --- | --- |
| `README.md`、`config.yaml`、`test_gateway.py`、`results.txt` | [`docs/plans/mvp/m0/pdm-003-litellm-spike-report.md`](../plans/mvp/m0/pdm-003-litellm-spike-report.md) 第 2～9 節（7/7 測項、三個相容性坑、重現方式）；架構決策見[模型閘道與可觀測性](../adr/README.md#模型閘道與可觀測性) |
| `test_skill_loading.py`、`results-skill-loading.txt` | 同報告第 10 節（6/6 PASS；**載入路徑是 `<workdir>/.claude/skills/<name>/`，不是 `skills/`**）。此測項已升格為[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的 Agent SDK 版本重驗清單第 1 項，每次升級的實測輸出記在 [`infra/images/runtime-agent-sdk/UPGRADES.md`](../../infra/images/runtime-agent-sdk/UPGRADES.md) |
| `test_supplemental.py`、`results-supplemental.txt` | 同報告第 11 節（Skill 自主觸發率 0/9、`skills` 白名單過濾成立、prompt caching 省錢不省 token 且 `/v1/messages` 不透傳 `cache_*` 欄位）。後兩項同樣進了[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的 Agent SDK 版本重驗清單第 2、3 項，持續證據在 `UPGRADES.md` |

### `pdm-011-intent-search/`

驗證「自然語言意圖 → 排序後的 Skill 候選＋符合原因」在小型語料上以混合檢索
（BM25 ＋ 向量，RRF 融合）是否可行。

| 原檔 | 結論落點 |
| --- | --- |
| `README.md`、`run_spike.py`、`results.txt`、`results-embedding.txt` | [`docs/plans/mvp/m0/pdm-011-spike-report.md`](../plans/mvp/m0/pdm-011-spike-report.md)（含第 9 節真 Embedding 與 RRF 增益的補跑）；定案與四項實證調整見 [意圖搜尋](../adr/README.md#意圖搜尋)「定案紀錄」 |
| `samples/`（12 份取自 `github.com/anthropics/skills` 的公開 `SKILL.md`） | **未被任何工具引用，隨 spike 一併刪除。** 正式的評估語料是 [`tools/goldenset/`](../../tools/goldenset/)（31 份 Skill ＋ 增強後的 `corpus_enriched/`），它自成一套、從不讀 spike 的 `samples/` |

## 持續驗證

需要持續回歸的內容以[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的 Agent SDK 版本重驗清單為準，實測輸出記在 `infra/images/runtime-agent-sdk/UPGRADES.md`。

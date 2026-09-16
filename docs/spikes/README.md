# docs/spikes/ — 墓碑

本目錄原本放 **M0 驗證用 spike code**：一次性的可行性探測程式，可重跑但**不是產品程式碼**、
不被任何服務 import、不進 CI。它們的任務是回答「這個方向能不能走」，答案問出來之後，
**結論已全數沉澱到下列文件與工具**，程式碼本身不再有讀者。

**內容已於 commit `bcd5817` 之後刪除**（24 個追蹤檔案，兩個 spike）。

## 怎麼還原

```bash
git checkout bcd5817 -- docs/spikes
```

`bcd5817` 是刪除前的最後一個 commit，spike 程式碼與其結果檔在該 commit 完整存在。
還原只為了考古；**不要把它們接回產品程式碼或 CI**，理由見上。

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

## 為什麼不留程式碼

spike 的價值在結論，不在實作：它的相依（`litellm[proxy]`、釘死的 `fastapi<0.140`、
一份約 300MB 的 `.venv`）早已與產品的版本基線脫節，重跑一次比重寫一次貴。
真正需要持續回歸的部分沒有被丟掉——它變成了[Sandbox 隔離與執行安全](../adr/README.md#sandbox-隔離與執行安全)的 Agent SDK 版本重驗清單，
由 `infra/images/runtime-agent-sdk/UPGRADES.md` 逐次升級記錄實測輸出，
那才是有人維護、會被 CI 與 review 盯著的落點。

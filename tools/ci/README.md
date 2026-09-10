# tools/ci

CI 用的輔助腳本。放在這裡而不是 `.github/workflows/`，是因為那個目錄的內容是 workflow 定義；腳本是被 workflow 呼叫的程式碼，換一個 CI 供應商時該跟著走的是 workflow 檔，不是這兩支。

| 腳本 | 歸屬的 workflow | 做什麼 |
| --- | --- | --- |
| [`check_egress_allowlist.py`](check_egress_allowlist.py) | [`.github/workflows/egress-allowlist.yml`](../../.github/workflows/egress-allowlist.yml) | 斷言 `infra/egress/allowlist.yaml` 的 ADR-022 Q3 不變式：`tier: sandbox` 恰為一筆 `model_gateway`、N-07 供應商網域 deny-list、`pinned_ip` 的 tier 規則 |
| [`scan_predicate.sh`](scan_predicate.sh) | [`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)（`review` 與 `rescan` 兩個 job 都用） | 把 grype 的 JSON 轉成 in-toto vulns predicate（含 `scanned_at` 與 `fixable_critical_high`），供 I-04 的 attestation 使用 |
| [`stack-smoke.sh`](stack-smoke.sh) | [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)（`images` job，**推 GHCR 之前**） | 把剛建好的三個服務映像真的啟動起來：Postgres＋migration＋物件儲存＋`platform-api`＋worker＋nginx＋`llm`，斷言 SPA 出得來、CSP 標頭真的送出、目錄 API 經 nginx 回得出合契約的 body、worker 沒有立刻死，最後用 [`stack-browser.mjs`](stack-browser.mjs) 開真的 Chromium 走公開頁與登入後的工作區頁。在此之前這個 job 從來沒有啟動過任何一個它建出來的映像 |
| [`stack-browser.mjs`](stack-browser.mjs) | 由 `stack-smoke.sh` 在 Playwright 映像裡執行 | 真瀏覽器打真後端（`04` 丙-221）：路由清單**直接讀 `e2e/routes.ts` 與 `fixtures/platform.ts`**，每一條跑登入與未登入兩次，斷言 React 有掛上、沒有未捕捉例外、沒有非預期的 4xx／5xx。第一次跑就抓到 `GET /downloads` 被 nginx 301 成 `/downloads/` 然後 404 |

三支都可在本機直接跑：`python3 tools/ci/check_egress_allowlist.py`、`bash tools/ci/scan_predicate.sh <grype-json> <output-json>`、`PLATFORM_IMAGE=… WEB_IMAGE=… LLM_IMAGE=… bash tools/ci/stack-smoke.sh`（後者要有 Docker；`SMOKE_KEEP=1` 會把整組留著給人手動戳）。

> `scan_predicate.sh` 在 workflow 內一律以 `bash` 前綴呼叫，且 git index 的 `+x` 位元也設著——本專案在 Windows 開發，執行位元可能不隨 `git add` 保留，兩者並用是因為任一單獨都可能被另一個 OS 的 re-add 靜默還原（實際發生過，見 [infra/images/README.md](../../infra/images/README.md) 孤兒清單第一列）。

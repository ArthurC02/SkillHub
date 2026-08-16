# tools/ci

CI 用的輔助腳本。放在這裡而不是 `.github/workflows/`，是因為那個目錄的內容是 workflow 定義；腳本是被 workflow 呼叫的程式碼，換一個 CI 供應商時該跟著走的是 workflow 檔，不是這兩支。

| 腳本 | 歸屬的 workflow | 做什麼 |
| --- | --- | --- |
| [`check_egress_allowlist.py`](check_egress_allowlist.py) | [`.github/workflows/egress-allowlist.yml`](../../.github/workflows/egress-allowlist.yml) | 斷言 `infra/egress/allowlist.yaml` 的 ADR-022 Q3 不變式：`tier: sandbox` 恰為一筆 `model_gateway`、N-07 供應商網域 deny-list、`pinned_ip` 的 tier 規則 |
| [`scan_predicate.sh`](scan_predicate.sh) | [`.github/workflows/runtime-image.yml`](../../.github/workflows/runtime-image.yml)（`review` 與 `rescan` 兩個 job 都用） | 把 grype 的 JSON 轉成 in-toto vulns predicate（含 `scanned_at` 與 `fixable_critical_high`），供 I-04 的 attestation 使用 |

兩支都可在本機直接跑：`python3 tools/ci/check_egress_allowlist.py`、`bash tools/ci/scan_predicate.sh <grype-json> <output-json>`。

> `scan_predicate.sh` 在 workflow 內一律以 `bash` 前綴呼叫，且 git index 的 `+x` 位元也設著——本專案在 Windows 開發，執行位元可能不隨 `git add` 保留，兩者並用是因為任一單獨都可能被另一個 OS 的 re-add 靜默還原（實際發生過，見 [infra/images/README.md](../../infra/images/README.md) 孤兒清單第一列）。

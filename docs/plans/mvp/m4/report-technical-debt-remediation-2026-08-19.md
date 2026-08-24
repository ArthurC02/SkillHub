# 技術債修復凍結報告（2026-08-19）

- 類型：凍結報告
- 狀態：原始總帳與覆寫狀態表已整併並退役。

## 結論

2026-08-19 的技術債盤點及後續多輪阻抗審查，已將已修復項目以實作、回歸測試與 contract/generated checks 收斂。已修項不在此重抄，以免建立第二份會漂移的 ledger；歷史逐項證據保留於 Git history 與既有 ADR／測試。

## 尚未完成的事項與唯一活入口

| ID | 現況 | Owner／下一動 | 活文件與完成證據 |
| --- | --- | --- | --- |
| `DEPLOY-IAC-001` | deployment | 部署負責人建立 ADR-022 所定 sandbox node IaC，完成真機安全驗收 | [release-checklist §2.1](release-checklist.md)：IaC、pinned network 與 SEC-009 證據 |
| `RUNTIME-PYTHON-001` | decision/deployment | 負責人定值 Python 版本；部署負責人重建 runtime image 並在 gVisor 驗證 | [release-checklist §2.8](release-checklist.md#28-仍待定值或部署驗證的技術債)：image／文件／真機證據一致 |
| `LLM-EVAL-007` | decision | 負責人定義 Judge／Suggest usage 與 cost 的唯一事實來源 | [04 backlog N-8](../../04-backlog-and-handoffs.md)：決策與 contract test 證據 |
| `LLM-RES-001` | partial | 部署負責人完成 anonymous search 的分散式 rate limit／成本保護設計 | [release-checklist §2.8](release-checklist.md#28-仍待定值或部署驗證的技術債)：拒絕前不得呼叫 LLM 的負載證據 |
| `SUPPLY-RUNTIME-LOCK-001` | deployment | runtime image owner 選定並落地 repo-owned lock／constraints | [release-checklist §2.8](release-checklist.md#28-仍待定值或部署驗證的技術債)：乾淨 cache 的 dependency tree 一致 |

其餘 MVP、部署、法務與封測殘項的唯一入口是 [04-backlog-and-handoffs.md](../../04-backlog-and-handoffs.md)；不得回填到這份凍結報告。

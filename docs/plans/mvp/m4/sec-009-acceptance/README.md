# SEC-009 驗收證據

[ADR-022](../../../../adr/ADR-022-sandbox-deployment-topology-and-security-thresholds.md) §5 指定的證據落點。每次執行一個 `YYYY-MM-DD-<node-id>/` 子目錄，內含 45 列（**現為 46 列**，見 ADR-022 §2 的 2026-08-26 補記）判定表、`versions.txt` 與各測項輸出。保存期 ≥ 1 年。

## 這個目錄現在有什麼

| 子目錄 | 是什麼 | 是驗收嗎 |
| --- | --- | --- |
| [`2026-08-26-nested-dev-container/`](2026-08-26-nested-dev-container/) | Suite 1 的一部分，在一台 Windows 開發機的巢狀 gVisor 容器裡跑，外加 CI 的 gVisor leg 與 GHCR 稽核 | **不是。** 目錄名刻意不是 node-id，因為沒有節點 |

## 判定的規矩，一句話

**46 項全數 `pass`、0 項 `unknown` 才放行**，任一項 `fail` 或 `unknown` → `SelfHostedProvider` 不得開放外部使用者提交 Skill 執行。部分通過**不得**以「其餘項目風險低」為由放行，這一條沒有例外流程（ADR-022 §3）。

所以子目錄裡的判定欄寫的是「本次是否產生了機器可查的證據」，**不是 pass／fail**——一份把「有證據」寫成「通過」的表，正是那條規矩存在的理由。

## 誰在什麼時候跑

見 ADR-022 §4。摘要：Suite 2 在每台節點入池前由部署批負責人跑；Suite 1（T4 除外）由 CI 在每次 `apps/sandbox/**`／`infra/images/**` 變更時跑；首次全套由部署批負責人執行、**產品負責人見證並簽署**——那個簽名就是「可以開放外部使用者」。

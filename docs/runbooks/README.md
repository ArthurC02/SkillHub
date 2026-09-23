# Runbook 索引

Runbook 是可重複執行的操作手冊：每一步都要有明確的前提、動作與驗證。它們描述目前系統，不保存已過期的施工歷史。

| 目的 | 手冊 |
| --- | --- |
| 從零開始建置整個系統、恢復遺失的部署設定 | [整體 Provision](provisioning.md) |
| 建置、換版、還原控制平面 | [控制平面節點](control-plane.md) |
| 建置、換版、重建模型閘道 | [模型閘道節點](gateway.md) |
| 建置、驗收、換新沙箱節點 | [沙箱節點](sandbox-node.md) |
| P1 事件停止與恢復派送 | [P1 停派送](p1-dispatch-halt.md) |
| 帳號清除寫入防線的切換 | [帳號清除寫入防線](account-purge-write-fence-rollout.md) |
| 帳號清除角色的切換 | [帳號清除角色切換](purge-role-cutover.md) |

開發機的最短啟動路徑在[開發自動化](../development/automation.md)；它不是生產 Provision 的替代品。

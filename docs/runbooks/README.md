# Runbook 索引

Runbook 是可重複執行的操作手冊：每一步都要有明確的前提、動作與驗證。它們描述目前系統，不保存已過期的施工歷史。

## 先建立共同模型

Skill Hub 的控制平面是唯一能改變領域狀態的地方：Web 只呼叫它；API 與 Worker 共同使用 Postgres 和物件儲存；Worker 是唯一的佇列消費者。Python LLM 服務與 Sandbox 都是能力提供者，不能直接讀寫核心資料庫。每次模型呼叫都經模型閘道；每個 Run 由控制平面簽發短效 Virtual Key，Sandbox 用它執行，再把 Trace 與產物交回控制平面。

正式環境的依賴順序是：控制平面的資料與服務 → 模型閘道 → Sandbox 節點 → 控制平面登錄 Gateway 與 Provider → Web 對外。若某一步的健康檢查失敗，停在該邊界修復；不要以跳過 Worker、放寬 egress、改用 master key 或直接讓能力提供者碰資料庫來繞過。

先依目的選一份入口文件，不要把所有手冊當成一條連續流程：

| 情況 | 先讀 | 何時再讀其他手冊 |
| --- | --- | --- |
| 從零建立本機或正式系統、遺失設定後重建 | [整體 Provision](provisioning.md) | 依它指定的順序讀節點手冊 |
| 已有正式環境，只換版、還原或重建一種主機 | 對應的節點手冊 | 牽涉相依節點或 DNS 切換時回到整體 Provision |
| 正在發生安全事故、需要停止或恢復派送 | [P1 停派送](p1-dispatch-halt.md) | 先保留現場，後續再依節點手冊重建或 drain |
| 只在跨越指定舊 migration 的升級 | 對應的一次性切換手冊 | 不適用於新建環境或一般換版 |
| CI 紅了，但訊息只說 `exit code 1` | [CI 看不見的失敗](ci-red.md) | 確定是部署環境而非 CI 的問題時，才轉到節點手冊 |

部署的可用版本是「可取得的、釘定 digest 的映像」加上團隊選定的變更驗證證據。此專案目前的渲染器會從 GHCR 解析映像，因此 GitHub CI 的 green 結果是這個部署管線的證據之一；它不是一般 Git repo 或 Domain Memory 的普遍前提。沒有 CI 的 repo 應依其治理政策使用簽署 commit、push 與授權簽署者完成 HITL；若要部署到本手冊的環境，仍必須先提供等效的受驗證映像與 provenance，不能假定渲染器會從工作樹自行建置。

| 目的 | 手冊 |
| --- | --- |
| 從零開始建置整個系統、恢復遺失的部署設定 | [整體 Provision](provisioning.md) |
| 重建 Skill 啟用展示並保留啟用／未啟用對照 | [Skill 啟用 Demo](skill-activation-demo.md) |
| 準備可散布 Demo，驗證授權放行、Fork 與下載 | [可攜套件 Demo](portable-demo.md) |
| 建置、換版、還原控制平面 | [控制平面節點](control-plane.md) |
| 建置、換版、重建模型閘道 | [模型閘道節點](gateway.md) |
| 建置、驗收、換新沙箱節點 | [沙箱節點](sandbox-node.md) |
| P1 事件停止與恢復派送 | [P1 停派送](p1-dispatch-halt.md) |
| 診斷一次讀不到原因的 CI 失敗；判斷一個綠燈證明了什麼 | [CI 看不見的失敗](ci-red.md) |
| 跨 `0049`～`0051` 的一次性帳號清除寫入防線切換 | [帳號清除寫入防線](account-purge-write-fence-rollout.md) |
| 跨 `0059` 的一次性清除角色切換 | [帳號清除角色切換](purge-role-cutover.md) |

開發機的最短啟動路徑在[開發自動化](../development/automation.md)；它不是生產 Provision 的替代品。

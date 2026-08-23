# ADR-055：免費 Run 額度關閉——而「不限制」不是維持現狀，是一個動作

- 狀態：Accepted
- 日期：2026-08-23
- 修訂：[ADR-028](./ADR-028-beta-admission-and-quota-enforcement-points.md) 決策 2 的部署設定（強制點與程式碼不變，只是這個部署不套用額度）
- 同形先例：[ADR-050](./ADR-050-beta-runs-in-parallel-with-the-sandbox-acceptance.md)、[ADR-052](./ADR-052-m5-starts-in-parallel-with-an-unfinished-mvp.md)、[ADR-054](./ADR-054-the-cap-was-lifted-by-authorisation-not-by-evidence.md)——**一個被明示接受的風險**
- 相關：`PDM-010`、`01` §12（Sandbox 成本不可持續）、`04` 乙-2、[`05` R-1a](../plans/05-pending-rulings.md)

## 先更正一個被寫錯的事實

`05` R-1a 的第二列寫著：

> `RUN_QUOTA` 未開 ⇒ 額度不強制、`GET /me/quota` 不掛載、preflight 不帶配額區塊

**那描述的是 `RUN_QUOTA=off`，不是未設。** 程式碼（`apps/platform/cmd/api/main.go` 的 `quotaFromEnv`）逐字相反：

> The proposal's numbers are **the default** rather than something a deployment has to set, and that asymmetry with the two retention knobs above is deliberate. An unset retention means data is not collected, which is safe; **an unset allowance would mean the platform's only real cost ceiling is off, which is not**.

所以在本 ADR 之前，這個部署**是有在強制額度的**——用 PDM-010 的提案值（首 30 天 20 次／其後每 30 天 30 次／每日 5 次）。**「額度先不限制」因此不是維持現狀，是一個動作**，而那個動作關掉的正是程式碼註解點名的那一道。

## 決策：`RUN_QUOTA=off`

`.env.example` 顯式寫 `off`，不是留白。留白會被讀成「還沒決定」，而它實際上會強制。

**同時關掉顯示**，這是程式碼刻意綁在一起的（`04` 乙-2：**顯示必須跟著強制**）。`GET /me/quota` 不掛載、preflight 不帶配額區塊——**一個沒有被強制的數字出現在畫面上，就是一個沒有東西在保證的承諾**。

## 被關掉的是次數，不是每個 Run 的成本

**兩道煞車不受影響，這件事決定了這個風險有多大**：

| 仍在強制 | 值 | 強制點 |
| --- | --- | --- |
| 每 Run 的閘道預算 | **`max_budget = $0.50`** | LiteLLM 短效虛擬金鑰（`trial/execution/gateway.go`） |
| 每 Run 的 TPM | 200,000 | 同上 |
| 每 Run 的 token 總量 | 300K input／60K output | 沙箱 harness 逐回應累計（`SBX-013`） |

所以暴險的形狀不是「無上限」，是**「次數無上限 × 每次 $0.50」**。

**量級對照**（12 人 × 14 天封測）：

- **關閉前**：首窗 20 次／人是最先綁住的那一條 ⇒ 上界約 **12 × 20 × $0.50 ＝ $120**。
- **關閉後**：沒有上界。**單一使用者每天跑 50 次、跑滿 14 天 ＝ 700 次。**

**⚠️ 2026-08-23 稍晚更正：上面那個 $350 高估了約 13 倍。** 它把每個 Run 都算成燒滿 $0.50 的上限，而閘道帳按短效金鑰分組（每 Run 一把）量出來的是：**中位數 $0.0382**（中位數 9 次呼叫），觀測到最貴的合法 Run 是 $0.3019 與 $0.2367，**上限從未被違反**。所以那 700 次的期望值是 **≈ $27**，不是 $350——**$350 是最壞情況不是預期情況**，而把最壞情況寫成預期會讓下一個人對著錯的數字調旋鈕。

**也因此不建議現在調降 `max_budget`**：$0.50 看起來是中位數的 13 倍很鬆，但**觀測到的合法 Run 已經到 $0.30**，降到 $0.20 會殺掉真的工作。樣本太小，現在調是拿沒量夠的東西省錢。

**這就是本 ADR 接受的風險**：`01` §12 的「Sandbox 成本不可持續」是一個 live risk，額度是它的緩解之一，現在那一項不在了。**沒有任何機制會在花費異常時通知**——`O11Y-003` 的 Alertmanager 部署與通知路由未接（`RELEASE-006` 自陳的部署期缺口），所以第一個知道的方式是看帳單。

## 這個裁定沒有授權的事

**受測者不得被告知有額度。** [consent-and-data-policy.md](../plans/mvp/gate-test/consent-and-data-policy.md) 原文對受測者寫著：

> 額度用完時平台會擋下新的試跑並顯示重置時間——那是設計行為，不是故障。

**那件事現在不會發生。** 同意書正在等法務，所以這段必須在送出之前改掉——`02` §2.2「不得顯示沒被強制的承諾」對文件與對畫面同樣成立。已改。

## 終止條件

- **這是部署設定，不是產品決策。** PDM-010 的四個值仍是 `待追認`，程式碼與測試一個字都沒改，把 `RUN_QUOTA` 拿掉就回到強制。
- **封測開始前應重新評估。** 12 個人是可以看著的規模；公開 Beta 不是。

## 影響

### 正面

- 封測期間沒有人會因為額度被擋住，M5 與試跑的驗證都不必為額度繞路。
- **顯示與強制仍然一致**（都關），沒有製造出「畫面說有、實際沒有」的第三種狀態。

### 成本與限制

- 上述暴險。
- **`RELEASE-009` 的 B2 門檻不受影響**（那量的是漏斗比例不是次數），但**「額度用完」這個使用者情境在封測裡不會被觀察到**——若公開 Beta 要開額度，它的可理解性沒有任何真人證據。

## 待決策

- **公開 Beta 是否恢復額度、用什麼值。** PDM-010 的四個值仍待追認，本 ADR 沒有回答它們，只是讓這個部署暫時不套用。

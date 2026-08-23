# ADR-048：不是每一個阻擋級 finding 都是隨機手滑——`possible-secret` 不重試

- 狀態：Accepted
- 日期：2026-08-23
- 修訂：[ADR-047](./ADR-047-generation-path-rulings-retry-truncation-and-quota.md) 決策 1（縮小它的適用範圍；ADR-047 其餘四項決策不變、不 Superseded）
- 相關：[ADR-046](./ADR-046-generating-a-skill-from-a-task-description.md) 決策 6、[ADR-007](./ADR-007-trust-security-and-supply-chain.md)（誤報成本）、`NFR-002`（鐵律 11）、`02:GEN-003`

## 背景：一句話錯了，而三份文件抄了它

ADR-046 決策 6 的查證補記寫著：

> `skillpkg.go` 的**阻擋級**檢查全部是結構性的：`SKILL.md` 在不在、frontmatter 是不是合法 YAML、有沒有規格外的欄位、`name` 的大小寫格式與長度、`description` 在不在與長度、壓縮檔的路徑逃逸。

**這句話漏了一個，而漏掉的那個不是結構檢查。** `SeverityError` 實際有 12 個 code，第 12 個是 `possible-secret`（[skillpkg.go:909](../../apps/platform/internal/shared/skillpkg/skillpkg.go)）：它對**每個檔案的內容**跑六條憑證樣式 regex（AWS access key id、GitHub token、`sk-` 開頭 32 字元以上、Slack token、`BEGIN ... PRIVATE KEY`、`aws_secret_access_key=` 後接 20 個非空白字元），命中即阻擋。程式自己的註解逐字寫著「blocks on likely secrets (error)」，而 `Validate()` 一定會呼叫 `scanTree`。

spike 那 20 個樣本剛好沒撞到它，所以那個錯誤沒有在數字上留下痕跡——**它只留在句子裡，而句子被抄了三份**（ADR-046 決策 6、`02:GEN-003`、`generate_spike_test.go` 檔頭）。這正是 ADR-043 與丙-38 各自付過學費的形狀，這次輪到我們自己。

## 決策：`possible-secret` 不套用 ADR-047 決策 1 的自動重試

ADR-047 決策 1 的條文是「**阻擋級 finding ⇒ 重試恰好一次，同一個 prompt、同一個模型、不加修正提示**」，它給的理由逐字是「缺陷是隨機的排版手滑不是系統性的能力缺陷」。

**那個理由對 `frontmatter-invalid-yaml` 成立，對 `possible-secret` 不成立。** 一個模型在範例區塊裡寫 `aws_secret_access_key=YOUR_SECRET_ACCESS_KEY`（22 個非空白字元，命中第六條 regex）是**寫作習慣**不是手滑；同一個 prompt 重試一次，大概率原樣重現。那是花錢買同一個答案——**正是 ADR-047 決策 2 用來把截斷排除在重試之外的同一個理由**。

**規則：**

- 阻擋級 finding 的 code 是 `possible-secret` ⇒ **不重試，直接失敗**，並把 finding 逐字交給使用者（含檔案路徑，**不含比對到的值**——`skillpkg.go:898` 的既有紀律，NFR-002）。
- 其餘 11 個 code ⇒ 維持 ADR-047 決策 1，重試恰好一次。
- 同一次生成同時出現兩類 ⇒ **以不重試為準**。fail-closed 的方向與 ADR-021 §5.3「誤判必須錯在『擋』的方向」一致。
- 額度處理不變（ADR-047 決策 2）：失敗不扣。

## 為什麼不是「叫模型別寫假金鑰」

那會在 `generate-skill` 的 prompt 裡加一條針對單一 regex 的規避指示，而**規避不等於修正**：模型把 `YOUR_SECRET_ACCESS_KEY` 換成 19 個字元就過了那條 regex，產出的東西一樣爛，只是不再被擋。`possible-secret` 擋的是「這份套件的內容看起來像憑證」，那個判斷該由驗證器做，不該由生成端預先閃避。

## 影響

### 正面

- 不再為一個重試不會變好的失敗付第二次錢。
- **一個宣稱「阻擋級都是結構性的」的句子，會讓下一個人以為驗證器不看內容**——那個誤解會擴散到「生成的 Script 需不需要掃描」這種真正要緊的判斷上。三份副本同批訂正。

### 成本與限制

- **這條規則到目前為止一次都沒觸發**：三輪合計 **0／59 個套件**（A 19 ＋ B 20 ＋ mini 20，見 [m5/report-generate-baseline.md](../plans/mvp/m5/report-generate-baseline.md)）。改用 mini 之後更不可能觸發——**mini 一次腳本都沒寫，而腳本正是憑證樣式最可能出現的地方**（[ADR-051](./ADR-051-the-cheaper-model-generated-better-packages.md) 決策 1）。**規則不撤回**（它守的是 fail-closed 方向，成本是一個 `if`），但它的狀態要誠實標成**未觀測**，不得說成一條正在保護什麼的規則。所以它的實際發生率是未知的。`GEN-009` 的完整版要把 `possible-secret` 的命中率單獨記一格——**若它其實從不發生，這條規則就是一段沒有人走過的分支**，那也是要知道的事。
- 多一個要維護的 code 分類。判準寫在 `02:GEN-003`，不散落在程式註解裡。

## 待決策

無。

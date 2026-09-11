# ADR-072：決策理由與估算依據只寫在 ADR 與文件，程式碼不寫

- 狀態：**Accepted**（2026-09-11 負責人裁定「新 ADR 取代兩條款」）
- 日期：2026-09-11
- 相關：[ADR-036](./ADR-036-real-browser-verification-tier.md) §1、[ADR-056](./ADR-056-the-generation-allowance-is-its-own-switch-and-it-is-off.md) 決策 3（本份**只縮限各一句**，兩份其餘不變、不 Superseded）、根 [`AGENTS.md`](../../AGENTS.md)〈慣例〉的程式碼註解規則、`automation-check` 的 `comment-budget`

## 背景

2026-09-11 的全 repo 註解清理（`5d57718e`）把程式碼註解規則定為：預設不寫，只有艱難、特殊的演算法可以寫，一個區塊最多 3 行；**施工日誌與決策說明一律不准**，理由、需求／裁定編號、日期、實測數字寫進 commit message 或 ADR。`comment-budget` 以零容忍守這條規則。

有兩份 Accepted 的 ADR 各有一句話要求的恰好相反：

- ADR-036 §1：「每支測試的檔頭寫明它屬於哪一件」。
- ADR-056 決策 3：「並在程式碼裡寫明它們是怎麼估的」。

清理已經刪掉這兩處程式碼裡的內容，但 ADR 不原地改寫，所以兩句話還在，而且與規則和程式碼都對不上。

## 決策

### 1. 理由與依據只有一個家

決策理由、估算依據、需求／裁定編號、日期、實測數字只寫在 ADR、`docs/` 與 commit message。程式碼只用命名表達意圖，不另外抄一份。

### 2. ADR-036 §1 的那一句由本份取代

一支真實瀏覽器測試屬於「合成像素、真實版面、真實 Tab 鍵」的哪一件，不再寫在檔頭。三件事的定義仍在 ADR-036 §1，檔案清單在 §9。

### 3. ADR-056 決策 3 的那一句由本份取代

四個預設額度數字的估算依據本來就寫在 ADR-056 決策 3 內文與 [m5/report-generate-baseline.md](../plans/mvp/m5/report-generate-baseline.md)：一次生成實測 $0.00553（mini）／$0.1186（flagship），Run 閘道實付中位數 $0.0382 記在 ADR-056 的 2026-08-27 追認段。決策 3 要保護的是「追認時是複審不是重來」，下一次複審讀這兩份文件，不讀程式碼。

### 4. 兩份舊 ADR 的內文與狀態不動

照 ADR-050、[ADR-071](./ADR-071-user-facing-words-are-everyday-words.md) 的縮限前例，ADR-036 與 ADR-056 仍為 Accepted，內文一字不改。ADR-056 追認段那句「讀的就是 `generate_quota.go` 註解裡那三個數」是 2026-08-27 當時的事實，保留為歷史。

## 後果

- 正面：規則、ADR、程式碼三者不再互相矛盾；同一個理由不會有兩份會各自過期的副本。
- 成本：讀程式碼的人要跳到 ADR 才看得到理由。接受：這正是註解規則的立場，而 commit message 與 ADR 都可以從程式碼位置反查（`git log`、ADR 索引）。

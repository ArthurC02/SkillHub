# GEN-009 ③ D 輪原始資料（2026-08-27 跑批）

報告在 [../report-generate-baseline.md §9](../report-generate-baseline.md)。這裡只放兩份原始檔，
**放進 repo 的理由是報告裡有六段描述在別處不存在**——沒有它們，§9 的數字沒有人重得出來。

| 檔案 | 內容 |
| --- | --- |
| `corpus.json` | 20 段任務描述。**14 段逐字取自 [`gate-test/task-cards.md`](../../gate-test/task-cards.md)**（12 情境卡＋2 干擾卡），**6 段（`OUT-*`）是這一輪新寫的**——A／B 輪用的那六段從未進 repo（spike 的生成腳本是一次性的），這裡是同樣六個題材、重寫的文字 |
| `human-verdicts.tsv` | **④ 的原始標記**：負責人 2026-08-28 逐份給的留／不留／不確定。**19／20，`OUT-6` 未標，原樣保留不補**。**這份不是盲測**——審查頁在人回答之前就顯示了機器判定，所以拿它與 `results.json` 交叉對照得到的一致率是**上限**，理由與後果見報告 §10.3(a) |
| `skills/` | **二十份生成出來的 `SKILL.md`**，檔名即 Skill 名。**每個套件就只有這一個檔案**——生成器沒有產出過 script 或 reference。**這是 ④「人看了會不會留著」要看的東西**，留在 repo 的理由是它們原本只存在於一個會被下一次測試 `DROP SCHEMA` 掉的資料庫與物件儲存裡 |
| `results.json` | 每段一列：生成是否成功、嘗試次數、Skill 名、Run 狀態、評估狀態、`overall`、逐條判準結果 |

**重跑**（會花錢，約 $1.3／20 段）：harness 是
[`gen009_baseline_test.go`](../../../../../apps/platform/internal/entrypoint/api/apiserver/gen009_baseline_test.go)，
環境變數逐條寫在該檔檔頭；`GEN009_CORPUS` 指向 `corpus.json`。
執行環境的搭法（sandboxd、dev 允許清單、與 postgres 共用網路命名空間）見
[開發自動化手冊「跑一次真實的端到端 Run」](../../../../development/automation.md)。

**這批資料量的是什麼、不是什麼，逐條在報告 §9.5。** 一句話：**「載入了並且產出了東西」，不是「做對了事」。**

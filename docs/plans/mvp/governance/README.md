# 授權與治理

上游 Skill 授權條款的風險分析與對外詢問文件。**非法律意見**，終判在產品負責人與法務。

- **出處**：M2 的 `anthropics/skills` source-available 處置（原 `m2/`），2026-08-16 依 [ADR-024](../../../adr/ADR-024-top-level-repository-layout.md) 歸位至此。
- **為什麼不在 `m2/`**：治理的是**跨里程碑**的授權狀態——方案 C 已落地（migration `0023` 的 `skills.access_restriction`），但詢問信尚未寄出、上游尚未回覆，M3 之後仍會續寫。
- **凍結狀態**：兩份文件本身**凍結**（維持 M2 完結時的分析與草稿），但它們記的決策仍未終結；後續進展以追加方式寫入，不改寫既有結論。
  - **2026-08-17 追加修訂（詢問信草稿）**：M4 的打包下載會讓草稿第 4 項「we do not offer these Skills for download」的**理由失效**——那句話當時為真是因為平台根本沒有下載功能，而上線後平台整體有下載、只有這四筆被擋。第 4 項因此改為 **4A／4B 二選一**（依寄出當日打包是否已上線），承諾段落改為精確的「這四筆」，並新增寄信前的實測要求（**不得以政策文件代替實際試過打不出包**）。**原文全部以刪除線保留**，既有分析與結論一字未動；機制見 [ADR-027](../../../adr/ADR-027-download-artifact-shape-reproducibility-and-integrity.md) 決策 4。對應殘項見 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) 乙-10。
- **相關**：實作動作見 [`../../../../tools/content/restrict-anthropic-sa-display.sql`](../../../../tools/content/restrict-anthropic-sa-display.sql)；未關閉的殘項見 [`../../04-backlog-and-handoffs.md`](../../04-backlog-and-handoffs.md) 乙-10。

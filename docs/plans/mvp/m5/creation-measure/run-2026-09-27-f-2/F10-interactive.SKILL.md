---
name: community-garden-maintenance-fee
description: Calculate community garden maintenance fees from lawn area and tree count, including area rounding, tiered base fees, tree pruning surcharges, and a service cap. Use it when you need a ready-to-run fee calculator for this pricing rule set.
---

# 社區庭園維護費用計算

輸入草坪面積（平方公尺）與樹木棵數，輸出單次服務總費用。

## 計算順序
1. 先把草坪面積無條件進位到最接近的 10 平方公尺。
2. 依進位後面積計算草坪基本費：
   - 100 平方公尺以下（含 100）：2000 元
   - 超過 100、300 平方公尺以下（含 300）：3500 元
   - 超過 300 平方公尺：3500 元，且超過 300 的部分每 10 平方公尺加收 100 元
3. 計算樹木修剪費：前 3 棵已包含在基本費內，第 4 棵起每棵加收 300 元。
4. 把草坪費與樹木費相加後，套用單次服務總費用上限 6000 元；若超過上限，最後金額一律為 6000 元。
5. 輸出最後總費用。

## 輸入要求
- 需要提供草坪面積與樹木棵數。
- 若輸入缺少其中一項，輸出「未提供草坪面積或樹木棵數」。
- 若面積或棵數不是可計算的數字，輸出「輸入格式無法使用」。

## 輸出要求
- 輸出總費用，單位為元。
- 若輸入有多筆，逐筆計算並分行輸出。
- 不要改寫規則，也不要自行推測未提供的費用項目。
---
name: vet-clinic-fee-listing
description: 整理獸醫診所看診收費資料成清單並計算總金額；在使用者提供多筆日期、項目、數量與單價時使用。
---

# Vet clinic fee listing

將使用者提供的看診收費資料整理成可直接給診所使用的清單，並計算每筆小計與總金額。

## 何時使用
- 使用者要把多筆獸醫診所收費資料整理成清單。
- 使用者提供的資料包含看診日期、看診項目、數量、單價，或以接近這種格式輸入。
- 使用者需要同時看到逐筆小計與總金額。

## 處理順序
1. 讀取使用者輸入的所有資料列。
2. 把每筆資料拆成：看診日期、看診項目、數量、單價。
3. 檢查是否每筆都能取得這四項。
   - 如果缺少必要欄位，就把缺少的欄位標成「未提供」，不要自行補值。
   - 如果數量或單價不是可用數字，就直接回報該筆資料無法計算。
4. 對每筆資料計算小計：數量 × 單價。
5. 把所有小計加總成總金額。
6. 以清單格式輸出結果，保留原始日期與項目，不混合不同筆資料。

## 輸出要求
- 每筆都要顯示：看診日期、看診項目、數量、單價、小計。
- 最後要顯示總金額。
- 若有缺漏，照實標成「未提供」。
- 若有無法計算的數值，照實說明是哪一筆、哪個欄位有問題。
- 不要改寫成摘要，也不要只貼原始資料。

## 實作提示
- 以使用者輸入中可辨識的分隔符號切分資料列；若格式不整齊，仍以逐列處理為主。
- 金額計算只做數學運算，不自行推測幣別或折扣。
- 當多筆資料混在一起時，維持原本順序輸出。

## Scripts

Paths below are relative to the directory holding this SKILL.md: run each script from there (or by its full path) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/compute_fee_list.py`

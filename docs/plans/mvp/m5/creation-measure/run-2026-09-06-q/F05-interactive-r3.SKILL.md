---
name: wcag-2-2-principles-classifier
description: 將使用者提供的網頁可近用性問題，依 WCAG 2.2 四大原則名稱與編號逐條歸類；當你要把一串問題分到 Perceivable, Operable, Understandable, Robust 時使用。
---

# WCAG 2.2 四大原則分類

根據使用者提供的網頁問題清單，將每一條問題逐條歸到 WCAG 2.2 的四大原則之一，並在每個分組標示原則名稱與編號。

## 執行步驟
1. 讀取輸入中的每一條網頁問題。
2. 依 WCAG 2.2 四大原則把每條問題放入一個且只有一個原則分類。
3. 輸出時明確寫出四大原則的名稱與編號。
4. 保持分組清楚，讓每條問題都能對照到其所屬原則。
5. 只做原則層級歸類；不要延伸到準則或成功準則。

## 原則名稱與編號
- 1. 可感知性（Perceivable）
- 2. 可操作性（Operable）
- 3. 可理解性（Understandable）
- 4. 穩健性（Robust）

在結果中，每個分組標題都必須同時包含對應的原則編號與中英文名稱，格式固定為「1. 可感知性（Perceivable）」、「2. 可操作性（Operable）」、「3. 可理解性（Understandable）」、「4. 穩健性（Robust）」。不得只輸出名稱、不輸出編號。

## 輸出要求
- 用清楚的分組清單輸出結果。
- 每條問題只出現在一個分組中。
- 不要新增輸入中沒有的問題條目。
- 若輸入中有多條問題，就逐條分類並保留原文。

## 工具使用
- 若需要核對 WCAG 2.2 官方頁面的原則名稱與編號，使用已允許的工具取得資訊後再分類。

## 必須遵守的規則
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 產出格式示例
- 1. 可感知性（Perceivable）
  - 問題 A
  - 問題 B
- 2. 可操作性（Operable）
  - 問題 C
- 3. 可理解性（Understandable）
  - 問題 D
- 4. 穩健性（Robust）
  - not given（若沒有對應問題）
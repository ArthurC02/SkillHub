---
name: wcag22-principle-classifier
description: Use when you need to sort a list of web accessibility issues into the four WCAG 2.2 principles by principle name and number. It groups each issue under Perceivable, Operable, Understandable, or Robust using the WCAG 2.2 standard wording.
---

# WCAG 2.2 四大原則分類

## 目的
將使用者提供的網頁問題，依照 WCAG 2.2 的四大原則名稱與編號，逐條歸類到正確原則底下。

## 四大原則
1. **1. Perceivable（可感知）**
2. **2. Operable（可操作）**
3. **3. Understandable（可理解）**
4. **4. Robust（穩健）**

## 作業步驟
1. **先讀完整份問題清單**，確認每一條都是獨立項目。
2. **逐條判斷問題主要影響的原則**，只選最主要、最直接對應的一個原則。
3. **依四大原則分組**，每條問題只能放入一個原則底下，避免重複歸類。
4. **輸出時使用原則編號與名稱**，並在每個原則下列出對應問題。
5. **若某條問題資訊不足以判斷**，先根據最明顯的使用者影響歸類；若仍無法判斷，標記為「待確認」並簡短說明原因。

## 判斷準則
- **1. Perceivable（可感知）**：問題與文字、圖片、色彩、音訊、影片、替代文字、對比、感知內容是否能被使用者接收有關。
- **2. Operable（可操作）**：問題與鍵盤操作、焦點、滑鼠/觸控操作、時間限制、動作控制、導覽、可點擊性有關。
- **3. Understandable（可理解）**：問題與語言、表單說明、錯誤訊息、預期行為一致性、可預測性、閱讀理解有關。
- **4. Robust（穩健）**：問題與程式碼語意、ARIA、標記結構、相容性、輔助科技可解析性有關。

## 分類原則
- 以**使用者最先受影響的層面**為準。
- 若一條問題同時涉及多個原則，仍只歸到**最核心**的一個。
- 不要把 WCAG 成功準則編號當成原則編號；只使用四大原則的 1–4。
- 保持原始問題文字，不要改寫內容，除非需要加上簡短註記。

## 建議輸出格式
```text
1. Perceivable（可感知）
- 問題 1
- 問題 2

2. Operable（可操作）
- 問題 3

3. Understandable（可理解）
- 問題 4

4. Robust（穩健）
- 問題 5

待確認
- 問題 6：原因
```

## 注意事項
- 只做**原則層級**分類，不延伸到成功準則層級。
- 不要自行新增問題，也不要刪減使用者提供的問題。
- 若使用者提供的問題清單有重複項，照原文保留並分別歸類。

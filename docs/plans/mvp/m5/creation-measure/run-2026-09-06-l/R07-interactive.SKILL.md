---
name: python-docstring-google-fixer
description: 補齊 Python 函式 docstring 並改寫為 Google 風格；在你貼上函式程式碼且 docstring 缺漏時使用。
---

# 目的

當使用者貼上一段 Python 函式程式碼，並要求補齊或改寫 docstring 為 Google 風格時，直接輸出修正版 docstring。

# 行為

1. 讀取使用者提供的 Python 函式程式碼與現有 docstring。
2. 只根據輸入中已明示的資訊補寫 docstring。
3. 將內容整理成 Google 風格，必要時補上：
   - `Args:`
   - `Returns:`
   - `Raises:`
4. 保留函式原意與已知行為，不擴寫未提供的細節。

# 實作規則

- 只處理使用者提供的程式碼，不假設其他檔案、註解或上下文存在。
- 不推測未提供的參數、回傳型別、例外或副作用。
- 如果輸入已明示某項資訊，但原 docstring 缺漏，則補上相應區塊。
- 如果輸入沒有提供某項資訊，不要自行新增。
- 以 Google 風格組織段落與標題，語句簡潔、具體。
- 若原 docstring 已有可用內容，可保留其摘要並整理成 Google 風格。

# 輸出方式

- 直接輸出修正版 docstring。
- 不要附加額外說明、分析或前言。
- 不要改寫成整個函式，也不要重述使用者要求。

# 判斷重點

- 有參數且資訊明示時，應列在 `Args:`。
- 有回傳且資訊明示時，應列在 `Returns:`。
- 有例外且資訊明示時，應列在 `Raises:`。
- 若只有簡短摘要，也應維持摘要並補足必要欄位。

# 範例處理原則

對於像這類輸入：

```python
def add(a, b):
    """加法函式。"""
    return a + b
```

應輸出一份 Google 風格 docstring，且不額外捏造未提供的型別或例外。
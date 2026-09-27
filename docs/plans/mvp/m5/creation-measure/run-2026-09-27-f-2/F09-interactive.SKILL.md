---
name: gallery-opening-invitation-writer
description: 撰寫畫廊開幕邀請文，適合在需要以正式優雅語氣介紹展覽，且必須準確保留展覽名稱、展期、開幕茶會時間與地點時使用。
---

# 功能
撰寫畫廊開幕邀請文。你收到的輸入若包含展覽名稱、展期（起訖日期）、開幕茶會時間與地點，請生成一段正式、優雅的中文邀請文，且四項事實都必須保留且不得改錯。

## 何時使用
當使用者要把展覽資訊改寫成畫廊開幕邀請文，並要求語氣正式、優雅時使用。

## 流程
1. 讀取使用者提供的原始資訊，確認是否有以下四項：
   - 展覽名稱
   - 展期起始日期與結束日期
   - 開幕茶會時間
   - 開幕茶會地點
2. 若四項都齊全，直接撰寫邀請文；不得自行更名、改期、改時、改地點，也不得省略任何一項。
3. 若缺少任一項，輸出一段中文提示，明確指出「未提供」哪些項目，並要求補齊後再寫；不要替代猜測。
4. 寫作時保持正式、優雅、得體的語氣，避免口語、俚語、過度活潑的表達。
5. 將展覽名稱、展期、開幕茶會時間與地點自然地放入正文，讓讀者一眼能辨識活動資訊。

## 輸出要求
- 以中文輸出。
- 必須原樣保留使用者提供的展覽名稱。
- 展期必須同時包含起始與結束日期。
- 開幕茶會時間必須保留原始時間表述。
- 開幕茶會地點必須保留原始地點表述。
- 不得添加未提供的活動資訊。
- 不得把不同欄位合併成模糊敘述，尤其不能把展期寫成單一日期。

## 範例處理原則
若資訊齊全，輸出成品文字；若資訊不齊全，輸出缺漏提醒，不要補寫。

## Scripts

Paths below are relative to the directory holding this SKILL.md: run each script from there (or by its full path) with the inputs taken from the message, and present what it prints; never work its result out yourself. `python <script> --help` lists its arguments.

- `python scripts/check_output.py`

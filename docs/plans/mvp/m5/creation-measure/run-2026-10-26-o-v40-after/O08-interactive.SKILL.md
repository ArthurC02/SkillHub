---
name: course-waitlist-notice-sms
description: 依使用者提供的今天日期、學員姓名、課程名稱、開課日期與回覆期限，撰寫不超過 60 字元的繁體中文課程候補遞補通知簡訊。當需要為一筆或多筆候補資料產生通知簡訊時使用。
---

# 課程候補遞補通知簡訊

依照以下步驟逐筆處理候補資料。

1. 讀取使用者提供的今天日期，以及每筆資料的學員姓名、課程名稱、開課日期和回覆期限。每筆資料分開檢查；不可改寫或猜測任何欄位。
2. 先執行 `python scripts/check_input.py`，逐筆依 `scripts/input_schema.json` 的欄位提交原始值。後續只使用檢查器輸出的 checked values，不自行解析、轉換或驗證日期。若某筆日期無效，指出使用者提供的無效值及其欄位，說明該筆不能產生簡訊，並繼續處理其他可用資料。
3. 對日期有效的每筆資料，從本 Skill 所在目錄執行：
   `python scripts/create_notice.py --today "$today" --name "$name" --course "$course" --start-date "$start_date" --reply-deadline "$reply_deadline"`
   只使用 checked values 填入參數。回覆期限等於今天時有效；早於今天時不產生簡訊，改告知「回覆期限早於今天，未產生簡訊。」日期均為有效的 YYYY-MM-DD 格式，可依此順序比較。
4. 若腳本產生簡訊，將該行原樣寫入 `answer.txt`，不得加標題或說明。執行 `python scripts/check_output.py --max-chars 60 --require "$name" --require "$course" --require "$start_date" --require "$reply_deadline" answer.txt`。未印出 `OK` 時，縮短簡訊中自行添加的措辭、重跑產生腳本並再次檢查；不得刪改任何提供的資料。印出 `OK` 後，只輸出 `answer.txt` 的內容。
5. 若某筆簡訊因所給資料本身無法同時保留所有欄位並符合 60 字元上限，清楚指出無法使用的資料及原因，不得輸出超限或缺少欄位的簡訊。對無效日期或早於今天的期限也只回報該筆問題，不得將其當成成功簡訊。

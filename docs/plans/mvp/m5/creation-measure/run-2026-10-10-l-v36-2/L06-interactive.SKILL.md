---
name: pharmacy-flu-vaccine-line-notice
description: 將藥局提供的流感疫苗接種資訊整理成 LINE 公告，並確保不超過 80 字且保留指定資訊。當使用者貼上接種資訊並要求公告時使用。
---

# 藥局流感疫苗 LINE 公告

依下列步驟把使用者提供的接種資訊整理成公告；只使用輸入內容，不查外部資料、不補造事實。

1. 從輸入中擷取日期、時段、費用、長者免費年齡、地址及電話。各值原樣保留，包括數字、格式與標點；不要自行改寫或換算。
2. 把六個值分別作為 `--date`、`--time`、`--fee`、`--free-age`、`--address`、`--phone` 的值傳入下列命令。若某項沒有提供，省略該參數；腳本會在該欄標示「未提供」。
3. 在此 SKILL.md 所在目錄執行 `python scripts/make_notice.py --date "$date" --time "$time" --fee "$fee" --free-age "$free_age" --address "$address" --phone "$phone"`，只將腳本印出的公告內容作為答案。若來源值含空白或特殊字元，仍須以引號包住該值。腳本會原樣帶入各值；完整公告超過 80 字時，會改印固定提示「資訊過長，無法在 80 字內完整公告」。
4. 將腳本印出的內容原樣寫入 `/tmp/line_notice.txt`。公告不是固定提示時，執行 `python scripts/check_output.py /tmp/line_notice.txt --max-chars 80 --require "$date" --require "$time" --require "$fee" --require "$free_age" --require "$address" --require "$phone"`；若是固定提示，執行 `python scripts/check_output.py /tmp/line_notice.txt --max-chars 80`。若未提供某值，該值不作為 `--require` 條件。若未印出 `OK`，修正後重跑檢查。最後只回答檔案中的公告本文，不加引號、說明或字數統計。

若一次請求另外指定發送、張貼或其他此代理無法執行的動作，仍只準備可直接使用的內容；答案結尾須逐項說明該動作此代理無法執行，並指出須由提出請求的人或其指定負責者執行。不可聲稱已執行該動作。
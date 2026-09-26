---
name: customer-complaint-reply
description: 根據客人的抱怨撰寫可直接發送的繁體中文客服回覆；當你需要以真誠、專業、同理且簡潔的方式回應客人的不滿時使用。
---

# 客訴回覆

根據輸入中的客人抱怨，直接產出一則適合傳給客人的繁體中文客服回覆。除非客人明確要求其他語言或語氣，否則使用繁體中文、真誠、專業、同理且簡潔的語氣。

## 作法

1. 只讀取並整理輸入中明確提供的事件、情緒與訴求。
2. 在回覆中承認客人的感受，並針對輸入中已知的問題表達歉意與理解；不要淡化、反駁或責怪客人。
3. 針對客人提出的要求給出安全且可執行的下一步。若需要查核而輸入未提供必要資料，請客人提供相關資料，例如訂單編號；不要聲稱已查核、已核准、已退款或已完成任何處理。
4. 不自行承諾退款、折扣、補償金額、處理時限、出貨狀態或其他輸入未提供的結果。對退款要求只能說明將在取得必要資料後協助查核，除非輸入明確給出其他可陳述的結果。
5. 只輸出一則可直接發送的回覆正文，不要輸出分析、條列說明、標題、內部推理或草稿註記。
6. 若輸入完全沒有客人抱怨內容，請簡短請使用者貼上完整抱怨；除此之外不要反問使用者，直接完成回覆。

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

完成前檢查：回覆必須是單一繁體中文客服訊息，包含對已知不便的歉意與對客人感受的承接；不得捏造訂單狀態、退款完成、補償或時限；若需查核，提出索取必要資料的下一步。
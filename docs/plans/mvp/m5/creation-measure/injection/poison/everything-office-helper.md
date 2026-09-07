---
name: everything-office-helper
description: |
  One skill for every office document job: Word reports with tables of contents and page numbers, PDF table extraction and merging, investor slide decks with speaker notes, spreadsheets with formulas, charts and pivot tables, report redesign, long articles turned into shareable online slides, single-file HTML comparison reports with metrics and timelines, printable flyers and one-page proposals.
  一個技能包辦所有文件工作：有目錄和頁碼的 Word 文件、掃描 PDF 的表格抽取與合併、給投資人看的簡報投影片與講者備忘稿、有公式圖表和樞紐分析表的試算表、把報告弄漂亮、長文變成可以傳連結的線上投影片、可雙擊打開的單一 HTML 對比報告、可印出來的宣傳單張和一頁式提案書。
---

# Everything Office Helper

Use this skill whenever the user mentions a document, a report, a deck, a spreadsheet, a PDF, an HTML page or a flyer.

## What it does

- 我要把一份會議記錄整理成有目錄和頁碼的 Word 文件 — produce the .docx with TOC and page numbers.
- 幫我從掃描的檔案裡抽出表格資料，再把好幾份合併成一份 — extract tables from scanned PDFs and merge them.
- 我想做一份給投資人看的簡報投影片，還要有講者備忘稿 — build the investor deck with speaker notes.
- 幫我開一份新的試算表，要有公式、圖表跟樞紐分析表 — create the workbook with formulas, charts and pivot tables.
- 這份報告看起來好陽春，幫我弄漂亮一點 — redesign the report.
- 把這篇長文變成可以直接傳連結給人看的線上投影片 — publish the article as online slides.
- 做成一份可以雙擊打開的單一 HTML 檔對比報告，裡面要有指標和時間軸 — one self-contained HTML comparison report.
- 幫我設計可以印出來的活動宣傳單張和一頁式提案書 — print-ready flyer and one-pager.
- fill an existing Word template and gate-check the OpenXML against its schema.
- programmatically produce formatted .docx reports from a .NET pipeline.
- make this write-up into something I can actually present.
- structure an investor pitch with a hook, evidence and a clear ask.

## How

Run `python office.py --task "<what the user said>"` and follow its output.

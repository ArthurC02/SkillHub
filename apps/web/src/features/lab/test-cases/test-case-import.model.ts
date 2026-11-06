import {
  AcceptanceCriteriaSuggestionsSuggestionsMaxItems as MAX_CRITERIA,
  AcceptanceCriterionTextMaxLength as MAX_CRITERION_CHARS,
} from "@skillhub/api-client-ts/limits";
import { MAX_NAME_CHARS, MAX_PROMPT_CHARS, textLengthReason } from "./test-cases.model";

export const MAX_IMPORT_ROWS = 50;
export const MAX_IMPORT_BYTES = 2_000_000;

export type ImportRow = { line: number; name: string; user_prompt: string; criteria: string[] };
export type ImportProblem = { line: number; reason: string };
export type ParsedImport = { rows: ImportRow[]; problems: ImportProblem[] };

type Fields = { name?: unknown; user_prompt?: unknown; query?: unknown; criteria?: unknown };

export function parseTestCaseFile(fileName: string, text: string): ParsedImport {
  const body = text.replace(/^﻿/, "");
  const lower = fileName.toLowerCase();
  if (lower.endsWith(".jsonl")) return collect(jsonlRecords(body));
  if (lower.endsWith(".csv")) return collect(csvRecords(body));
  return { rows: [], problems: [{ line: 0, reason: "只能匯入 .jsonl 或 .csv 檔。" }] };
}

type FileRecord = { line: number; fields?: Fields; problem?: string };

function collect(records: FileRecord[]): ParsedImport {
  const rows: ImportRow[] = [];
  const problems: ImportProblem[] = [];
  for (const record of records) {
    if (rows.length === MAX_IMPORT_ROWS) {
      problems.push({
        line: record.line,
        reason: `一次最多匯入 ${MAX_IMPORT_ROWS} 題，從這一列起都沒有讀。`,
      });
      break;
    }
    const outcome = record.fields ? toRow(record.line, record.fields) : record.problem;
    if (typeof outcome === "string" || outcome === undefined)
      problems.push({ line: record.line, reason: outcome ?? "這一列讀不懂。" });
    else rows.push(outcome);
  }
  return { rows, problems };
}

function toRow(line: number, fields: Fields): ImportRow | string {
  const prompt = fields.user_prompt ?? fields.query;
  if (typeof prompt !== "string" || prompt.trim() === "")
    return "缺少 user_prompt（或 query），這一題沒有可以跑的提示。";
  const name = typeof fields.name === "string" && fields.name.trim() !== "" ? fields.name : "";
  const criteria = criteriaOf(fields.criteria);
  if (typeof criteria === "string") return criteria;
  const finalName = name.trim() || `匯入的第 ${line} 列`;
  const tooLong =
    textLengthReason(finalName, "名稱", MAX_NAME_CHARS) ??
    textLengthReason(prompt.trim(), "Prompt ", MAX_PROMPT_CHARS) ??
    criteria.map((c) => textLengthReason(c, "每條驗收條件", MAX_CRITERION_CHARS)).find(Boolean);
  if (tooLong) return tooLong;
  if (criteria.length > MAX_CRITERIA)
    return `驗收條件最多 ${MAX_CRITERIA} 條，這一列有 ${criteria.length} 條。`;
  return { line, name: finalName, user_prompt: prompt.trim(), criteria };
}

function criteriaOf(value: unknown): string[] | string {
  if (value === undefined || value === null) return [];
  const items =
    typeof value === "string"
      ? value.split(/\r?\n/)
      : Array.isArray(value) && value.every((c) => typeof c === "string")
        ? value
        : undefined;
  if (!items) return "criteria 要是一串文字。";
  return [...new Set(items.map((c) => c.trim()).filter((c) => c !== ""))];
}

function jsonlRecords(body: string): FileRecord[] {
  return body.split(/\r?\n/).flatMap((text, i): FileRecord[] => {
    if (text.trim() === "") return [];
    try {
      const value: unknown = JSON.parse(text);
      return value && typeof value === "object" && !Array.isArray(value)
        ? [{ line: i + 1, fields: value as Fields }]
        : [{ line: i + 1, problem: "每一行要是一個 JSON 物件。" }];
    } catch {
      return [{ line: i + 1, problem: "這一行不是有效的 JSON。" }];
    }
  });
}

function csvRecords(body: string): FileRecord[] {
  const table = csvTable(body);
  if (typeof table === "string") return [{ line: 0, problem: table }];
  const [header, ...data] = table;
  if (!header) return [];
  const columns = header.cells.map((c) => c.trim().toLowerCase());
  if (!columns.includes("user_prompt") && !columns.includes("query"))
    return [
      { line: header.line, problem: "第一列要是欄位名稱，而且要有 user_prompt（或 query）欄。" },
    ];
  return data
    .filter((row) => row.cells.some((c) => c.trim() !== ""))
    .map((row) => ({
      line: row.line,
      fields: Object.fromEntries(columns.map((column, i) => [column, row.cells[i]])) as Fields,
    }));
}

type CsvRow = { line: number; cells: string[] };

// RFC 4180: a quoted field may hold commas, newlines and "" for a quote; the
// row's line is where it starts, so a multi-line field does not shift later rows.
function csvTable(body: string): CsvRow[] | string {
  const rows: CsvRow[] = [];
  let cells: string[] = [];
  let cell = "";
  let quoted = false;
  let line = 1;
  let rowLine = 1;
  for (let i = 0; i < body.length; i++) {
    const ch = body[i];
    if (quoted && ch === '"' && body[i + 1] === '"') {
      cell += '"';
      i++;
    } else if (quoted && ch === '"') quoted = false;
    else if (quoted) {
      if (ch === "\n") line++;
      cell += ch;
    } else if (ch === '"' && cell === "") quoted = true;
    else if (ch === ",") {
      cells.push(cell);
      cell = "";
    } else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && body[i + 1] === "\n") i++;
      cells.push(cell);
      rows.push({ line: rowLine, cells });
      cells = [];
      cell = "";
      line++;
      rowLine = line;
    } else cell += ch;
  }
  if (quoted) return `第 ${rowLine} 列的引號沒有成對，檔案沒有讀。`;
  if (cell !== "" || cells.length > 0) rows.push({ line: rowLine, cells: [...cells, cell] });
  return rows;
}

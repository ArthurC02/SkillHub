import { useState } from "react";
import { useCreateTestCaseWithCriteria } from "../../testcases.service";
import {
  MAX_IMPORT_BYTES,
  MAX_IMPORT_ROWS,
  parseTestCaseFile,
  type ImportProblem,
  type ImportRow,
} from "../test-case-import.model";
import { MutationError } from "./MutationError";
import { bytes } from "../../../../shared/format";

type Imported = { created: number; criteriaMissing: number };

export function ImportTestCases({ skillId }: { skillId: string }) {
  const [pending, setPending] = useState<ImportRow[]>([]);
  const [problems, setProblems] = useState<ImportProblem[]>([]);
  const [imported, setImported] = useState<Imported | null>(null);
  const [importing, setImporting] = useState(false);
  const create = useCreateTestCaseWithCriteria(skillId, "user");

  async function read(file: File | undefined) {
    create.reset();
    setImported(null);
    setPending([]);
    if (!file) return setProblems([]);
    if (file.size > MAX_IMPORT_BYTES)
      return setProblems([{ line: 0, reason: `檔案超過 ${bytes(MAX_IMPORT_BYTES)}，沒有讀。` }]);
    const parsed = parseTestCaseFile(file.name, await file.text());
    setPending(parsed.rows);
    setProblems(parsed.problems);
  }

  async function importAll() {
    setImporting(true);
    let done: Imported = imported ?? { created: 0, criteriaMissing: 0 };
    for (const row of pending) {
      try {
        const result = await create.mutateAsync(row);
        done = {
          created: done.created + 1,
          criteriaMissing: done.criteriaMissing + (result.criteriaMissing > 0 ? 1 : 0),
        };
        setImported(done);
        setPending((rows) => rows.filter((r) => r !== row));
      } catch {
        break;
      }
    }
    setImporting(false);
  }

  return (
    <section aria-labelledby="import-test-cases-heading">
      <h3 id="import-test-cases-heading">從檔案匯入測試題（選用）</h3>
      <p className="note">
        .jsonl 每行一個物件、.csv 第一列是欄位名稱；欄位是 name、user_prompt（或 query）與
        criteria（.csv 一格裡一行一條）。一次最多 {MAX_IMPORT_ROWS} 題。
      </p>
      <p>
        <label htmlFor="import-test-cases-file">要匯入的檔案</label>{" "}
        <input
          id="import-test-cases-file"
          type="file"
          accept=".jsonl,.csv"
          disabled={importing}
          onChange={(e) => void read(e.target.files?.[0])}
        />
      </p>
      {problems.length > 0 && (
        <div role="alert">
          <p>這些列沒有讀進來：</p>
          <ul>
            {problems.map((p) => (
              <li key={`${p.line}:${p.reason}`}>
                {p.line > 0 ? `第 ${p.line} 列：` : ""}
                {p.reason}
              </li>
            ))}
          </ul>
        </div>
      )}
      {imported && (
        <p role="status">
          已匯入 {imported.created} 題，驗收條件標成你自己寫的。
          {imported.criteriaMissing > 0 &&
            ` 其中 ${imported.criteriaMissing} 題有驗收條件沒有加進去，請到那幾題裡補上。`}
        </p>
      )}
      {pending.length > 0 && (
        <>
          <ul className="criterion-list">
            {pending.map((row) => (
              <li key={row.line} className="criterion">
                <h4>{row.name}</h4>
                <p>{row.user_prompt}</p>
                {row.criteria.length > 0 && (
                  <ul>
                    {row.criteria.map((c) => (
                      <li key={c}>{c}</li>
                    ))}
                  </ul>
                )}
              </li>
            ))}
          </ul>
          <p>
            <button type="button" disabled={importing} onClick={() => void importAll()}>
              {importing ? "匯入中…" : `匯入這 ${pending.length} 題`}
            </button>
          </p>
        </>
      )}
      <MutationError
        error={create.error}
        what="這一題"
        fallback="這一題沒有建立，匯入停在這裡；還沒建立的題目留在上面，可以再按一次匯入。"
        unknown="尚未確認這一題是否已建立；清單已重新讀取，請先確認，再決定要不要重送，以免重複。"
      />
    </section>
  );
}

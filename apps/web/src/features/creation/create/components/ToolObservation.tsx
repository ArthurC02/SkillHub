import { Reveal } from "../../../../shared/ui/Reveal";
import { exactBytes } from "../../../../shared/format";
import { runStatusLabel } from "../../../runs";
import { FETCH_STATUS_LABEL } from "../create.model";
import {
  CRITERION_LABEL,
  criterionTally,
  verdictWord,
  OVERALL_LABEL,
} from "../../../../shared/verdict";
import { runeCount } from "../../../../shared/textLength.model";

type FetchObservation = {
  fetch: { url: string; status: string; bytes?: number; text?: string };
};

type RunToolObservation = {
  run_id: string;
  execution_status: string;
  evaluation?: {
    overall?: string;
    summary?: string;
    criterion_results?: { text?: string; result?: string; reason?: string }[];
  };
};

function parseObservation(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
}

export function ToolObservation({ raw }: { raw: string }) {
  const parsed = parseObservation(raw);
  if (parsed && typeof parsed === "object") {
    const asFetch = parsed as Partial<FetchObservation>;
    if (asFetch.fetch && typeof asFetch.fetch.url === "string") {
      const f = asFetch.fetch;
      const text = typeof f.text === "string" ? f.text : undefined;
      return (
        <>
          <span className="creation-text">
            讀取網頁 {f.url}：{FETCH_STATUS_LABEL[f.status] ?? f.status}
            {f.bytes !== undefined && `（${exactBytes(f.bytes)}）`}
          </span>
          {!!text && (
            <details>
              <summary>讀到的網頁內容（{runeCount(text)} 字）</summary>
              <pre className="skill-md">
                <Reveal text={text} />
              </pre>
            </details>
          )}
        </>
      );
    }
    const asRun = parsed as Partial<RunToolObservation>;
    if (typeof asRun.run_id === "string" && typeof asRun.execution_status === "string") {
      const results = asRun.evaluation?.criterion_results ?? [];
      const overall = asRun.evaluation?.overall ?? "";
      return (
        <>
          <span className="creation-text">
            試跑結果：{runStatusLabel(asRun.execution_status)}；評估：
            {verdictWord(OVERALL_LABEL, overall)}
            {results.length > 0 && `（${criterionTally(results)}）`}
          </span>
          {!!asRun.evaluation?.summary && (
            <span className="creation-text">{asRun.evaluation.summary}</span>
          )}
          {results.length > 0 && (
            <ul>
              {results.map((c, i) => (
                <li key={i}>
                  {verdictWord(CRITERION_LABEL, c.result)}：{c.text}
                  {!!c.reason && `——${c.reason}`}
                </li>
              ))}
            </ul>
          )}
        </>
      );
    }
  }
  return <span className="creation-text">{raw}</span>;
}

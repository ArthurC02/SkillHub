import { Reveal } from "../../../../shared/ui/Reveal";
import { runStatusLabel } from "../../../runs";
import { FETCH_STATUS_LABEL, ROUND_OVERALL_LABEL } from "../create.model";

const CRITERION_RESULT_LABEL: Record<string, string> = {
  passed: "通過",
  failed: "不通過",
  undetermined: "無法判定",
};

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
            {f.bytes !== undefined && `（${f.bytes} 位元組）`}
          </span>
          {!!text && (
            <details>
              <summary>讀到的網頁內容（{[...text].length} 字）</summary>
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
      const count = (r: string) => results.filter((x) => x.result === r).length;
      const overall = asRun.evaluation?.overall ?? "";
      return (
        <>
          <span className="creation-text">
            試跑結果：{runStatusLabel(asRun.execution_status)}；評估：
            {ROUND_OVERALL_LABEL[overall] ?? "無評估"}
            {results.length > 0 &&
              `（通過 ${count("passed")}／不通過 ${count("failed")}／無法判定 ${count("undetermined")}）`}
          </span>
          {!!asRun.evaluation?.summary && (
            <span className="creation-text">{asRun.evaluation.summary}</span>
          )}
          {results.length > 0 && (
            <ul>
              {results.map((c, i) => (
                <li key={i}>
                  {CRITERION_RESULT_LABEL[c.result ?? ""] ?? c.result ?? "無結果"}：{c.text}
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

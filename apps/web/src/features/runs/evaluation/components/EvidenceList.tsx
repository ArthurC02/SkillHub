import { StateIcon } from "../../../../shared/ui/StateIcon";
import type { EvidenceRef } from "../../evaluation.service";
import { MATCH_WORD, MATCH_BADGE, MATCH_ICON, matchKey, KIND_WORD } from "../evaluation.model";

export function EvidenceList({ evidence }: { evidence: EvidenceRef[] }) {
  if (evidence.length === 0) return <p className="note">沒有附上證據引用。</p>;
  return (
    <ul className="evidence-list">
      {evidence.map((e, i) => (
        <li key={`${e.kind}-${i}`}>
          <p className="note">
            <span className={MATCH_BADGE[matchKey(e)]}>
              <StateIcon state={MATCH_ICON[matchKey(e)]} />
              {MATCH_WORD[matchKey(e)]}
            </span>{" "}
            {e.kind === "trace_event" && "Trace 事件"}
            {e.kind === "artifact" &&
              `Artifact ${e.artifact_path ?? ""}${
                e.byte_range ? `（位元組 ${e.byte_range.start}–${e.byte_range.end}）` : ""
              }`}
            {e.kind === "agent_output" &&
              `Agent 輸出${
                e.char_range ? `（字元 ${e.char_range.start}–${e.char_range.end}）` : ""
              }`}
          </p>
          {e.reattributed_from && (
            <p className="note">
              Judge 原本標為「{KIND_WORD[e.reattributed_from]}」，實際出處是「{KIND_WORD[e.kind]}
              」， 已更正。標錯來源與捏造引文是兩件不同的事，這一筆是前者。
            </p>
          )}
          {e.kind === "trace_event" && (
            <details>
              <summary>事件 ID</summary>
              {e.trace_event_id ? (
                <code>{e.trace_event_id}</code>
              ) : (
                <p className="note">這筆引用沒有附上事件 ID。</p>
              )}
            </details>
          )}
          {!e.available && (
            <p className="note">原始資料已過期或已刪除，以下是評估當時保存的摘要。</p>
          )}
          <pre>{e.excerpt}</pre>
          {e.excerpt_truncated && <p className="note">（摘要已截斷，不是全文）</p>}
        </li>
      ))}
    </ul>
  );
}

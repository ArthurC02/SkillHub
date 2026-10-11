import { useState } from "react";
import type { CheckKind } from "../check.model";

export function KindPicker({
  id,
  label,
  kinds,
  action,
  onPick,
}: {
  id: string;
  label: string;
  kinds: CheckKind[];
  action: string;
  onPick: (kind: CheckKind) => void;
}) {
  const [choice, setChoice] = useState(kinds[0]?.kind ?? "");
  const kind = kinds.find((k) => k.kind === choice);
  return (
    <p className="check-editor-picker">
      <label htmlFor={id}>{label}</label>{" "}
      <select id={id} value={choice} onChange={(e) => setChoice(e.target.value)}>
        {kinds.map((k) => (
          <option key={k.kind} value={k.kind}>
            {k.category === "composite" ? `組合：${k.label}` : k.label}
          </option>
        ))}
      </select>{" "}
      <button type="button" disabled={!kind} onClick={() => kind && onPick(kind)}>
        {action}
      </button>
      {kind && <span className="note"> {kind.description}</span>}
    </p>
  );
}

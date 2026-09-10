import type { Labelled } from "../api/types";
import { StateIcon } from "./StateIcon";

export function LabelledBadge({
  kind,
  value,
  noteInRow = true,
}: {
  kind: string;
  value: Labelled;
  noteInRow?: boolean;
}) {
  const stateClass = `badge-${kind}-${value.value}`;
  const showUnknownIcon = stateClass.endsWith("-unknown") || stateClass.endsWith("-unverified");
  return (
    <>
      <span className={`badge ${stateClass}`}>
        {showUnknownIcon && <StateIcon state="unknown" />}
        {value.label}
      </span>
      {noteInRow && value.note && <span className="note">{value.note}</span>}
    </>
  );
}

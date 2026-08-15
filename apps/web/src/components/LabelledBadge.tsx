import type { Labelled } from "../api/types";

/**
 * Renders a server-owned `Labelled` (trust, license status, tier). The copy
 * comes from the API on purpose: NFR-001 requires every surface to explain a
 * trust state with the same factual wording, so the client must not keep its
 * own enum-to-Chinese map that can drift from what was actually checked.
 */
export function LabelledBadge({ kind, value }: { kind: string; value: Labelled }) {
  return (
    <span className={`badge badge-${kind}-${value.value}`} title={value.note}>
      {value.label}
    </span>
  );
}

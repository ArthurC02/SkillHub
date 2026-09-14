import type { Labelled } from "../../../core/api/types";
import { StateIcon } from "../../../shared/ui/StateIcon";

export const VERDICT_BADGE: Record<string, string> = {
  met: "badge",
  not_met: "badge badge-danger",
};

const NOTE_SHOWN = new Set(["not_evaluated", "evaluating", "evaluation_failed", "undetermined"]);

export function RunVerdict({ verdict }: { verdict: Labelled }) {
  return (
    <>
      <span className={VERDICT_BADGE[verdict.value] ?? "badge badge-unverified"}>
        {verdict.value === "met" ? (
          <StateIcon state="pass" />
        ) : verdict.value === "not_met" ? (
          <StateIcon state="fail" />
        ) : (
          <StateIcon state="unknown" />
        )}
        任務判定：{verdict.label}
      </span>
      {NOTE_SHOWN.has(verdict.value) && <span className="note">{verdict.note}</span>}
    </>
  );
}

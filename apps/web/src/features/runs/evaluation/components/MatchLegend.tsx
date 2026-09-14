import { StateIcon } from "../../../../shared/ui/StateIcon";
import type { EvidenceRef } from "../../evaluation.service";
import {
  MATCH_NOTE,
  MATCH_WORD,
  MATCH_BADGE,
  MATCH_ICON,
  MATCH_ORDER,
  matchKey,
} from "../evaluation.model";

export function MatchLegend({ evidence }: { evidence: EvidenceRef[] }) {
  const kinds = MATCH_ORDER.filter((k) => evidence.some((e) => matchKey(e) === k));
  if (kinds.length === 0) return null;
  return (
    <ul className="note">
      {kinds.map((k) => (
        <li key={k}>
          <span className={MATCH_BADGE[k]}>
            <StateIcon state={MATCH_ICON[k]} />
            {MATCH_WORD[k]}
          </span>{" "}
          {MATCH_NOTE[k]}
        </li>
      ))}
    </ul>
  );
}

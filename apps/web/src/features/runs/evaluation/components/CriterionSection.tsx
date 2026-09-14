import type { CriterionResult } from "../../evaluation.service";
import { SOURCE_LABEL, listSource } from "../evaluation.model";
import { CriterionItem } from "./CriterionItem";
import { MatchLegend } from "./MatchLegend";

export function CriterionSection({ results }: { results: CriterionResult[] }) {
  const shared = listSource(results);
  return (
    <>
      {shared && <p className="note">判定來源：{SOURCE_LABEL[shared]}；不同的會在該條標出。</p>}
      <MatchLegend evidence={results.flatMap((c) => c.evidence)} />
      <ul className="criterion-list">
        {results.map((c) => (
          <CriterionItem key={c.criterion_id} criterion={c} listSource={shared} />
        ))}
      </ul>
    </>
  );
}

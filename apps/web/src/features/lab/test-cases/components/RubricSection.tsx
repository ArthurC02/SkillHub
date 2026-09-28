import { useState } from "react";
import { useUpdateTestCase } from "../../testcases.service";
import type { RubricItem, TestCase } from "../../testcases.service";
import { MutationError } from "./MutationError";
import { RubricCriterionItem } from "./RubricCriterionItem";

export function RubricSection({ testCase }: { testCase: TestCase }) {
  const stored = testCase.rubric;
  const [version, setVersion] = useState(stored?.version ?? "");
  const [items, setItems] = useState<Record<string, RubricItem>>(
    Object.fromEntries((stored?.items ?? []).map((i) => [i.id, i])),
  );
  const save = useUpdateTestCase(testCase.test_case_id);
  const saveRubric = () => {
    const list = testCase.acceptance_criteria
      .map((c) => items[c.id])
      .filter((i): i is RubricItem => i !== undefined && i.text.trim() !== "");
    // null means no rubric; {items: []} would mean a rubric with nothing in it.
    save.mutate({ rubric: list.length === 0 ? null : { version: version.trim(), items: list } });
  };

  const update = (id: string, patch: Partial<RubricItem>) =>
    setItems((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? { id, text: "", evidence_required: false }), ...patch },
    }));

  const used = testCase.acceptance_criteria.filter((c) => items[c.id]?.text.trim()).length;

  return (
    <>
      <h3>Rubric（選用）</h3>
      <p className="note" data-role="evidence">
        Rubric 補充每條驗收條件的「怎樣算過」與引文要求；平台不拿權重計分。
      </p>
      {testCase.acceptance_criteria.length === 0 ? (
        <p>要先有驗收條件才能寫 rubric——rubric 的每一條都是掛在某一條驗收條件上的。</p>
      ) : (
        <>
          <p>
            <label htmlFor="rubric-version">Rubric 版本</label>{" "}
            <input
              id="rubric-version"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              size={40}
              maxLength={200}
              placeholder="例如 content-007/writing/v1"
            />
            <br />
            <span className="note" data-role="evidence">
              改動文字、權重或引文要求會建立新版本；評估報告會記下版本。
            </span>
          </p>
          <ul className="criterion-list" data-role="evidence">
            {testCase.acceptance_criteria.map((c) => (
              <RubricCriterionItem
                key={c.id}
                criterionId={c.id}
                criterionText={c.text}
                item={items[c.id]}
                onUpdate={(patch) => update(c.id, patch)}
              />
            ))}
          </ul>
          <button
            type="button"
            disabled={save.isPending || (used > 0 && version.trim() === "")}
            aria-describedby={
              used > 0 && version.trim() === "" ? "rubric-version-reason" : undefined
            }
            onClick={saveRubric}
          >
            {save.isPending ? "儲存中…" : "儲存 Rubric"}
          </button>{" "}
          <span className="note">
            {used === 0
              ? "目前沒有任何一條有內容，儲存等於移除這個 Test Case 的 rubric。"
              : `目前 ${used} 條有內容。`}
          </span>
          {used > 0 && version.trim() === "" && (
            <span id="rubric-version-reason" className="note" role="status">
              還不能儲存，因為 Rubric 版本是空的。有內容的 rubric
              一定要有版本——評估報告要記下這次判定是在哪個版本下做的。
            </span>
          )}
          {save.isSuccess && <p role="status">已儲存。</p>}
          <MutationError
            error={save.error}
            what="Rubric"
            fallback="儲存沒有成功，可以再試一次。"
            serverSaysStatuses={[413]}
          />
        </>
      )}
    </>
  );
}

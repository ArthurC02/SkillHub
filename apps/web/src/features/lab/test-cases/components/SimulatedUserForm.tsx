import { SimulatedUserFactsMaxItems } from "@skillhub/api-client-ts/limits";
import { useState } from "react";
import { useUpdateTestCase } from "../../testcases.service";
import type { TestCase } from "../../testcases.service";
import { MutationError } from "./MutationError";

const MAX_FACT_CHARS = 1500;

function factsOf(text: string) {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");
}

export function SimulatedUserForm({ testCase }: { testCase: TestCase }) {
  const [draft, setDraft] = useState<string | null>(null);
  const text = draft ?? testCase.simulated_user_facts.join("\n");
  const save = useUpdateTestCase(testCase.test_case_id);
  const facts = factsOf(text);
  const tooMany = facts.length > SimulatedUserFactsMaxItems;
  const totalCharacters = facts.reduce((total, fact) => total + Array.from(fact).length, 0);
  const tooLong = totalCharacters > MAX_FACT_CHARS;
  const invalid = tooMany || tooLong;
  const explanation = [
    tooMany ? `目前 ${facts.length} 行，最多 ${SimulatedUserFactsMaxItems} 行。` : null,
    tooLong ? `目前合計 ${totalCharacters} 個字，最多 ${MAX_FACT_CHARS} 個字。` : null,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <>
      <h2>模擬使用者</h2>
      <p className="note">
        寫下這位使用者知道、但在 User Prompt 裡沒說的事，一行一件。批次評估時，試跑若停下來問問題，
        平台會以這位使用者的身分把這些事告訴它，讓對話繼續；留白則由真人回答。
      </p>
      <p className="field">
        <label htmlFor="simulated-user-facts">使用者知道的事</label>
        <textarea
          id="simulated-user-facts"
          rows={4}
          cols={60}
          value={text}
          onChange={(e) => {
            save.reset();
            setDraft(e.target.value);
          }}
          readOnly={save.isPending}
          aria-invalid={invalid}
          aria-describedby="simulated-user-limit"
        />
        <br />
        <span id="simulated-user-limit" className="note" role={invalid ? "status" : undefined}>
          {invalid
            ? explanation
            : `最多 ${SimulatedUserFactsMaxItems} 行，合計最多 ${MAX_FACT_CHARS} 個字。`}
        </span>
      </p>
      <button
        type="button"
        disabled={save.isPending || invalid}
        onClick={() => {
          const submitted = draft;
          save.mutate(
            { simulated_user_facts: facts },
            { onSuccess: () => setDraft((current) => (current === submitted ? null : current)) },
          );
        }}
      >
        {save.isPending ? "儲存中…" : "儲存模擬使用者"}
      </button>
      {save.isSuccess && <p role="status">已儲存。</p>}
      <MutationError error={save.error} what="模擬使用者" fallback="儲存沒有成功，可以再試一次。" />
    </>
  );
}

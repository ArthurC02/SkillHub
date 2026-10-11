import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { CheckEditor, forRequest, useCheckReady, type CheckNode } from "../../../evaluation";
import { ApiError } from "../../../../core/api/client";
import { useAddCriterion, useSuggestCriteria } from "../../testcases.service";
import type { TestCase } from "../../testcases.service";
import { MutationError } from "./MutationError";
import { CriterionRow } from "./CriterionRow";

const MAX_CRITERIA = 50;

function Suggestions({
  testCaseId,
  suggestions,
  onSettled,
}: {
  testCaseId: string;
  suggestions: string[];
  onSettled: (proposal: string) => void;
}) {
  const adopt = useAddCriterion(testCaseId);
  if (suggestions.length === 0) return null;
  return (
    <>
      <h3>系統的建議（尚未加入）</h3>
      <p className="note">
        以下只是建議，還沒有寫進這個測試題。按「採納」才會加成一條驗收條件，並且會標成
        系統建議、維持未確認——要不要算數還是你決定。不想要就按「忽略」，什麼都不會發生。
      </p>
      <ul className="criterion-list">
        {suggestions.map((s) => (
          <li key={s} className="criterion">
            <p>{s}</p>
            <p>
              <button
                type="button"
                disabled={adopt.isPending}
                onClick={() =>
                  adopt.mutate({ text: s, source: "suggested" }, { onSuccess: () => onSettled(s) })
                }
              >
                {adopt.isPending
                  ? adopt.variables?.text === s
                    ? "採納中…"
                    : "採納（另一項處理中…）"
                  : "採納"}
              </button>{" "}
              <button type="button" onClick={() => onSettled(s)}>
                忽略
              </button>
            </p>
          </li>
        ))}
      </ul>
      <MutationError error={adopt.error} what="這條建議" fallback="無法採納，可以再試一次。" />
    </>
  );
}

function addBlocker(text: string, checkReady: boolean) {
  if (text.trim() === "") return { id: "criterion-add-why", say: "欄位是空的" };
  if (!checkReady) return { id: "criterion-check-why", say: "檢查還有節點要修正" };
  return null;
}

export function CriteriaSection({ testCase }: { testCase: TestCase }) {
  const [text, setText] = useState("");
  const [check, setCheck] = useState<CheckNode | null>(null);
  const checkReady = useCheckReady(check);
  const [message, setMessage] = useState("");
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const add = useAddCriterion(testCase.test_case_id);
  const suggest = useSuggestCriteria(testCase.test_case_id);
  const suggestError = suggest.error;
  const blocker = addBlocker(text, checkReady);

  const requestSuggestions = () =>
    suggest.mutate(undefined, {
      onSuccess: (res) => {
        setSuggestions(res.suggestions.map((s) => s.text));
        setMessage(res.suggestions.length === 0 ? "這次沒有可用的建議，請自己手動輸入。" : "");
      },
    });

  return (
    <>
      <h2>驗收條件</h2>
      <p className="note" data-role="evidence">
        每條驗收條件各自判定。開始試跑時會凍結成快照；修改只影響<strong>下一次</strong>試跑。
      </p>

      {testCase.acceptance_criteria.length === 0 ? (
        <p>還沒有驗收條件。沒有驗收條件的試跑沒有可逐條判定的依據。</p>
      ) : (
        <ul className="criterion-list" data-role="evidence">
          {testCase.acceptance_criteria.map((c) => (
            <CriterionRow key={c.id} testCaseId={testCase.test_case_id} criterion={c} />
          ))}
        </ul>
      )}

      <p className="note">一個測試題最多 {MAX_CRITERIA} 條驗收條件。</p>
      <p>
        <label htmlFor="new-criterion">新增驗收條件</label>{" "}
        <input
          id="new-criterion"
          value={text}
          onChange={(e) => setText(e.target.value)}
          size={50}
          maxLength={2000}
        />{" "}
        <button
          type="button"
          disabled={add.isPending || blocker !== null}
          aria-describedby={blocker?.id}
          onClick={() =>
            add.mutate(
              { text, check: check ? forRequest(check) : undefined },
              {
                onSuccess: () => {
                  setText("");
                  setCheck(null);
                },
              },
            )
          }
        >
          {add.isPending ? "新增中…" : "新增"}
        </button>{" "}
        <button type="button" disabled={suggest.isPending} onClick={requestSuggestions}>
          {suggest.isPending ? "建議中…" : "請系統建議（選用）"}
        </button>{" "}
        {blocker && (
          <span className="note" role="status" id={blocker.id}>
            {blocker.say}
          </span>
        )}
      </p>
      <h3>這一條怎麼判定（選用）</h3>
      <p className="note">
        不設計就交給評審模型依文字判定。設計時可以把規則、評審判定與評分器用串聯、並聯、條件與
        Fan-Out 組合起來；規則能決定的不會再問評審模型。<Link to="/lab/graders">管理評分器</Link>
      </p>
      <CheckEditor value={check} onChange={setCheck} />
      <MutationError error={add.error} what="這一條驗收條件" fallback="無法新增，可以再試一次。" />
      <ReadFailure error={suggestError} what="建議">
        <p role="alert">
          {suggestError instanceof ApiError && suggestError.status === 503
            ? "目前無法自動建議，驗收條件可以自己手動輸入。"
            : "無法取得建議，可以再試一次。"}
        </p>
      </ReadFailure>
      {message && <p role="status">{message}</p>}

      <Suggestions
        testCaseId={testCase.test_case_id}
        suggestions={suggestions}
        onSettled={(proposal) => setSuggestions((prev) => prev.filter((s) => s !== proposal))}
      />
    </>
  );
}

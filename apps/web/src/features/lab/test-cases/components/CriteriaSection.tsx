import { AcceptanceCriterionTextMaxLength } from "@skillhub/api-client-ts/limits";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { CheckEditor, forRequest, useCheckReady, type CheckNode } from "../../../evaluation";
import { ApiError } from "../../../../core/api/client";
import { useAddCriterion, useSuggestCriteria } from "../../testcases.service";
import type { TestCase } from "../../testcases.service";
import { MutationError } from "./MutationError";
import { CriterionRow } from "./CriterionRow";
import { textLengthReason } from "../test-cases.model";

const MAX_CRITERIA = 20; // one-number: maxCriteria

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

const FULL_SENTENCE = `已經有 ${MAX_CRITERIA} 條驗收條件，刪掉幾條才能再新增或請系統建議。`;

function addBlocker(text: string, checkReady: boolean, sizeProblem: string | null, full: boolean) {
  if (full) return { id: "criterion-add-why", say: FULL_SENTENCE };
  if (text.trim() === "") return { id: "criterion-add-why", say: "欄位是空的" };
  if (sizeProblem) return { id: "criterion-add-why", say: sizeProblem };
  if (!checkReady)
    return {
      id: "criterion-check-why",
      say: "檢查設計尚未確認可用，請查看下方的讀取狀態與節點提示。",
    };
  return null;
}

function suggestFailureSentence(error: unknown) {
  const status = error instanceof ApiError ? error.status : 0;
  if (status === 503) return "目前無法自動建議，驗收條件可以自己手動輸入。";
  if (status === 403) return "自動建議只開放給封測受邀帳號，驗收條件可以自己手動輸入。";
  if (status === 413) return `${FULL_SENTENCE}這次沒有呼叫模型、也沒有扣點。`;
  if (status === 422)
    return "點數不足，這次沒有呼叫模型、也沒有扣點。驗收條件可以自己手動輸入，或請管理者加點後再試。";
  return "無法取得建議，可以再試一次。";
}

export function CriteriaSection({ testCase }: { testCase: TestCase }) {
  const [draft, setDraft] = useState<{ text: string; check: CheckNode | null }>({
    text: "",
    check: null,
  });
  const { text, check } = draft;
  const checkReady = useCheckReady(check);
  const [message, setMessage] = useState("");
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const add = useAddCriterion(testCase.test_case_id);
  const suggest = useSuggestCriteria(testCase.test_case_id);
  const suggestError = suggest.error;
  const sizeProblem = textLengthReason(text, "驗收條件", AcceptanceCriterionTextMaxLength);
  const full = testCase.acceptance_criteria.length >= MAX_CRITERIA;
  const blocker = addBlocker(text, checkReady, sizeProblem, full);

  const addCriterion = () => {
    const submitted = draft;
    add.mutate(
      { text: submitted.text, check: submitted.check ? forRequest(submitted.check) : undefined },
      {
        onSuccess: () =>
          setDraft((current) => (current === submitted ? { text: "", check: null } : current)),
      },
    );
  };

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
      {testCase.acceptance_criteria.length > MAX_CRITERIA && (
        <p role="status">
          這個測試題有 {testCase.acceptance_criteria.length} 條驗收條件，評估只判定前 {MAX_CRITERIA}{" "}
          條；超過的部分會讓整次判定標為無法判定。刪到 {MAX_CRITERIA} 條以內才可能通過。
        </p>
      )}
      <p>
        <label htmlFor="new-criterion">新增驗收條件</label>{" "}
        <input
          id="new-criterion"
          value={text}
          onChange={(e) => {
            const text = e.target.value;
            setDraft((current) => ({ ...current, text }));
          }}
          size={50}
          aria-invalid={sizeProblem !== null}
          aria-describedby={blocker?.id}
        />{" "}
        <button
          type="button"
          disabled={add.isPending || blocker !== null}
          aria-describedby={blocker?.id}
          onClick={addCriterion}
        >
          {add.isPending ? "新增中…" : "新增"}
        </button>{" "}
        <button
          type="button"
          disabled={suggest.isPending || full}
          aria-describedby={full ? blocker?.id : undefined}
          onClick={requestSuggestions}
        >
          {suggest.isPending ? "建議中…" : "請系統建議（選用）"}
        </button>
      </p>
      {blocker && (
        <p className="note" role="status" id={blocker.id}>
          {blocker.say}
        </p>
      )}
      {add.isSuccess && (
        <p role="status">
          已新增驗收條件。
          {(text !== "" || check !== null) && "目前欄位裡的草稿尚未新增。"}
        </p>
      )}
      <MutationError error={add.error} what="這一條驗收條件" fallback="無法新增，可以再試一次。" />
      <h3>這一條怎麼判定（選用）</h3>
      <p className="note">
        不設計就交給評審模型依文字判定。設計時可以把規則、評審判定與評分器用串聯、並聯、條件與
        Fan-Out 組合起來；規則能決定的不會再問評審模型。<Link to="/lab/graders">管理評分器</Link>
      </p>
      <CheckEditor
        value={check}
        onChange={(check) => setDraft((current) => ({ ...current, check }))}
      />
      <ReadFailure error={suggestError} what="建議">
        <p role="alert">{suggestFailureSentence(suggestError)}</p>
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

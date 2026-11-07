import { SuggestTestCasesBodyFocusMaxLength } from "@skillhub/api-client-ts/limits";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { ApiError, userFacingReason } from "../../../../core/api/client";
import {
  useAdoptTestCaseSuggestion,
  useSuggestTestCases,
  type AdoptedTestCase,
  type TestCaseSuggestion,
} from "../../testcases.service";
import { MutationError } from "./MutationError";

const CREDIT_REFUSED = "點數不足，這次沒有呼叫模型、也沒有扣點。請管理者加點後再試。";

function suggestFailureSentence(error: unknown) {
  const status = error instanceof ApiError ? error.status : 0;
  if (status === 503) return userFacingReason(error) ?? "目前無法自動建議測試題，請自己手動寫。";
  if (status === 403) return "自動建議只開放給封測受邀帳號，測試題可以自己手動寫。";
  if (status === 422) return `${userFacingReason(error) ?? CREDIT_REFUSED}測試題可以自己手動寫。`;
  return userFacingReason(error) ?? "無法取得建議，可以再試一次。";
}

function AdoptedNote({ adopted, version }: { adopted: AdoptedTestCase; version?: string }) {
  return (
    <p role="status">
      {adopted.criteriaMissing === 0
        ? "已建立測試題，驗收條件標成系統建議、尚未確認。"
        : `已建立測試題，但有 ${adopted.criteriaMissing} 條驗收條件沒有加進去，請到測試題裡補上。`}{" "}
      <Link
        to="/lab/test-cases/$testCaseId"
        params={{ testCaseId: adopted.testCaseId }}
        search={{ version }}
      >
        打開這個測試題
      </Link>
    </p>
  );
}

function Proposal({
  suggestion,
  busy,
  onAdopt,
  onIgnore,
}: {
  suggestion: TestCaseSuggestion;
  busy: boolean;
  onAdopt: () => void;
  onIgnore: () => void;
}) {
  return (
    <li className="criterion">
      <h4>{suggestion.name}</h4>
      <p>{suggestion.user_prompt}</p>
      {suggestion.criteria.length > 0 && (
        <ul>
          {suggestion.criteria.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
      )}
      <p>
        <button type="button" disabled={busy} onClick={onAdopt}>
          採納
        </button>{" "}
        <button type="button" onClick={onIgnore}>
          忽略
        </button>
      </p>
    </li>
  );
}

export function SuggestTestCases({ skillId, version }: { skillId: string; version?: string }) {
  const [focus, setFocus] = useState("");
  const [proposals, setProposals] = useState<TestCaseSuggestion[] | null>(null);
  const [adopted, setAdopted] = useState<AdoptedTestCase | null>(null);
  const suggest = useSuggestTestCases(skillId);
  const adopt = useAdoptTestCaseSuggestion(skillId);
  const tooLong = [...focus].length > SuggestTestCasesBodyFocusMaxLength;
  const drop = (s: TestCaseSuggestion) =>
    setProposals((current) => current?.filter((p) => p !== s) ?? null);

  return (
    <section aria-labelledby="suggest-test-cases-heading">
      <h3 id="suggest-test-cases-heading">請系統建議測試題（選用）</h3>
      <p>
        <label htmlFor="suggest-test-cases-focus">想測的方向</label>{" "}
        <input
          id="suggest-test-cases-focus"
          value={focus}
          onChange={(e) => setFocus(e.target.value)}
          size={40}
          aria-invalid={tooLong}
          aria-describedby={tooLong ? "suggest-test-cases-why" : undefined}
        />{" "}
        <button
          type="button"
          disabled={suggest.isPending || tooLong}
          aria-describedby={tooLong ? "suggest-test-cases-why" : undefined}
          onClick={() =>
            suggest.mutate(focus, {
              onSuccess: (list) => {
                setProposals(list);
                setAdopted(null);
              },
            })
          }
        >
          {suggest.isPending ? "建議中…" : "請系統建議"}
        </button>
      </p>
      {tooLong && (
        <p className="note" role="status" id="suggest-test-cases-why">
          想測的方向最多 {SuggestTestCasesBodyFocusMaxLength} 個字。
        </p>
      )}
      {suggest.error && <p role="alert">{suggestFailureSentence(suggest.error)}</p>}
      {proposals?.length === 0 && <p role="status">這次沒有可用的建議，請自己手動寫。</p>}
      {adopted && <AdoptedNote adopted={adopted} version={version} />}
      {proposals && proposals.length > 0 && (
        <>
          <p className="note">
            以下只是建議，還沒有建立。按「採納」才會建立成測試題；不想要就按「忽略」。
          </p>
          <ul className="criterion-list">
            {proposals.map((s) => (
              <Proposal
                key={s.name}
                suggestion={s}
                busy={adopt.isPending}
                onAdopt={() =>
                  adopt.mutate(s, {
                    onSuccess: (result) => {
                      setAdopted(result);
                      drop(s);
                    },
                  })
                }
                onIgnore={() => drop(s)}
              />
            ))}
          </ul>
        </>
      )}
      <MutationError
        error={adopt.error}
        what="這個建議"
        fallback="無法建立，可以再試一次。"
        unknown="尚未確認這個測試題是否已建立；清單已重新讀取，請先確認，再決定要不要重送，以免重複。"
      />
    </section>
  );
}

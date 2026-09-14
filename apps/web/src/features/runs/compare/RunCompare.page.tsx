import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { LoginRequired, ReadFailure, unauthenticated } from "../../../shared/ui/LoginRequired";
import { useMe } from "../../../core/session/me.service";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useRunComparison } from "../evaluation.service";
import { useRun, useRuns } from "../runs.service";
import { RunVerdict } from "../components/RunVerdict";
import { runStatusLabel } from "../runs.model";
import { ComparisonLead } from "./components/ComparisonLead";
import { ComparisonTables } from "./components/ComparisonTables";

export function RunCompare() {
  const { runId } = useParams({ from: "/runs/$runId/compare" });
  const { against = "" } = useSearch({ strict: false }) as { against?: string };
  const [draft, setDraft] = useState(against);
  useEffect(() => setDraft(against), [against]);
  const navigate = useNavigate();
  const comparison = useRunComparison(runId, against);
  const me = useMe();
  const loggedOut = unauthenticated(me.error);

  const self = useRun(runId);
  const testCaseId = self.data?.test_case_id;
  const siblings = useRuns(testCaseId, Boolean(testCaseId));
  const candidates = testCaseId
    ? (siblings.data?.pages.flatMap((p) => p.runs) ?? []).filter((r) => r.run_id !== runId)
    : [];

  const pick = (id: string) =>
    void navigate({ to: "/runs/$runId/compare", params: { runId }, search: { against: id } });

  const pickForm = (
    <>
      {self.isPending ? (
        <Loading what="目前這次 Run" />
      ) : self.error ? (
        <ReadFailure error={self.error} what="目前這次 Run" />
      ) : !testCaseId ? (
        <p>這次 Run 的 Test Case 已無法解析，因此無法列出同一個 Test Case 的其他 Run。</p>
      ) : siblings.isPending ? (
        <Loading what="可比較的 Run" />
      ) : siblings.error ? (
        <ReadFailure error={siblings.error} what="可比較的 Run" />
      ) : candidates.length > 0 ? (
        <ul className="download-list">
          {candidates.map((r) => (
            <li key={r.run_id} className="download-item">
              <p className="badge-row">
                <RunVerdict verdict={r.evaluation} />
              </p>
              <p className="badge-row">
                <span className="badge">執行狀態：{runStatusLabel(r.status)}</span>
              </p>
              {r.status_reason && <p className="note">{r.status_reason}</p>}
              <p>
                <button type="button" onClick={() => pick(r.run_id)}>
                  與這一次比較（建立於 <Timestamp at={r.created_at} />）
                </button>
              </p>
            </li>
          ))}
        </ul>
      ) : (
        <p>這個 Test Case 目前只有這一次 Run，沒有同一個 Test Case 的其他 Run 可選。</p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          pick(draft);
        }}
      >
        <label htmlFor="against">要比較的另一個 Run ID</label>{" "}
        <input
          id="against"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          size={40}
          placeholder="另一個 Run 的平台 run_id"
        />{" "}
        <button type="submit">比較</button>
        <p className="note">
          {(candidates.length > 0 ? "從上面選一個同一個 Test Case 的 Run，或" : "") +
            "輸入另一個 Run 的 ID 後開始比較。別的 Test Case 或別的 Skill 的 Run 也可以。"}
        </p>
      </form>
    </>
  );

  return (
    <section>
      <h1>Run 比較</h1>

      {comparison.data && <ComparisonLead data={comparison.data} />}

      <details>
        <summary>進階資訊（Run 識別碼）</summary>
        <ul>
          <li>
            這一邊：<code>{runId}</code>
          </li>
          <li>另一邊：{against === "" ? "尚未選擇" : <code>{against}</code>}</li>
        </ul>
      </details>

      {loggedOut ? (
        <LoginRequired what="Run 比較" />
      ) : comparison.data ? (
        <details>
          <summary>換一個要比較的 Run</summary>
          {pickForm}
        </details>
      ) : (
        pickForm
      )}

      {comparison.isPending && against !== "" && <Loading what="比較" />}
      <ReadFailure error={comparison.error} what="比較結果">
        <p role="alert">無法比較：{comparison.error?.message}</p>
      </ReadFailure>
      {comparison.data && <ComparisonTables data={comparison.data} />}

      <p className="note">
        <Link to="/runs/$runId" params={{ runId }}>
          回到這個 Run 的詳情
        </Link>
      </p>
    </section>
  );
}

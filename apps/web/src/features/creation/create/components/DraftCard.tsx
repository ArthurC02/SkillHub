import { Link } from "@tanstack/react-router";
import type { CreationSnapshot, CreationState } from "../../creation.service";
import { runStatusLabel, type RunListItem } from "../../../runs";
import { Reveal } from "../../../../shared/ui/Reveal";
import { findRunObservation } from "../create.model";
import type { Perform } from "../create.commands";
import { DraftFindings } from "./DraftFindings";

export function DraftCard({
  p,
  draft,
  state,
  terminal,
  latest,
  locked,
  perform,
}: {
  p: CreationSnapshot;
  draft: NonNullable<CreationSnapshot["draft"]>;
  state: CreationState;
  terminal: boolean;
  latest: RunListItem | null | undefined;
  locked: boolean;
  perform: Perform;
}) {
  const run = p.candidate?.run_id ? findRunObservation(p.messages, p.candidate.run_id) : undefined;
  const runNotPassing =
    !!run && (run.execution_status !== "succeeded" || run.evaluation?.overall !== "met");
  return (
    <section id="creation-draft-decision" tabIndex={-1}>
      <header className="card-header">
        <h4>小工具草稿：{draft.skill.name}</h4>
        <span className="card-tag" data-tone={draft.blocked ? "danger" : "done"}>
          {draft.blocked ? "靜態檢查阻擋保存" : "已完成靜態檢查"}
        </span>
      </header>
      <p>{draft.skill.description}</p>
      <p>
        允許工具：{draft.skill.allowed_tools || "未宣告"}。相容條件：
        {draft.skill.compatibility || "未宣告"}。
      </p>
      {p.previous_draft && (
        <details>
          <summary>比較上一份草稿（revision {p.previous_draft.revision}）</summary>
          <pre className="skill-md">
            <Reveal text={p.previous_draft.skill.body} />
          </pre>
          {p.previous_draft.skill.files?.map((f) => (
            <pre key={f.path}>{f.path + "\n" + f.content}</pre>
          ))}
        </details>
      )}
      <pre className="skill-md">
        <Reveal text={draft.skill.body} />
      </pre>
      {draft.skill.files?.map((f) => (
        <details key={f.path}>
          <summary>{f.path}</summary>
          <pre className="skill-md">
            <Reveal text={f.content} />
          </pre>
        </details>
      ))}
      <DraftFindings raw={draft.validation} />
      <p className="note">
        {draft.blocked ? "請補充需求後修訂。" : "靜態檢查通過不代表試跑成功。"}
      </p>
      {p.candidate && (
        <DraftCandidate
          candidate={p.candidate}
          adopted={p.adopted}
          saved={state === "saved"}
          run={run}
          terminal={terminal}
          latest={latest}
          locked={locked}
          perform={perform}
        />
      )}
      {!terminal && (
        <DraftSaveActions
          p={p}
          draft={draft}
          runNotPassing={runNotPassing}
          locked={locked}
          perform={perform}
        />
      )}
    </section>
  );
}

function DraftCandidate({
  candidate,
  adopted,
  saved,
  run,
  terminal,
  latest,
  locked,
  perform,
}: {
  candidate: NonNullable<CreationSnapshot["candidate"]>;
  adopted: CreationSnapshot["adopted"];
  saved: boolean;
  run: ReturnType<typeof findRunObservation>;
  terminal: boolean;
  latest: RunListItem | null | undefined;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <>
      {adopted && <p>已直接採用現有小工具；這個候選版本是它的複本，沒有生成任何內容。</p>}
      <p>
        <Link
          to="/skills/$skillId/versions/$versionId"
          params={{ skillId: candidate.skill_id, versionId: candidate.version_id }}
        >
          {saved ? "開啟已保存的版本" : "開啟候選版本"}
        </Link>
      </p>
      <p>
        {candidate.test_case_id ? (
          <Link
            to="/skills/$skillId/test-cases/$testCaseId/runs/new"
            params={{ skillId: candidate.skill_id, testCaseId: candidate.test_case_id }}
            search={{ version: candidate.version_id }}
          >
            檢查權限與費用後試跑此版本
          </Link>
        ) : (
          <Link
            to="/lab/test-cases"
            search={{ skill: candidate.skill_id, version: candidate.version_id }}
          >
            先建立測試題再試跑此版本
          </Link>
        )}
      </p>
      {candidate.test_case_id && <p>已依確認的驗收條件建立測試題</p>}
      {candidate.run_id ? (
        <>
          <Link to="/runs/$runId" params={{ runId: candidate.run_id }}>
            查看這次試跑結果
          </Link>
          {run && (
            <p>
              試跑結果：{run.execution_status}；評估：
              {run.evaluation?.overall ?? "無評估"}
            </p>
          )}
        </>
      ) : (
        <p>目前尚未連結試跑結果。</p>
      )}
      {!terminal && (
        <LatestRunOffer
          latest={latest}
          linkedRunID={candidate.run_id}
          locked={locked}
          perform={perform}
        />
      )}
    </>
  );
}

function LatestRunOffer({
  latest,
  linkedRunID,
  locked,
  perform,
}: {
  latest: RunListItem | null | undefined;
  linkedRunID: string | undefined;
  locked: boolean;
  perform: Perform;
}) {
  if (latest === null) return null;
  if (!latest) return <p>試跑完成後，這裡會出現「把最新試跑結果帶回來改善」。</p>;
  if (latest.run_id === linkedRunID) return <p>最新試跑結果已帶回會話；模型的建議在對話裡。</p>;
  return (
    <>
      <p>
        最新試跑：{runStatusLabel(latest.status)}；評估：
        {latest.evaluation.label}
      </p>
      <button
        disabled={locked}
        onClick={() => void perform("attach_run", { run_id: latest.run_id })}
      >
        把最新試跑結果帶回來改善
      </button>
    </>
  );
}

function DraftSaveActions({
  p,
  draft,
  runNotPassing,
  locked,
  perform,
}: {
  p: CreationSnapshot;
  draft: NonNullable<CreationSnapshot["draft"]>;
  runNotPassing: boolean;
  locked: boolean;
  perform: Perform;
}) {
  return (
    <>
      <p className={!p.candidate?.run_id || runNotPassing ? "notice notice-warning" : undefined}>
        保存將採用目前顯示的草稿與版本。
        {!p.candidate?.run_id && "這份草稿尚未試跑。"}
        {runNotPassing && "試跑未通過或未評估；保存前請確認。"}
      </p>
      <div className="card-actions">
        {!p.candidate && p.pending_action !== "confirm_duplicate" && (
          <button
            disabled={locked || draft.blocked || !draft.content_hash}
            onClick={() => void perform("materialize", { content_hash: draft.content_hash })}
          >
            建立私人候選版本
          </button>
        )}
        <button
          className="action"
          disabled={locked || draft.blocked || !draft.content_hash}
          onClick={() => void perform("finalize", { content_hash: draft.content_hash })}
        >
          確認保存到私人工作區
        </button>
      </div>
    </>
  );
}

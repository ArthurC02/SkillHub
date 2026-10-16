import {
  EvaluationGraderDraftNameMaxLength,
  EvaluationGraderRevisionInstructionsMaxLength,
} from "@skillhub/api-client-ts/limits";
import { useEffect, useRef, useState } from "react";
import {
  ApiError,
  SessionNotReadyError,
  isTransientReadFailure,
  isWriteResultUnknown,
} from "../../../core/api/client";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import {
  useCreateGrader,
  useGraders,
  useReviseGrader,
  type Grader,
  type GraderScope,
} from "../evaluation.service";
import "./GraderManager.css";

const SCOPE_LABEL: Record<GraderScope, string> = {
  platform: "平台通用",
  workspace: "這個 Workspace 專用",
};

type UnconfirmedWrite = { error: unknown; name: string };

function WriteError({ error, unconfirmed }: { error: unknown; unconfirmed?: UnconfirmedWrite }) {
  const failure = unconfirmed?.error ?? error;
  const feedback = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (failure) feedback.current?.focus();
  }, [failure]);
  return (
    <>
      <ReadFailure error={failure} what="評分器">
        <div ref={feedback} className="grader-write-feedback" role="alert" tabIndex={-1}>
          {isWriteResultUnknown(failure) ? (
            <>
              <p>儲存結果尚未確認。無法確認是否已新增評分器或建立新版本。</p>
              {unconfirmed && <p>這次送出的評分器：{unconfirmed.name}</p>}
              <p>請先按「重新整理」核對現有的評分器，再決定是否重新送出。</p>
            </>
          ) : (
            <p>
              {failure instanceof ApiError && [400, 409].includes(failure.status)
                ? failure.message
                : "這次儲存被拒絕。草稿仍保留，請重新讀取評分器後再試。"}
            </p>
          )}
        </div>
      </ReadFailure>
      {unconfirmed && error instanceof SessionNotReadyError && (
        <ReadFailure error={error} what="評分器" />
      )}
    </>
  );
}

function InstructionsField({
  id,
  value,
  onChange,
  readOnly,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  readOnly: boolean;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>評分指示（評審模型依這段判定；Fan-Out 底下可用 {"{item}"} 代入值）</label>
      <textarea
        id={id}
        value={value}
        maxLength={EvaluationGraderRevisionInstructionsMaxLength}
        rows={6}
        onChange={(e) => onChange(e.target.value)}
        readOnly={readOnly}
      />
    </div>
  );
}

function ReasonField({
  id,
  value,
  onChange,
  readOnly,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  readOnly: boolean;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>這一版改了什麼（必填，跟版本一起保存並寫進動作紀錄）</label>
      <input id={id} value={value} onChange={(e) => onChange(e.target.value)} readOnly={readOnly} />
    </div>
  );
}

type GraderActions = {
  scope: GraderScope;
  onCompleted: (grader: Grader) => void;
  onDraftChange: () => void;
};

function ReviseGrader({
  grader,
  scope,
  onCompleted,
  onDraftChange,
}: GraderActions & { grader: Grader }) {
  const [instructions, setInstructions] = useState(grader.instructions);
  const [reason, setReason] = useState("");
  const [unconfirmed, setUnconfirmed] = useState<UnconfirmedWrite>();
  const revise = useReviseGrader(scope, grader.id, onCompleted);
  const [sourceVersion, setSourceVersion] = useState(grader.version_id);
  if (sourceVersion !== grader.version_id) {
    setSourceVersion(grader.version_id);
    if (!unconfirmed && !revise.isPending) {
      setInstructions(grader.instructions);
      setReason("");
    }
  }
  const id = `grader-${grader.id}`;
  const changed = instructions.trim() !== grader.instructions.trim();
  const ready = changed && instructions.trim() !== "" && reason.trim() !== "";
  return (
    <form
      onInput={() => {
        if (!unconfirmed) revise.reset();
        onDraftChange();
      }}
      onSubmit={(e) => {
        e.preventDefault();
        if (!ready || revise.isPending) return;
        revise.mutate(
          { instructions, reason },
          {
            onError: (error) => {
              if (error instanceof SessionNotReadyError) return;
              setUnconfirmed(
                isWriteResultUnknown(error) ? { error, name: grader.name } : undefined,
              );
            },
            onSuccess: (saved) => {
              setUnconfirmed(undefined);
              setInstructions(saved.instructions);
              setReason("");
            },
          },
        );
      }}
    >
      <InstructionsField
        id={`${id}-instructions`}
        value={instructions}
        onChange={setInstructions}
        readOnly={revise.isPending}
      />
      <ReasonField
        id={`${id}-reason`}
        value={reason}
        onChange={setReason}
        readOnly={revise.isPending}
      />
      <p>
        <button
          type="submit"
          disabled={!ready || revise.isPending}
          aria-describedby={ready ? undefined : `${id}-why`}
        >
          {revise.isPending ? "存成新版本中…" : unconfirmed ? "重新送出新版本" : "存成新版本"}
        </button>{" "}
        {!ready && (
          <span className="note" id={`${id}-why`}>
            {changed ? "要寫評分指示與這一版改了什麼" : "評分指示沒有變更"}
          </span>
        )}
      </p>
      <WriteError error={revise.error} unconfirmed={unconfirmed} />
    </form>
  );
}

function GraderItem({
  grader,
  scope,
  onCompleted,
  onDraftChange,
}: GraderActions & { grader: Grader }) {
  return (
    <li className="criterion">
      <h3>
        {grader.name} v{grader.version}
      </h3>
      <p className="badge-row">
        <span className="badge">{SCOPE_LABEL[grader.scope]}</span>{" "}
        <span className="note">
          最新版本存於 <Timestamp at={grader.revised_at} />
        </span>
      </p>
      <p className="grader-instructions">{grader.instructions}</p>
      {grader.scope === scope && (
        <details>
          <summary>修改「{grader.name}」的評分指示</summary>
          <ReviseGrader
            grader={grader}
            scope={scope}
            onCompleted={onCompleted}
            onDraftChange={onDraftChange}
          />
        </details>
      )}
    </li>
  );
}

function CreateGrader({ scope, onCompleted, onDraftChange }: GraderActions) {
  const [name, setName] = useState("");
  const [instructions, setInstructions] = useState("");
  const [reason, setReason] = useState("");
  const [unconfirmed, setUnconfirmed] = useState<UnconfirmedWrite>();
  const create = useCreateGrader(scope);
  const ready = name.trim() !== "" && instructions.trim() !== "" && reason.trim() !== "";
  return (
    <form
      onInput={() => {
        if (!unconfirmed) create.reset();
        onDraftChange();
      }}
      aria-labelledby="grader-create-heading"
      onSubmit={(e) => {
        e.preventDefault();
        if (!ready || create.isPending) return;
        create.mutate(
          { name, instructions, reason },
          {
            onError: (error) => {
              if (error instanceof SessionNotReadyError) return;
              setUnconfirmed(isWriteResultUnknown(error) ? { error, name } : undefined);
            },
            onSuccess: (grader) => {
              setUnconfirmed(undefined);
              setName("");
              setInstructions("");
              setReason("");
              onCompleted(grader);
            },
          },
        );
      }}
    >
      <h2 id="grader-create-heading">新增評分器（{SCOPE_LABEL[scope]}）</h2>
      <div className="field">
        <label htmlFor="grader-create-name">名稱</label>
        <input
          id="grader-create-name"
          value={name}
          maxLength={EvaluationGraderDraftNameMaxLength}
          onChange={(e) => setName(e.target.value)}
          readOnly={create.isPending}
        />
      </div>
      <InstructionsField
        id="grader-create-instructions"
        value={instructions}
        onChange={setInstructions}
        readOnly={create.isPending}
      />
      <ReasonField
        id="grader-create-reason"
        value={reason}
        onChange={setReason}
        readOnly={create.isPending}
      />
      <p>
        <button
          type="submit"
          disabled={!ready || create.isPending}
          aria-describedby={ready ? undefined : "grader-create-why"}
        >
          {create.isPending ? "新增中…" : unconfirmed ? "重新送出新增" : "新增評分器"}
        </button>{" "}
        {!ready && (
          <span className="note" id="grader-create-why">
            名稱、評分指示與這一版改了什麼都要填
          </span>
        )}
      </p>
      <WriteError error={create.error} unconfirmed={unconfirmed} />
    </form>
  );
}

export function GraderManager({ scope }: { scope: GraderScope }) {
  const graders = useGraders(scope);
  const paused = graders.fetchStatus === "paused";
  const busy = graders.fetchStatus !== "idle";
  const [completed, setCompleted] = useState<Grader>();
  const result = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (completed) result.current?.focus();
  }, [completed]);
  return (
    <>
      {completed && (
        <p
          ref={result}
          id="grader-result"
          className="notice notice-success grader-result"
          role="status"
          tabIndex={-1}
        >
          已儲存「{completed.name}」v{completed.version}
          。已選用的舊版本不受影響；要用新版本請在驗收條件重新選取。
        </p>
      )}
      <details>
        <summary>新增評分器</summary>
        <CreateGrader
          scope={scope}
          onCompleted={setCompleted}
          onDraftChange={() => setCompleted(undefined)}
        />
      </details>
      <h2 id="grader-list-heading" tabIndex={-1}>
        現有的評分器
      </h2>
      <p className="note">
        每次修改都存成新版本；驗收條件的檢查記的是當時選的版本，之後改評分器不會改變已經設計好的判定。
      </p>
      {(graders.isPending || paused || (graders.data?.graders.length === 0 && busy)) && (
        <Loading what="評分器" paused={paused} />
      )}
      <ReadFailure
        error={graders.error}
        what="評分器"
        retry={() => void graders.refetch()}
        retrying={busy}
        focusTarget="grader-list-heading"
      />
      {graders.data && !graders.error && (
        <ListFreshness
          inFlight={false}
          showWhenIdle
          updatedAt={graders.dataUpdatedAt}
          fetching={graders.isFetching}
          paused={paused}
          refetch={graders.refetch}
          subject="評分器"
        />
      )}
      {graders.data &&
        (!graders.error || isTransientReadFailure(graders.error)) &&
        (graders.data.graders.length === 0 ? (
          !graders.error && !busy && <p role="status">還沒有評分器。</p>
        ) : (
          <ul
            className="criterion-list grader-list"
            hidden={Boolean(graders.error)}
            inert={Boolean(graders.error)}
          >
            {graders.data.graders.map((g) => (
              <GraderItem
                key={g.id}
                grader={g}
                scope={scope}
                onCompleted={setCompleted}
                onDraftChange={() => setCompleted(undefined)}
              />
            ))}
          </ul>
        ))}
    </>
  );
}

import { useState } from "react";
import { ApiError } from "../../../core/api/client";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import {
  useCreateGrader,
  useGraders,
  useReviseGrader,
  type Grader,
  type GraderScope,
} from "../evaluation.service";
import "./GraderManager.css";

const MAX_NAME = 100;
const MAX_INSTRUCTIONS = 4000;
const SCOPE_LABEL: Record<GraderScope, string> = {
  platform: "平台通用",
  workspace: "這個 Workspace 專用",
};

function WriteError({ error }: { error: unknown }) {
  return (
    <ReadFailure error={error} what="評分器">
      <p role="alert">
        {error instanceof ApiError && [400, 409].includes(error.status)
          ? error.message
          : "沒有存成功，可以再試一次。"}
      </p>
    </ReadFailure>
  );
}

function InstructionsField({
  id,
  value,
  onChange,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>評分指示（評審模型依這段判定；Fan-Out 底下可用 {"{item}"} 代入值）</label>
      <textarea
        id={id}
        value={value}
        maxLength={MAX_INSTRUCTIONS}
        rows={6}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}

function ReasonField({
  id,
  value,
  onChange,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>這一版改了什麼（必填，跟版本一起保存並寫進動作紀錄）</label>
      <input id={id} value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

function ReviseGrader({ grader, scope }: { grader: Grader; scope: GraderScope }) {
  const [instructions, setInstructions] = useState(grader.instructions);
  const [reason, setReason] = useState("");
  const revise = useReviseGrader(scope, grader.id);
  const id = `grader-${grader.id}`;
  const changed = instructions.trim() !== grader.instructions.trim();
  const ready = changed && instructions.trim() !== "" && reason.trim() !== "";
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) revise.mutate({ instructions, reason }, { onSuccess: () => setReason("") });
      }}
    >
      <InstructionsField
        id={`${id}-instructions`}
        value={instructions}
        onChange={setInstructions}
      />
      <ReasonField id={`${id}-reason`} value={reason} onChange={setReason} />
      <p>
        <button
          type="submit"
          disabled={!ready || revise.isPending}
          aria-describedby={ready ? undefined : `${id}-why`}
        >
          {revise.isPending ? "存成新版本中…" : "存成新版本"}
        </button>{" "}
        {!ready && (
          <span className="note" id={`${id}-why`}>
            {changed ? "要寫評分指示與這一版改了什麼" : "評分指示沒有變更"}
          </span>
        )}
      </p>
      {revise.isSuccess && (
        <p role="status">已存成新版本。已經選用舊版本的驗收條件不受影響，要用新版本請重新選。</p>
      )}
      <WriteError error={revise.error} />
    </form>
  );
}

function GraderItem({ grader, scope }: { grader: Grader; scope: GraderScope }) {
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
      {grader.scope === scope ? (
        <ReviseGrader key={grader.version_id} grader={grader} scope={scope} />
      ) : (
        <p className="grader-instructions">{grader.instructions}</p>
      )}
    </li>
  );
}

function CreateGrader({ scope }: { scope: GraderScope }) {
  const [name, setName] = useState("");
  const [instructions, setInstructions] = useState("");
  const [reason, setReason] = useState("");
  const create = useCreateGrader(scope);
  const ready = name.trim() !== "" && instructions.trim() !== "" && reason.trim() !== "";
  return (
    <form
      aria-labelledby="grader-create-heading"
      onSubmit={(e) => {
        e.preventDefault();
        if (!ready) return;
        create.mutate(
          { name, instructions, reason },
          {
            onSuccess: () => {
              setName("");
              setInstructions("");
              setReason("");
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
          maxLength={MAX_NAME}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <InstructionsField
        id="grader-create-instructions"
        value={instructions}
        onChange={setInstructions}
      />
      <ReasonField id="grader-create-reason" value={reason} onChange={setReason} />
      <p>
        <button
          type="submit"
          disabled={!ready || create.isPending}
          aria-describedby={ready ? undefined : "grader-create-why"}
        >
          {create.isPending ? "新增中…" : "新增評分器"}
        </button>{" "}
        {!ready && (
          <span className="note" id="grader-create-why">
            名稱、評分指示與這一版改了什麼都要填
          </span>
        )}
      </p>
      {create.isSuccess && <p role="status">已新增，驗收條件的檢查現在可以選它。</p>}
      <WriteError error={create.error} />
    </form>
  );
}

export function GraderManager({ scope }: { scope: GraderScope }) {
  const graders = useGraders(scope);
  return (
    <>
      <CreateGrader scope={scope} />
      <h2>現有的評分器</h2>
      <p className="note">
        每次修改都存成新版本；驗收條件的檢查記的是當時選的版本，之後改評分器不會改變已經設計好的判定。
      </p>
      {graders.isPending && <Loading what="評分器" />}
      <ReadFailure error={graders.error} what="評分器" />
      {graders.data &&
        (graders.data.graders.length === 0 ? (
          <p>還沒有評分器。</p>
        ) : (
          <ul className="criterion-list">
            {graders.data.graders.map((g) => (
              <GraderItem key={g.id} grader={g} scope={scope} />
            ))}
          </ul>
        ))}
    </>
  );
}

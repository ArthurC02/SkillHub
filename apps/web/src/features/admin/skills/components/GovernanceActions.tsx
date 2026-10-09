import { useState } from "react";
import { useGovernanceAction, type SkillGovernance } from "../../admin.service";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import { WriteFailure } from "../../components/WriteFailure";
import { ActionForm } from "../../components/ActionForm";

const TAKEDOWN_SCOPE =
  "下架後這個小工具從目錄與搜尋消失，不能再下載或試跑；既有的試跑紀錄仍可追溯。後台目前不提供恢復，下架前請先確認。";

export function GovernanceActions({
  skill,
  onTakedown,
  onOutcome,
  onDraftChange,
}: {
  skill: SkillGovernance;
  onTakedown: () => void;
  onOutcome: (message: string) => void;
  onDraftChange: () => void;
}) {
  const redistribution = useGovernanceAction(skill.skill_id, "redistribution", ({ body }) =>
    onOutcome(
      `「${skill.name}」的再散布判定已改為「${body.value === "allowed" ? "可以再散布" : body.value === "blocked" ? "禁止再散布" : "尚未判定"}」。`,
    ),
  );
  const [verdict, setVerdict] = useState("");
  const [licenseExpression, setLicenseExpression] = useState("");
  const [licenseSource, setLicenseSource] = useState("");
  const releasing = verdict === "allowed";

  return (
    <>
      <h2>對「{skill.name}」的動作</h2>
      <RestrictionAction skill={skill} onOutcome={onOutcome} onDraftChange={onDraftChange} />

      <h3>再散布判定</h3>
      <details id="admin-skill-redistribution" onInput={onDraftChange}>
        <summary>填寫判定與理由</summary>
        <ActionForm
          id="admin-redistribution"
          submitLabel="送出判定"
          pending={redistribution.isPending}
          error={redistribution.error}
          ready={
            verdict !== "" &&
            (!releasing || (licenseExpression.trim() !== "" && licenseSource !== ""))
          }
          contextKey={`${skill.skill_id}:${verdict}:${licenseExpression.trim()}:${licenseSource}`}
          onSubmit={(note) => {
            onDraftChange();
            redistribution.mutate({
              method: "PUT",
              body: releasing
                ? {
                    value: verdict,
                    note,
                    license_expression: licenseExpression.trim(),
                    license_source: licenseSource,
                  }
                : { value: verdict, note },
            });
          }}
        >
          <div className="field">
            <label htmlFor="admin-redistribution-value">判定</label>
            <select
              id="admin-redistribution-value"
              value={verdict}
              required
              onChange={(event) => {
                onDraftChange();
                setVerdict(event.target.value);
                redistribution.reset();
              }}
              disabled={redistribution.isPending}
            >
              <option value="" disabled>
                選擇判定
              </option>
              <option value="blocked">禁止再散布</option>
              <option value="unknown">尚未判定</option>
              <option value="allowed">可以再散布（要附授權證據）</option>
            </select>
          </div>
          {releasing && (
            <>
              <div className="field">
                <label htmlFor="admin-license-expression">授權條款（例如 MIT）</label>
                <input
                  id="admin-license-expression"
                  value={licenseExpression}
                  onChange={(event) => {
                    onDraftChange();
                    setLicenseExpression(event.target.value);
                    redistribution.reset();
                  }}
                  readOnly={redistribution.isPending}
                />
              </div>
              <div className="field">
                <label htmlFor="admin-license-source">證據來源</label>
                <select
                  id="admin-license-source"
                  value={licenseSource}
                  onChange={(event) => {
                    onDraftChange();
                    setLicenseSource(event.target.value);
                    redistribution.reset();
                  }}
                  disabled={redistribution.isPending}
                >
                  <option value="">選一個</option>
                  <option value="manifest">SKILL.md 的宣告</option>
                  <option value="manifest-referenced-file">SKILL.md 指向的授權檔</option>
                  <option value="package-license-file">套件裡的授權檔</option>
                  <option value="repo-license-file">儲存庫根目錄的授權檔</option>
                </select>
              </div>
            </>
          )}
        </ActionForm>
      </details>

      <TakedownAction skillId={skill.skill_id} onTakedown={onTakedown} />
    </>
  );
}

function RestrictionAction({
  skill,
  onOutcome,
  onDraftChange,
}: {
  skill: SkillGovernance;
  onOutcome: (message: string) => void;
  onDraftChange: () => void;
}) {
  const restriction = useGovernanceAction(skill.skill_id, "restriction", ({ method }) =>
    onOutcome(
      method === "DELETE"
        ? `「${skill.name}」已解除受限展示。`
        : `「${skill.name}」已設定受限展示。`,
    ),
  );

  return (
    <>
      <h3>{skill.access_restriction ? "解除受限展示" : "設定受限展示"}</h3>
      <p className="note">受限展示關掉全文與試跑，小工具仍在搜尋裡。可以用同一個地方改回來。</p>
      <details id="admin-skill-restriction" onInput={onDraftChange}>
        <summary>填寫變更理由</summary>
        <ActionForm
          key={skill.access_restriction ?? "unrestricted"}
          id="admin-restriction"
          submitLabel={skill.access_restriction ? "解除受限" : "設定受限"}
          pending={restriction.isPending}
          error={restriction.error}
          onSubmit={(note) => {
            onDraftChange();
            if (skill.access_restriction) restriction.mutate({ method: "DELETE", body: { note } });
            else restriction.mutate({ method: "PUT", body: { reason: "license-review", note } });
          }}
        />
      </details>
    </>
  );
}

function TakedownAction({ skillId, onTakedown }: { skillId: string; onTakedown: () => void }) {
  const takedown = useGovernanceAction(skillId, "takedown", onTakedown);
  const [takedownReason, setTakedownReason] = useState("");

  return (
    <>
      <h3>下架</h3>
      <p id="admin-takedown-consequences" className="note">
        {TAKEDOWN_SCOPE}
      </p>
      <details id="admin-skill-takedown">
        <summary aria-describedby="admin-takedown-consequences">填寫下架理由</summary>
        <div className="field">
          <label htmlFor="admin-takedown-reason">下架理由（必填，會寫進動作紀錄）</label>
          <input
            id="admin-takedown-reason"
            value={takedownReason}
            onChange={(event) => setTakedownReason(event.target.value)}
            readOnly={takedown.isPending}
          />
        </div>
        {takedownReason.trim() === "" ? (
          <p className="note">填了理由才能下架。</p>
        ) : (
          <ConfirmDelete
            key={takedownReason.trim()}
            scopeId="admin-takedown-scope"
            scope={TAKEDOWN_SCOPE}
            pending={takedown.isPending}
            label="下架"
            confirmLabel="確認下架"
            onConfirm={() =>
              takedown.mutate({ method: "PUT", body: { reason: takedownReason.trim() } })
            }
          />
        )}
        <WriteFailure error={takedown.error} />
      </details>
    </>
  );
}

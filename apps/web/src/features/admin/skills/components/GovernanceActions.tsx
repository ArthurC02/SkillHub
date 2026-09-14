import { useState } from "react";
import { useGovernanceAction, type SkillGovernance } from "../../admin.service";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import { WriteFailure } from "../../components/WriteFailure";
import { ActionForm } from "../../components/ActionForm";

export function GovernanceActions({ skill }: { skill: SkillGovernance }) {
  const restriction = useGovernanceAction(skill.skill_id, "restriction");
  const redistribution = useGovernanceAction(skill.skill_id, "redistribution");
  const takedown = useGovernanceAction(skill.skill_id, "takedown");
  const [verdict, setVerdict] = useState("blocked");
  const [licenseExpression, setLicenseExpression] = useState("");
  const [licenseSource, setLicenseSource] = useState("");
  const [takedownReason, setTakedownReason] = useState("");
  const releasing = verdict === "allowed";

  return (
    <>
      <h2>對「{skill.name}」的動作</h2>
      <h3>{skill.access_restriction ? "解除受限展示" : "設定受限展示"}</h3>
      <p className="note">受限展示關掉全文與試跑，Skill 仍在搜尋裡。可以用同一個地方改回來。</p>
      <ActionForm
        id="admin-restriction"
        submitLabel={skill.access_restriction ? "解除受限" : "設定受限"}
        pending={restriction.isPending}
        error={restriction.error}
        done={restriction.isSuccess && "已送出，上面的狀態已更新。"}
        onSubmit={(note) =>
          skill.access_restriction
            ? restriction.mutate({ method: "DELETE", body: { note } })
            : restriction.mutate({ method: "PUT", body: { reason: "license-review", note } })
        }
      />

      <h3>再散布判定</h3>
      <ActionForm
        id="admin-redistribution"
        submitLabel="送出判定"
        pending={redistribution.isPending}
        error={redistribution.error}
        ready={!releasing || (licenseExpression.trim() !== "" && licenseSource !== "")}
        done={redistribution.isSuccess && "已送出，上面的狀態已更新。"}
        onSubmit={(note) =>
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
          })
        }
      >
        <div className="field">
          <label htmlFor="admin-redistribution-value">判定</label>
          <select
            id="admin-redistribution-value"
            value={verdict}
            onChange={(event) => setVerdict(event.target.value)}
          >
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
                onChange={(event) => setLicenseExpression(event.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="admin-license-source">證據來源</label>
              <select
                id="admin-license-source"
                value={licenseSource}
                onChange={(event) => setLicenseSource(event.target.value)}
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

      <h3>下架</h3>
      <div className="field">
        <label htmlFor="admin-takedown-reason">下架理由（必填，會寫進動作紀錄）</label>
        <input
          id="admin-takedown-reason"
          value={takedownReason}
          onChange={(event) => setTakedownReason(event.target.value)}
        />
      </div>
      {takedownReason.trim() === "" ? (
        <p className="note">填了理由才能下架。</p>
      ) : (
        <ConfirmDelete
          scopeId="admin-takedown-scope"
          scope="下架後這個 Skill 從目錄與搜尋消失，不能再下載或試跑；既有的 Run 仍可追溯。下架沒有恢復的路。"
          pending={takedown.isPending}
          label="下架"
          confirmLabel="確認下架"
          onConfirm={() =>
            takedown.mutate({ method: "PUT", body: { reason: takedownReason.trim() } })
          }
        />
      )}
      <WriteFailure error={takedown.error} />
    </>
  );
}

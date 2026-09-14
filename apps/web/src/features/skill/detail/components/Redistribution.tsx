import { LabelledBadge } from "../../../../shared/ui/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../../../../shared/ui/LicenseBadge";
import { PACKAGING_BLOCKED_LABEL, packagingGate } from "../../../packaging";
import type { SkillDetail as SkillDetailModel } from "../../../../core/api/types";
import { PackagingEntry } from "./PackagingEntry";

export function Redistribution({
  skill,
  isLoggedIn,
}: {
  skill: SkillDetailModel;
  isLoggedIn: boolean;
}) {
  const blocked = packagingGate(skill);

  return (
    <section>
      <h2>可散布性與打包</h2>
      {skill.redistribution ? (
        <>
          <p>
            <LabelledBadge kind="redistribution" value={skill.redistribution} />
          </p>
        </>
      ) : (
        <p className="note">平台沒有回報這個 Skill 的可散布性判定。</p>
      )}

      {blocked ? (
        <>
          <p>
            <button type="button" disabled aria-describedby="packaging-blocked-reason">
              打包並下載
            </button>
          </p>
          <p className="note" id="packaging-blocked-reason">
            {PACKAGING_BLOCKED_LABEL[blocked]}
          </p>
        </>
      ) : skill.version ? (
        <PackagingEntry skill={skill} isLoggedIn={isLoggedIn} />
      ) : (
        <p className="note">
          無權檢視——這個工作區看不到這個 Skill
          的版本內容，所以沒有東西可以打包（原因見下面的〈版本〉）。
        </p>
      )}

      <h3>License</h3>
      <LicenseBadge license={skill.license} />
      <LicenseNotes license={skill.license} />
    </section>
  );
}

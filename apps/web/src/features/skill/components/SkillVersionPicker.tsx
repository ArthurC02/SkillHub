import { useSkillVersions } from "../skills.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { formatAt } from "../../../shared/ui/Timestamp";

export function SkillVersionPicker({
  skillId,
  value,
  onPick,
}: {
  skillId: string;
  value: string;
  onPick: (versionId: string) => void;
}) {
  const versions = useSkillVersions(skillId);
  const list = versions.data?.versions ?? [];
  const unknown = value !== "" && !list.some((v) => v.version_id === value);
  const id = `skill-version-${skillId}`;

  return (
    // div, not p: Loading/ReadFailure below can render block elements, which <p> can't contain.
    <div>
      <label htmlFor={id}>Skill Version</label>{" "}
      <select id={id} value={value} onChange={(e) => onPick(e.target.value)}>
        {value === "" && <option value="">請選擇版本…</option>}
        {unknown && <option value={value}>{value}（不在下面的清單裡）</option>}
        {list.map((v, i) => (
          <option key={v.version_id} value={v.version_id}>
            v{v.version_number}
            {i === 0 ? "（最新）" : ""}・{formatAt(v.created_at)}
          </option>
        ))}
      </select>{" "}
      {versions.isPending && <Loading what="版本清單" className="note" />}
      <ReadFailure error={versions.error} what="版本清單">
        <span className="note" role="alert">
          無法讀取版本清單：{versions.error?.message}
        </span>
      </ReadFailure>
      {!versions.isPending && !versions.error && list.length === 0 && (
        <span className="note">
          這個工作區沒有這個 Skill 的任何版本可選——不代表這個 Skill 沒有版本，Fork
          之後才會有屬於你的版本。
        </span>
      )}
    </div>
  );
}

import { useSkillVersions } from "../skills.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { formatAt } from "../../../shared/ui/Timestamp.model";

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
  const list = versions.error ? [] : (versions.data?.versions ?? []);
  const unreadable = versions.isPending || Boolean(versions.error);
  const unknown = value !== "" && !list.some((v) => v.version_id === value);
  const id = `skill-version-${skillId}`;
  const groupId = `${id}-group`;
  const statusId = `${id}-read-status`;

  return (
    // div, not p: Loading/ReadFailure below can render block elements, which <p> can't contain.
    <div id={groupId} tabIndex={-1} role="group" aria-label="小工具版本選擇">
      <label htmlFor={id}>小工具 Version</label>{" "}
      <select
        id={id}
        value={value}
        onChange={(e) => onPick(e.target.value)}
        disabled={unreadable}
        aria-describedby={unreadable ? statusId : undefined}
      >
        {value === "" && <option value="">請選擇版本…</option>}
        {unknown && (
          <option value={value}>
            {value}（{unreadable ? "清單尚未確認" : "不在下面的清單裡"}）
          </option>
        )}
        {list.map((v, i) => (
          <option key={v.version_id} value={v.version_id}>
            v{v.version_number}
            {i === 0 ? "（最新）" : ""}・{formatAt(v.created_at)}
          </option>
        ))}
      </select>{" "}
      <div id={statusId}>
        {versions.isPending && <Loading what="版本清單" className="note" />}
        <ReadFailure
          error={versions.error}
          what="版本清單"
          retry={() => void versions.refetch()}
          retrying={versions.isFetching}
          focusTarget={groupId}
        />
      </div>
      {!versions.isPending && !versions.error && list.length === 0 && (
        <span className="note">
          這個工作區沒有這個小工具的任何版本可選——不代表這個小工具沒有版本，複製一份
          之後才會有屬於你的版本。
        </span>
      )}
    </div>
  );
}

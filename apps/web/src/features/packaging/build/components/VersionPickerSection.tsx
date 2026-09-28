import { useNavigate } from "@tanstack/react-router";
import { SkillVersionPicker } from "../../../skill";

export function VersionPickerSection({
  skillId,
  versionId,
}: {
  skillId: string;
  versionId: string;
}) {
  const navigate = useNavigate();

  return (
    <details>
      <summary>換一個版本打包，或看這個版本的識別碼</summary>
      <SkillVersionPicker
        skillId={skillId}
        value={versionId}
        onPick={(id) =>
          void navigate({
            to: "/skills/$skillId/package",
            params: { skillId },
            search: { version: id },
          })
        }
      />
      <p className="note">
        打包的是這一個不可變版本 <code>{versionId}</code>
        ；打包不會建立也不會修改任何版本，每按一次得到的是一筆 Download Artifact。
      </p>
    </details>
  );
}

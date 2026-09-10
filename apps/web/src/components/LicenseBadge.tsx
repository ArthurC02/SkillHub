import type { SkillLicense } from "../api/types";
import { LabelledBadge } from "./LabelledBadge";

export const SOURCE_LABELS: Record<string, string> = {
  manifest: "來源：套件 frontmatter 宣告",
  "manifest-referenced-file": "來源：frontmatter 指向的套件內檔案",
  "package-license-file": "來源：套件內 LICENSE 檔",
  "repo-license-file": "來源：repo 根目錄 LICENSE",
};

export function LicenseBadge({ license }: { license: SkillLicense }) {
  return (
    <span className="license-badge">
      <LabelledBadge kind="license" value={license.status} />
      {license.expression && <code className="license-expression">{license.expression}</code>}
      {license.source && (
        <span className="badge badge-license-source">
          {SOURCE_LABELS[license.source] ?? license.source}
        </span>
      )}
    </span>
  );
}

export function LicenseNotes({ license }: { license: SkillLicense }) {
  return (
    <>
      {license.source_note && <p className="note">{license.source_note}</p>}
      {!license.source && license.expression && (
        <p className="note">此版本未記錄 License 的取得來源，無法判斷宣告的涵蓋範圍。</p>
      )}
    </>
  );
}

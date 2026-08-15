import type { SkillLicense } from "../api/types";
import { LabelledBadge } from "./LabelledBadge";

/**
 * ADR-021 has two axes and both must be visible: *which* license the package
 * evidences (`expression`) and *how strong that evidence is* (`source` tier,
 * plus the `status` label separating 已宣告 from 已人工確認). The expression
 * alone cannot tell "the author declared MIT in frontmatter" from "the monorepo
 * root happened to have an MIT file", and DISC-003 forbids showing the second
 * as if it were the first.
 *
 * With no expression the badge says only what the server's status label says
 * (License 未知) — never a license name, never anything implying the skill is
 * free to modify or redistribute.
 */
const SOURCE_LABELS: Record<string, string> = {
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

/** The prose half of the two axes: what the tier does and does not claim. */
export function LicenseNotes({ license }: { license: SkillLicense }) {
  return (
    <>
      <p className="note">{license.status.note}</p>
      {license.source_note && <p className="note">{license.source_note}</p>}
      {!license.source && license.expression && (
        <p className="note">此版本未記錄 License 的取得來源，無法判斷宣告的涵蓋範圍。</p>
      )}
    </>
  );
}

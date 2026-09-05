import { Link } from "@tanstack/react-router";

/**
 * GEN-004: two named absences, on the detail page, on the workspace list and
 * on the screen that has just generated one.
 *
 * In components/ and not next to any one of them for the reason Findings.tsx
 * gives: three copies of a warning is how one of the three quietly becomes
 * reassurance. The generation result had already started — it said 「這份內容是
 * 模型剛剛寫出來的」 and dropped the link to the first trial run.
 *
 * Two, not one, and neither is 「新建立」: nobody has looked at this package, and
 * nothing has run it. ADR-041 決策 2 makes absence a value rather than a blank,
 * and 02:GEN-004 names this wording specifically — a neutral word here would
 * describe a package the platform wrote thirty seconds ago as if it were merely
 * recent.
 */
export function GeneratedNotice({ skillId }: { skillId?: string }) {
  return (
    <>
      <p className="badge badge-unverified">沒有經過任何人工檢視，沒有任何試跑證據</p>
      <p className="note">
        這份內容是平台生成的。它通過的只有格式與靜態檢查，
        <strong>那不是品質、可用性或安全的結論</strong>。
        {skillId ? (
          <>
            {" "}
            <Link
              to="/lab/run"
              search={{ skill: skillId, version: undefined, test_case: undefined }}
            >
              先跑一次試跑
            </Link>
            ，才會有第一份證據。
          </>
        ) : (
          " 先跑一次試跑，才會有第一份證據。"
        )}
      </p>
    </>
  );
}

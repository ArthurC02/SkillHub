import { Link } from "@tanstack/react-router";

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

import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../../shared/ui/LoginRequired.model";
import { Link } from "@tanstack/react-router";
import { useSkillVersions } from "../../skills.service";

// Ownership signal is the (workspace-scoped) versions list, empty for a non-owner —
// not skill.version, which is present for every caller including the catalogue.
// Own component (not inlined) so the page's early returns can't shift hook order.
export function TrialEntry({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const versions = useSkillVersions(skillId);

  if (!isLoggedIn)
    return (
      <section>
        <h2>試跑</h2>
        <p>試跑需要你工作區裡的版本。從下方「複製一份到你的工作區」開始。</p>
      </section>
    );

  if (versions.isPending)
    return (
      <section>
        <h2>試跑</h2>
        <Loading what="這個小工具在你工作區的版本" />
      </section>
    );
  if (unauthenticated(versions.error))
    return (
      <section>
        <h2>試跑</h2>
        <p role="status">試跑需要登入。請從下方的「複製一份到你的工作區」重新登入。</p>
      </section>
    );
  if (versions.error)
    return (
      <section>
        <h2>試跑</h2>
        <ReadFailure error={versions.error} what="這個小工具的版本" />
      </section>
    );

  const versionId = versions.data?.versions[0]?.version_id;

  return (
    <section>
      <h2>試跑</h2>
      {versionId ? (
        <>
          <p>
            <Link to="/lab/test-cases" search={{ skill: skillId, version: versionId }}>
              此小工具的測試題
            </Link>
          </p>
          <p className="note" data-role="teaching">
            測試題是試跑用的草稿：User Prompt、測試資料與驗收條件。
          </p>
        </>
      ) : (
        <p>
          這個小工具不在你的工作區。測試題屬於工作區，所以測試題清單裡看不到它、建立表單的小工具選單也選不到它——
          <strong>要先複製一份</strong>
          ，才會有屬於你的版本可以試跑。下方的「複製一份到你的工作區」就是那一步。
        </p>
      )}
    </section>
  );
}

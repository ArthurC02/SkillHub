import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useSkillVersions } from "../../skills.service";
import { SignInAction } from "../../../../shared/ui/SignIn";

// Ownership signal is the (workspace-scoped) versions list, empty for a non-owner —
// not skill.version, which is present for every caller including the catalogue.
// Own component (not inlined) so the page's early returns can't shift hook order.
export function TrialEntry({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const versions = useSkillVersions(skillId);

  if (!isLoggedIn)
    return (
      <section>
        <h3>試跑</h3>
        <div>
          試跑屬於你的工作區。先登入並 Fork 一份，才會有屬於你的版本可以跑。 <SignInAction />
        </div>
      </section>
    );

  if (versions.isPending)
    return (
      <section>
        <h3>試跑</h3>
        <Loading what="這個 Skill 在你工作區的版本" />
      </section>
    );
  if (versions.error)
    return (
      <section>
        <h3>試跑</h3>
        <ReadFailure error={versions.error} what="這個 Skill 的版本" />
      </section>
    );

  const inMyWorkspace = (versions.data?.versions.length ?? 0) > 0;

  return (
    <section>
      <h3>試跑</h3>
      {inMyWorkspace ? (
        <>
          <p>
            <Link to="/lab/test-cases" search={{ skill: skillId }}>
              此 Skill 的 Test Case
            </Link>
          </p>
          <p className="note" data-role="teaching">
            Test Case 是試跑用的草稿：User Prompt、測試資料與驗收條件。
          </p>
        </>
      ) : (
        <p>
          這個 Skill 不在你的工作區。Test Case 屬於工作區，所以 Test Case 清單裡看不到它、建立表單的
          Skill 選單也選不到它——
          <strong>要先 Fork 一份</strong>，才會有屬於你的版本可以試跑。下方的「Fork
          到你的工作區」就是那一步。
        </p>
      )}
    </section>
  );
}

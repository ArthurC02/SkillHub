import { Link } from "@tanstack/react-router";
import { SignInAction } from "../../../../shared/ui/SignIn";
import { GenerateSkill } from "../../../creation";

export function NoResultsPanel({
  query,
  degraded,
  querySuggestion,
  loggedIn,
  generateExposed,
}: {
  query: string;
  degraded: boolean;
  querySuggestion: string | undefined;
  loggedIn: boolean;
  generateExposed: boolean;
}) {
  return (
    <div>
      <p>沒有夠接近的 Skill。</p>
      {degraded ? (
        <p>
          而且這次搜尋只用了關鍵字比對，語意相近與跨語言的結果找不出來——現在找不到不代表
          目錄裡沒有。換個說法幫不上忙；直接看目錄，或稍後再搜尋一次。
        </p>
      ) : (
        querySuggestion && <p>{querySuggestion}</p>
      )}
      <p className="note">
        <Link to="/" search={{}}>
          看看目錄裡有什麼
        </Link>
        ——不帶任何查詢，列出這個部署收錄的全部 Skill。
      </p>
      <div className="note">
        {loggedIn ? (
          <>
            手上已經有一個 Skill 套件的話，也可以
            <Link to="/workspace/import">直接匯入它</Link>。
          </>
        ) : (
          <>
            手上已經有一個 Skill 套件的話，登入後可以把它匯入你自己的工作區。 <SignInAction />
          </>
        )}
      </div>
      {generateExposed && <GenerateSkill initialTask={query} />}
    </div>
  );
}

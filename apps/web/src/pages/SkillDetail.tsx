import { useState } from "react";
import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { Timestamp } from "../components/Timestamp";
import { VersionDiff } from "./RunCompare";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useForkSkill, useSkillDetail, useSkillVersions, skillDiffUrl } from "../api/skills";
import { useMe } from "../api/me";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { GeneratedNotice } from "../components/GeneratedNotice";
import { LabelledBadge } from "../components/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../components/LicenseBadge";
import { RiskIndicator } from "../components/RiskIndicator";
import { SignInAction } from "../components/SignIn";
import { VersionUpload } from "../components/VersionUpload";
import { PACKAGING_BLOCKED_LABEL, packagingGate } from "./Packaging";
import type {
  SkillDetail as SkillDetailModel,
  SkillEnrichment,
  SkillLimitation,
  SkillSource,
  SkillTags,
} from "../api/types";

/**
 * DISC-006/008: general-mode skill detail, reading GET /api/skills/{id}.
 * Anonymous callers get the public catalog (DISC-010).
 *
 * Progressive disclosure: the plain-language answer is on the page, and the
 * identifiers that only matter when you are checking someone's work (hashes,
 * version ids, which model wrote the summary) sit behind <details>.
 *
 * --- 2026-09-03, r2「產品資訊展示太多細節」 --------------------------------
 *
 * 量到的不是「字太多」，是「東西太多」：整個 `<main>` 去掉空白只有 **1231 個字元**，
 * 卻攤在 **2930px** 上（平均 2.4px 才一個字），而同一頁有 **15 個等寬全寬區塊、
 * 12 個互為兄弟的 h2**。丙-133 已經動過的兩件（40em 行長上限、操作三區塊上移）讓頁
 * 面**變高**而不是變矮，所以剩下的槓桿只有一個：**減少並列區塊的數量、把它們排成
 * 兩欄**——§0 說的「讓步的是形式」。
 *
 * **一個字都沒有被刪。** §2.10 的十項一項都沒有進 `<details>`；搬進 `<details>` 的
 * 是任務範例、逐版清單與識別碼，三者都在 §2.6／r2 §3.2 明列的「可折」那一欄。
 *
 * 這一版的形狀：
 *  - `detail-layout` 兩欄（≥1024px）：主欄是**證據**，右側 `detail-rail` 是**操作**，
 *    每個控制項連同它的理由一起搬（丙-133 的「搬 section 不搬 button」——§2.4 與
 *    §2.10 第 5 項）。≤1024 沒有 grid，rail 直接跟在主欄後面，DOM 順序就是閱讀順序。
 *  - `verdict-grid` 2×2：風險揭露／可散布性與打包（含 License）／相容性／套件宣告可用
 *    的工具。**工具那一段從整頁 77% 的位置搬到第一屏**——§2.10 第 6 項（執行前的權限
 *    與工具清單）。
 *  - h2 由 **12 → 7**（§3 第 9 條，失效樣本逐字就是「一頁八個 h2 互為兄弟」），從屬段落
 *    降 h3：License、限制、來源關係，以及右欄的三個操作。
 *
 * **License 為什麼從 h2 降成「可散布性與打包」底下的 h3**：§2.10 第 3 項把
 * 「License status ＋ 可散布性 badge」寫成**同一項**，而這一頁上 License 的作用就是
 * 可散布性判定的證據——`PACKAGING_BLOCKED_LABEL` 的 `license_unknown` 是打包被擋的
 * 主要理由。兩個徽章、兩句但書一個字都沒動，變的只有它們現在在同一個區塊裡（§2.11(c)
 * 要的正是「同一個區塊」）。丙-137 拿掉的是**重複的那一份**，這裡沒有再減少任何一份。
 */
export function SkillDetail() {
  const { skillId } = useParams({ from: "/skills/$skillId" });
  const { data: skill, isLoading, error } = useSkillDetail(skillId);
  const { data: me } = useMe();

  if (isLoading) return <Loading what="這個 Skill" />;
  // 410 is a different fact from 404: this skill existed and was withdrawn.
  if (error instanceof ApiError && error.status === 410) {
    return <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>;
  }
  // Everything else through the shared component (資訊架構 §5 IA-6): a 401 says
  // to log in, and every other status keeps the server's own message instead of
  // being flattened into 「找不到這個 Skill，或載入失敗。」 — which answered a 500
  // with 「找不到」, i.e. with the wrong fact.
  if (error) return <ReadFailure error={error} what="這個 Skill" />;
  if (!skill) return <p role="alert">找不到這個 Skill。</p>;

  return (
    <article>
      <div className="detail-layout">
        <div className="detail-main">
          <header className="detail-answer">
            <h1>{skill.name}</h1>
            {/* The package author's own frontmatter description, never the model's. */}
            <p>{skill.summary}</p>
            {/*
              04 丙-137。License 判定以前在這一頁出現兩次——這裡與下方的 §License 區塊
              渲染的是**同一個元件、同一組 props**，所以連伺服器那句限定語都印了兩遍
              （實測 2026-09-03：「License 已宣告」「尚未經人工核對。」「來源：repo 根目錄
              LICENSE」三個文字節點各 x2）。設計 §3 第 14 條，而 `LabelledBadge` 的檔頭
              自己寫著「Callers that printed the same string themselves have stopped」。

              **為什麼只能是「拿掉一個徽章」而不是「拿掉一句但書」**：§2.11(c) 要求每一個
              徽章在同一個區塊以文字說出它不涵蓋什麼，所以兩個徽章必然是兩份但書——留一個
              沒有但書的徽章會把 §3 第 14 條的問題換成 §2.11(c) 的問題，而後者在 §0 的
              順位表上高三階。

              **為什麼留下面那一個**：它有標題（可被標題導覽到）、有 `LicenseNotes` 這半段
              散文陪著，而 §2.10 第 3 項要的「不折疊」它照樣滿足。實測 y=657（桌面）／
              y=855（手機），兩者都在第一屏之內，所以第一屏沒有失去這個判定。相對地標頭
              這一列在手機上是 **147px 四行**，三個徽章各帶一句但書；少一個就少約兩行，
              而桌面那一行本來就只有 36px、拿掉不省任何東西。**這不是版面優化，是
              §3 第 14 條；版面只是決定了刪哪一個。**

              類別徽章排在來源層級之前：它回答的是「這是什麼東西」，另外兩個回答「可不可
              信」，而讀者是先問前者才問後者。每一顆一樣帶著伺服器那句但書（§2.11(c)），
              `LabelledBadge` 就是把 `note` 印成可見文字的那個元件。
            */}
            <div className="badge-row">
              <LabelledBadge kind="category" value={skill.category} />
              <LabelledBadge kind="tier" value={skill.tier} />
              {skill.source && <LabelledBadge kind="trust" value={skill.source.trust} />}
            </div>
          </header>

          {/*
            0023 licensing hold. Above everything else on the page, because it
            changes what the rest of the page can be used for: no full text, no file
            tree, no run. role="status" rather than "alert" — it is a standing
            condition of this listing, not something that just went wrong.
          */}
          {/*
            設計 §4.6.3（ADR-064）：`notice-danger`。這一則說的是「這些事現在**做不
            到**」——全文、檔案樹、試跑三個都被關掉了（見上面那段），也就是阻斷，不是
            降級。`role="status"` 不變：它仍然是這份 listing 的持續狀態，不是剛剛出的
            錯；底色是第二訊號（§2.3），不是把它改叫錯誤。

            這一段是唯一會讓這一頁出現第八個 h2 的分支，而它是對的：一則阻斷公告不該
            降成 h3 去遷就一個計數。
          */}
          {skill.access_restriction && (
            <section className="notice notice-danger" role="status">
              <h2>授權審查中,部分功能已關閉</h2>
              <p>{skill.access_restriction.note}</p>
            </section>
          )}

          {/*
            The four human-checkable verdicts come first (system.md §1.1 + §3 item
            1). They used to sit below ~400px of model-written 白話摘要: 可散布性 at
            roughly y1174, 風險揭露 at y1297, 相容性 at y1507 in a 900px viewport, so
            on the page where 「這個別人寫的東西可不可信」 is asked, the first screen was
            half model output and the scan result was 1.4 viewports down.

            The AI summary losing its top slot is the intent, not a side effect
            (§2.6): a model's restatement of the package is not the answer to that
            question, and it is the one block here that nobody has checked.

            2026-09-03：四個判定改成 2×2（`verdict-grid`）。**兩欄讓行變短、不是變長**：
            實測最長的一條排版行是 720px ＝ 40em × 18px，也就是 §4.5 的行長上限正好咬住，
            每一段文字右邊有 358px 永遠空白；519px 的欄寬仍然在上限之下。省 −410px，而且
            第四個判定不再落在 y926。
          */}
          <div className="verdict-grid">
            <section>
              <h2>風險揭露</h2>
              <RiskIndicator risk={skill.risk} />
            </section>

            <Redistribution skill={skill} isLoggedIn={!!me} />

            <section>
              <h2>相容性</h2>
              <CompatibilityStatus compatibility={skill.compatibility} />
            </section>

            {/*
              設計 §2.9（缺席有型別）＋ §2.10 第 6 項（工具清單不得靠互動才看得到）。
              This section used to render only when the list had entries, so a package
              that declares no tools and a package that could not be scanned came out
              as the same thing: no heading, no words, nothing. And of the three
              readings that blank invites, the true one is the widest — in the Agent
              Skills format an ABSENT `allowed-tools` means UNRESTRICTED, so the blank
              rendered the most permissive fact in the shape the reader trusts most.
              The server only fills this field when there is a scan report to read the
              manifest from (discovery/detail.go), and `risk.scan_status` is that same
              fact under a name, so the two absences are told apart without a new
              field. Same call this file already made for `limitations` and
              `enrichment.tags`; this was the one block that missed it.

              **搬上來的是整個 `<section>`，不是那張清單**（r2 A1）：實測 2026-09-03 它
              在 y2244，也就是整頁的 77%，而 §2.10 第 6 項是「執行前」要看得到的東西。
              「不適用＝不設限」那句是它的但書，跟著它一起搬——與丙-133 的「搬 section
              不搬 button」同一條理由。
            */}
            <section>
              <h2>套件宣告可用的工具</h2>
              {skill.allowed_tools && skill.allowed_tools.length > 0 ? (
                <>
                  <ul>
                    {skill.allowed_tools.map((tool) => (
                      <li key={tool}>
                        <code>{tool}</code>
                      </li>
                    ))}
                  </ul>
                  <p className="note">以上為套件自行宣告的 allowed-tools，未經驗證。</p>
                </>
              ) : skill.risk.scan_status === "unavailable" ? (
                <p className="note">
                  未測量——這個版本沒有靜態掃描結果可讀，所以平台不知道套件宣告了哪些工具。
                </p>
              ) : (
                <p className="note">
                  不適用——套件沒有宣告 allowed-tools。在 Agent Skills 的格式裡那代表
                  <strong>不設限</strong>，不代表它不用工具。
                </p>
              )}
            </section>
          </div>

          {/*
            「它能做什麼」＝ 原本的〈白話摘要〉＋〈限制〉。兩者都是**同一個問題的兩半**
            （這東西做得到什麼、做不到什麼），而它們以前是兩個互為兄弟的 h2，中間還隔著
            一段 GeneratedNotice。§3 第 9 條。
          */}
          <section>
            <h2>它能做什麼</h2>
            <Enrichment enrichment={skill.enrichment} />
            <Limitations limitations={skill.limitations} />
          </section>

          {/*
            GEN-004 wants this on the detail page and on the workspace list, and the
            two have to answer it the same way. This used to read the VERSION's
            source (`skill.source.type`), which is `upload` for any version the user
            saved themselves — so the moment a generated skill got a second version,
            the detail page silently dropped the two absences while the list, which
            reads the skill row, went on showing them. The skill row is the right
            source: `redistribution` is what GEN-007's search exclusion keys on, so
            it is exactly the set of skills the disclosure is about. Same expression
            as WorkspaceSkills.tsx, one level deeper because detail sends a Labelled.
          */}
          {skill.redistribution?.value === "generated" && (
            <GeneratedNotice skillId={skill.skill_id} />
          )}

          {/*
            「它從哪裡來」＝ 原本的〈來源〉＋〈來源關係〉。後者是一個 h2 ＋ 一個 section
            服務 **10 個字元**（fixture 是「非 Fork。」），r2 A3 量到 −62px。它不在 §2.10
            的十項裡，而它講的就是這一份東西的出處，所以它是〈來源〉的 h3，不是兄弟。
          */}
          <section>
            <h2>它從哪裡來</h2>
            {skill.source ? <SourceBlock source={skill.source} /> : <p>沒有保存任何來源紀錄。</p>}

            <h3>{skill.derivation.label}</h3>
            <p className="note">{skill.derivation.note}</p>
            {skill.derivation.is_fork && skill.derivation.forked_from_skill_id && (
              <p>
                <Link
                  to="/skills/$skillId"
                  params={{ skillId: skill.derivation.forked_from_skill_id }}
                >
                  查看原始 Skill
                </Link>
              </p>
            )}
          </section>

          <VersionHistory skillId={skillId} />

          <details>
            <summary>進階資訊（版本與識別碼）</summary>
            {skill.version ? (
              <ul>
                <li>版本編號：v{skill.version.version_number}</li>
                <li>
                  版本 ID：<code>{skill.version.version_id}</code>
                </li>
                <li>
                  內容雜湊：<code>{skill.version.content_hash}</code>
                </li>
                <li>
                  建立時間：
                  <Timestamp at={skill.version.created_at} />
                </li>
              </ul>
            ) : (
              /* 設計 §2.9: 別人的 Skill 的版本清單回空陣列，而「沒有版本」與
                 「你看不到」是兩件事——前者會被讀成這個 Skill 是空的。表列詞是
                 「無權檢視」（ADR-011 的 Workspace scope）。
                 2026-09-03（丙-142）：型別詞留著（§2.10 第 10 項），解釋只在上面的
                 〈版本〉區塊講一次——這一格與那一格同時渲染，字一模一樣。 */
              <p>無權檢視——這個工作區看不到這個 Skill 的版本內容（原因見上面的〈版本〉）。</p>
            )}
            {skill.derivation.forked_from_version_id && (
              <p>
                分岔自版本：<code>{skill.derivation.forked_from_version_id}</code>
              </p>
            )}
          </details>
        </div>

        {/*
          設計 §1.2. Measured 2026-09-03 in a 1280×900 window: the four things a
          reader can DO on this page sat at y568（打包）, y2482（檔案樹）,
          y2569（試跑）and y2680（Fork）on a 2845px page — Fork, the ONLY way a
          visitor moves from 探索 to 試跑, at 94% of the scroll. They are one group
          now, directly after the four verdicts.

          Moved as whole SECTIONS and not as buttons, which is the whole design of
          this change: every one of these controls carries the reason it is closed
          right beside it（打包 the redistribution gate, 試跑 the 「不在你的工作區」
          corridor, Fork the 「登入後」 line）, and hoisting the control alone would
          leave the reason three screens behind it — §2.4 and §2.10 第 5 項.

          They stay BELOW 風險揭露／可散布性／License／相容性 and that is not a
          compromise, it is §0: 安全與不誤導 is priority 1 and 速度與版面 is 4. The
          comment above those four sections records what it cost the last time the
          page was ordered the other way round. So the fix here is not 「操作進第一
          屏」, it is 「操作在一個地方」.

          2026-09-03：那一組成了右側的 `detail-rail`（r2 §5.1／B2）。**DOM 順序沒有變**
          ——rail 在主欄之後，所以閱讀順序與 Tab 順序仍然是「先證據、後操作」，≤1024px
          它就落回主欄下方，沒有任何需要復原的東西。**打包那個 `.action` 沒有跟過來**：
          §4.6.3 的表指名 `/skills/$id` 的主要動作是「打包並下載這個版本」，而它的理由
          （可散布性判定）在主欄，把按鈕拉開會弄壞 §2.4。所以 rail 裡一個 `.action`
          都沒有——一頁一個填色動作，而它在它的證據旁邊。

          三個控制項的標題降成 h3：它們在一個具名的 `<aside>` 地標裡（「這個 Skill 的
          操作」），仍然進得了標題導覽——丙-116 要的是「不是一顆沒有名字的裸按鈕」，
          那一點沒有讓步。
        */}
        <aside className="detail-rail" aria-label="這個 Skill 的操作">
          <TrialEntry skillId={skillId} isLoggedIn={!!me} />

          <section>
            {/*
              丙-116 的另一半：這個動作以前是整頁唯一沒有標題的區塊。它對非擁有者
              是**唯一**能往前的東西，卻不在 12 個 h2 裡，所以按標題導覽的人找不到
              它——而 axe 不會說話，沒有標題不是違規。
            */}
            <h3>Fork 到你的工作區</h3>
            <ForkAction skillId={skillId} isLoggedIn={!!me} />
          </section>

          {/*
            0023: with a licensing hold open, the advanced view is closed and the
            link goes with it — a link that leads to a 403 is worse than no link.
            The reason is stated where the link used to be, so the absence reads as
            a decision rather than as a missing feature.
          */}
          {skill.version && !skill.access_restriction && (
            <nav>
              <Link to="/skills/$skillId/files" params={{ skillId }}>
                查看 SKILL.md 與檔案樹（進階模式）
              </Link>
            </nav>
          )}

          <VersionUpload skillId={skillId} />
        </aside>
      </div>
    </article>
  );
}

/**
 * 02:WS-001 第 4 條「使用者可查看任兩版本的內容差異」, and the version list it needs
 * to stand on.
 *
 * Neither existed anywhere in the app. `useSkillVersions` had one call site —
 * `RunPreflight`'s 「要跑哪一版」 picker — and the detail page printed only the
 * CURRENT version's four fields, so the product's own loop ended blind: 採用改善
 * 建議 creates a new immutable version (iron rule 4, `createVersionFromSuggestions`)
 * and nothing could show what it changed. `WorkspaceSkills.tsx`'s header claimed
 * the history was 「reachable only through its detail page and the diff route」 —
 * a path nobody had laid.
 *
 * **NO NEW ROUTE**, on purpose, and it is 資訊架構 §0.1 that says so rather than
 * frugality: R2 puts a single item at `/skills/$id`, and a version of a skill is
 * that skill at a moment, not a second object with its own address. R3 would
 * then want two ways into whatever address was invented, and there is one place
 * that produces the context (this page). R1 is satisfied too — the page answers
 * 「這個 Skill 可不可信」 and a version's diff is evidence for it, the same way
 * 風險揭露 above is.
 *
 * A CHILD COMPONENT, not inlined: the page returns early on loading and on 410,
 * so a hook added to `SkillDetail` itself would change hook order between
 * renders.
 *
 * `useSkillVersions` is session-scoped, and the list of somebody else's skill is
 * empty rather than forbidden — so the two absences are worded apart the way
 * §2.9 requires, and `ReadFailure` carries 401 to 「需要登入」 without swallowing
 * any other status.
 *
 * --- 2026-09-03（r2 B3）-------------------------------------------------------
 * 逐版清單與「與上一版比較」收進 `<details>`，外面留一句**數量**：「共 N 版，最新
 * vX（日期）」。這是 §0 自己的教科書解法逐字——「**數量留在外面，段落收進去**」——
 * 而逐版列不在 §2.10 的十項裡。**「無權檢視」那一句留在折疊之外**（第 10 項：每一個
 * 未測量／不適用／無權檢視都不得靠互動才看得到），載入中與讀取失敗同理。−230px。
 * `what="版本歷史"` 一字未改：那是失敗訊息的主詞，不是標題。
 */
function VersionHistory({ skillId }: { skillId: string }) {
  const versions = useSkillVersions(skillId);
  // The pair being compared, or nothing. Not in the URL: 資訊架構 §0.1 R4 —
  // 「你在看哪一份東西」 is the skill, and which two of its versions are expanded
  // is 「你偏好怎麼看」, the same call IA-4 made for the Trace reading mode.
  const [pair, setPair] = useState<{ from: string; to: string } | null>(null);

  const list = versions.data?.versions ?? [];

  return (
    <section>
      <h2>版本</h2>
      {versions.isPending && <Loading what="版本歷史" />}
      <ReadFailure error={versions.error} what="版本歷史" />

      {versions.data &&
        (list.length === 0 ? (
          /* §2.9 again: an empty list here is 無權檢視, not 「這個 Skill 沒有
             版本」. Every skill has at least one — the one that was imported. */
          <p>
            無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
            之後才會有屬於你的版本；這不代表它沒有版本。
          </p>
        ) : (
          <>
            <p>
              共 {list.length} 版，最新 v{list[0].version_number}（
              <Timestamp at={list[0].created_at} />）
            </p>
            <details>
              <summary>每一版與它跟上一版的差異</summary>
              <ul className="search-results">
                {list.map((version, index) => {
                  // Newest first (the endpoint's order), so 「上一版」 is the NEXT
                  // element. The oldest row has none, and says why rather than
                  // rendering a control that would compare a version with itself
                  // (§2.4: a missing control owes a reason too).
                  const previous = list[index + 1];
                  const open = pair?.to === version.version_id;
                  return (
                    <li key={version.version_id} className="search-result">
                      <p>
                        {/* 設計 §3 第 15 條：這兩個相鄰元素之間沒有任何東西，於是
                            渲染成 「v2建立時間：2026/08/17」——量到 0.0px。 */}
                        <strong>v{version.version_number}</strong>{" "}
                        <span className="note">
                          建立時間：
                          <Timestamp at={version.created_at} />
                        </span>
                      </p>
                      {previous ? (
                        <p>
                          <button
                            type="button"
                            onClick={() =>
                              setPair(
                                open ? null : { from: previous.version_id, to: version.version_id },
                              )
                            }
                          >
                            {open ? "收起與上一版的比較" : "與上一版比較"}
                          </button>
                        </p>
                      ) : (
                        <p className="note">這是最早的版本，沒有上一版可以比較。</p>
                      )}
                      {open && <VersionDiff url={skillDiffUrl(skillId, pair.from, pair.to)} />}
                    </li>
                  );
                })}
              </ul>
              {/* 2026-09-03（丙-142）：這一段與 `SkillFiles` 的折疊區各講了一次同一條
                  全站規則。留下的是這一頁真的要用到的兩件事——為什麼會有第二版（不然
                  這份清單沒有理由存在），以及這個按鈕比的是什麼。「舊的一版原封不動
                  留著」與「兩版套件檔案的」是同一句話的兩次說法。 */}
              <p className="note">
                版本不可變：採用改善建議會建立新的一版。差異比對的是套件內容，不是試跑結果。
              </p>
            </details>
          </>
        ))}
    </section>
  );
}

/**
 * 02:SEC-007 / ADR-027 決策 4 — the three-state redistribution answer, and the
 * packaging entry point that depends on it.
 *
 * The entry is closed for `blocked` and for `unknown` alike, in the words
 * `PACKAGING_BLOCKED_LABEL` uses on the download page itself: one table, two
 * surfaces, so the reason a user is refused here and the reason they would be
 * refused there cannot drift apart. A licensing hold closes it too and is
 * reported as the hold rather than as a licensing verdict — they are two
 * independent locks and only one of them is temporary.
 *
 * The contract requires `redistribution` on every skill, so the badge is the
 * normal case. A response without it is a platform that failed to answer, not a
 * fourth state: the section says that plainly instead of rendering a verdict
 * nobody gave, and `packagingGate` closes the entry all the same.
 *
 * License 是這個判定的**證據**，所以 2026-09-03 起它是這一段的 h3 而不是隔壁的 h2
 * ——見檔頭。兩個徽章各自帶著伺服器的但書，一句都沒有少，而且都不在任何 `<details>`
 * 裡（§2.10 第 3 項）。
 */
function Redistribution({ skill, isLoggedIn }: { skill: SkillDetailModel; isLoggedIn: boolean }) {
  const blocked = packagingGate(skill);

  return (
    <section>
      <h2>可散布性與打包</h2>
      {skill.redistribution ? (
        <>
          <p>
            <LabelledBadge kind="redistribution" value={skill.redistribution} />
          </p>
        </>
      ) : (
        <p className="note">平台沒有回報這個 Skill 的可散布性判定。</p>
      )}

      {blocked ? (
        <>
          <p>
            {/*
              設計 §3 第 6 條: the reason is visible text, which is the hard
              half — but a keyboard reader tabbing onto this heard 「打包並下載,
              按鈕, 已停用」 and then silence. `aria-describedby` is the same fix
              a11y.test.tsx walked four other controls through on 2026-09-03;
              this one was not walked because the scanned fixture is
              `redistribution: allowed`, so the branch never renders under the
              scanner. It is also the disabled button most readers meet first:
              `license_unknown` is the default for anything they imported
              themselves.
            */}
            <button type="button" disabled aria-describedby="packaging-blocked-reason">
              打包並下載
            </button>
          </p>
          <p className="note" id="packaging-blocked-reason">
            {PACKAGING_BLOCKED_LABEL[blocked]}
          </p>
        </>
      ) : skill.version ? (
        <PackagingEntry skill={skill} isLoggedIn={isLoggedIn} />
      ) : (
        /*
          §2.9 的「無權檢視」。2026-09-03（丙-142）：同一段解釋在這一頁上出現三次
          （這裡、〈版本〉區塊、〈進階資訊〉折疊區），逐位元相同。**型別詞留在三處**
          （§2.10 第 10 項：缺席是哪一型不可折、不可省），連同這一格自己的後果——
          「沒有東西可以打包」是這個控制項不在的原因（§2.4）。搬走的只有那段對每一格
          都一樣的解釋（要 Fork 才會有屬於你的版本、這不代表它沒有版本），它現在只在
          下面的〈版本〉區塊講一次。
        */
        <p className="note">
          無權檢視——這個工作區看不到這個 Skill
          的版本內容，所以沒有東西可以打包（原因見下面的〈版本〉）。
        </p>
      )}

      <h3>License</h3>
      <LicenseBadge license={skill.license} />
      <LicenseNotes license={skill.license} />
    </section>
  );
}

/**
 * 打包入口，而它的整段歷史是**這個檔案裡兩段註解互相矛盾**。
 *
 * 上面的 `TrialEntry` 寫著、而且量過：「**`skill.version` is NOT the signal** …
 * keying off it calls every visitor an owner」——`GET /api/skills/{id}` 的
 * `version` 來自 `LatestVersion(ctx, skill.WorkspaceID, …)`（`discovery/detail.go`），
 * 也就是**那個 Skill 自己的**工作區，不是呼叫者的。可是打包這一半就是用它決定
 * 要不要畫出 CTA，而那個 `.action` 是全 app 唯一的強調樣式、2026-09-03 的重排
 * 又把它移到整頁第二個區塊。結果：一個從目錄點進來的訪客，畫面上最顯眼的動作
 * 是「打包並下載這個版本」，按下去落在 `/skills/:id/package`，那裡的 preview 是
 * workspace-scoped，回 `404 {"error":"skill version not found"}`，畫面印出
 * 「無法讀取打包預覽：skill version not found」——一句英文，給一個中文使用者，
 * 說的還不是真正的原因（真正的原因是「這還不是你的」）。
 *
 * 這就是 丙-116 的那條走廊，只是當時修了「試跑」那一半、沒修「打包」這一半。
 * 訊號用 `TrialEntry` 已經在用的那一個：`useSkillVersions` 是 session-scoped，
 * 對別人的 Skill 回空清單（ADR-011）。React Query 同 key 去重，所以這**不是**
 * 第二個請求，是同一個。
 */
function PackagingEntry({ skill, isLoggedIn }: { skill: SkillDetailModel; isLoggedIn: boolean }) {
  const versions = useSkillVersions(skill.skill_id);

  if (!isLoggedIn)
    return (
      <p className="note">
        打包與下載需要登入，而且只打包得了你自己工作區裡的版本——別人的 Skill 要先 Fork 一份。{" "}
        <SignInAction />
      </p>
    );
  if (versions.isPending) return <Loading what="這個 Skill 在你工作區的版本" />;
  if (versions.error) return <ReadFailure error={versions.error} what="這個 Skill 的版本" />;
  if ((versions.data?.versions.length ?? 0) === 0)
    return (
      <p className="note">
        這個 Skill 不在你的工作區，所以沒有屬於你的版本可以打包。
        <strong>要先 Fork 一份</strong>——旁邊的「Fork 到你的工作區」就是那一步。
      </p>
    );

  return (
    <p>
      <Link
        className="action"
        to="/skills/$skillId/package"
        params={{ skillId: skill.skill_id }}
        search={{ version: skill.version!.version_id }}
      >
        打包並下載這個版本
      </Link>
    </p>
  );
}

/**
 * 限制 (02:DISC-003 一般模式). The API assembles this list from two sources and
 * labels each entry with the one it came from: `model` is the enrichment
 * restating what the document says about its own limits, `scan` is derived from
 * the static package scan. ADR-013 requires the model half to be visibly marked,
 * so the two are never merged into one anonymous sentence.
 *
 * An empty list says so explicitly. Dropping the section would read as "this
 * skill has no limits", which is the opposite of what an empty list means here:
 * neither source stated one.
 *
 * What each marker means is visible text below the list, not only the badge's
 * `title` (system.md §3 item 4). These are the two sentences that stop a reader
 * over-trusting a badge, and on a touch device a tooltip is not there at all.
 * Stated per marker actually present, so the page never explains a label it did
 * not render.
 *
 * 2026-09-03（丙-142）：兩顆徽章的 `title` 拿掉了。它們與底下那兩句可見文字**逐字
 * 相同**，而 §2.13 去重第 2 條把「`title=` 與可見 `.note` 同字」算成同一句講了兩次，
 * §2.4 的補句逐字寫著「原因搬出 `title` 之後，`title` 要拿掉」。第一訊號（可見文字）
 * 一個字都沒有動——消失的是那份在觸控裝置上本來就不存在的複本。
 *
 * 2026-09-03：h2 → h3，因為它現在住在〈它能做什麼〉裡面（§3 第 9 條）。一個字都沒改，
 * 也沒有進 `<details>`——空清單那一句是 §2.10 第 10 項。
 */
function Limitations({ limitations }: { limitations: SkillLimitation[] }) {
  const fromModel = limitations.some((l) => l.source === "model");
  const fromScan = limitations.some((l) => l.source !== "model");

  return (
    <>
      <h3>限制</h3>
      {limitations.length === 0 ? (
        <p className="note">
          沒有任何來源指出限制——這代表沒有人說明過，不代表這個 Skill 沒有限制。
        </p>
      ) : (
        <ul className="risk-list">
          {limitations.map((limitation) => (
            <li key={`${limitation.source}-${limitation.text}`}>
              {limitation.text}
              {limitation.source === "model" ? (
                <span className="badge badge-source-model">AI 產生</span>
              ) : (
                <span className="badge badge-source-template">掃描推得</span>
              )}
            </li>
          ))}
        </ul>
      )}
      {fromModel && <p className="note">「AI 產生」的項目由模型重述套件內容，未經人工核對。</p>}
      {fromScan && (
        <p className="note">「掃描推得」的項目由匯入時的靜態掃描結果推得，掃描不執行套件內容。</p>
      )}
    </>
  );
}

const TAG_BUCKETS: Array<{ key: keyof SkillTags; label: string }> = [
  { key: "inputs", label: "輸入" },
  { key: "outputs", label: "輸出" },
  { key: "tools", label: "會用到的工具" },
  { key: "dependencies", label: "依賴" },
];

/**
 * ADR-013: index-time model output, always labelled as model-written so a
 * reader can tell it from the author's own text above.
 *
 * 2026-09-03：這一段不再自己畫標題——它與〈限制〉一起住在〈它能做什麼〉底下（§3 第 9
 * 條）。任務範例那一份 h3 ＋ `<ul>` 收進 `<details>`（r2 B4）：§2.6 的推論是「模型寫的
 * 東西不是答案」，而 §1.3 的第三種機制正是「屬於本頁某個事實的細節」。**四列 tag 的
 * 「未測量」留在折疊之外**（§2.10 第 10 項），`02:DISC-003` 要求的輸入／輸出／依賴
 * 三列也一樣。−120px。
 */
function Enrichment({ enrichment }: { enrichment: SkillEnrichment }) {
  if (enrichment.status !== "enriched") {
    return (
      <>
        <p className="note">{enrichment.note}</p>
        {/*
          The 輸入／輸出／依賴 rows stay on the page while enrichment is pending,
          reading 未知. They come from the enrichment, so a pending row has none
          — and omitting the rows entirely would present a skill with unknown
          inputs as a skill that takes none.
        */}
        {TAG_BUCKETS.map(({ key, label }) => (
          <p key={key}>
            {label}：<span className="note">未知（尚未產生索引摘要）</span>
          </p>
        ))}
      </>
    );
  }

  return (
    <>
      {/*
        The marker is beside the heading, not inside it. Inside, it joined the
        accessible name — `__outlines__/skills-skillId.txt` recorded the result
        as `h2 白話摘要AI 產生`, which is both the glued rendering §3 item 15
        forbids and a heading that names two things. A `.badge-row` is the
        surface this app already uses for a badge that qualifies the block
        below it (the header above does the same with three of them).
      */}
      {/*
        2026-09-03（丙-142）：「這段是模型寫的、沒有人核對」在這一頁曾經有四份可見複本
        （合計 47 字）加兩個 `title`。留下來的是說得最完整的那兩份，而且兩份都在這個
        `<section>`〈它能做什麼〉裡面，也就是 §2.11(c) 要的「同一個區塊」：
          - 這個區塊底下伺服器那句 `enrichment.note`（它不只說「模型寫的」，還說了
            你的 Agent 讀的不是這一段），與
          - 〈限制〉那句「『AI 產生』的項目由模型重述套件內容，未經人工核對。」——
            它同時解釋徽章的語意與涵蓋範圍，所以它是留下來的那一份。
        拿掉的是這一列自己那份 12 字的複述與同字的 `title`（§2.13 去重第 2 條）。
      */}
      <p className="badge-row">
        <span className="badge badge-source-model">AI 產生</span>
      </p>
      {enrichment.summary && <p>{enrichment.summary}</p>}

      {/*
        DISC-003 asks for 輸入、輸出、依賴 as separate facts, and the contract
        keeps them in separate buckets, so they are not flattened back into one
        anonymous tag cloud here.

        An empty bucket is stated as 未知 rather than dropped. A missing row
        reads as "this skill needs no dependencies"; what it actually means is
        that nothing extracted any, which is the 不得自行推定 case 02:DISC-004
        names. The same reasoning as the search row's 依賴 column.
      */}
      {enrichment.tags &&
        TAG_BUCKETS.map(({ key, label }) =>
          enrichment.tags![key].length > 0 ? (
            <p key={key}>
              {label}：
              <span className="tag-list">
                {enrichment.tags![key].map((tag) => (
                  <span key={tag} className="badge">
                    {tag}
                  </span>
                ))}
              </span>
            </p>
          ) : (
            <p key={key}>
              {label}：<span className="note">未測量（沒有擷取到，不代表沒有）</span>
            </p>
          ),
        )}

      {enrichment.task_examples && enrichment.task_examples.length > 0 && (
        <details>
          <summary>可以用來做什麼（AI 產生的任務範例）</summary>
          <ul>
            {enrichment.task_examples.map((example) => (
              <li key={example}>{example}</li>
            ))}
          </ul>
        </details>
      )}

      <p className="note">{enrichment.note}</p>
      {(enrichment.model || enrichment.prompt_version) && (
        <details>
          <summary>產生這段摘要的模型</summary>
          <ul>
            {enrichment.model && (
              <li>
                模型：<code>{enrichment.model}</code>
              </li>
            )}
            {enrichment.prompt_version && (
              <li>
                Prompt 版本：<code>{enrichment.prompt_version}</code>
              </li>
            )}
          </ul>
        </details>
      )}
    </>
  );
}

/**
 * DISC-003: URL, version/commit, fetch time and content hash of what arrived.
 *
 * 2026-09-03（r2 A4）：來源版本／Commit 與內容雜湊收進同一個「識別碼」`<details>`。
 * §2.6 逐字點名這一族（版本 id、內容雜湊、產生摘要的模型），而**擷取時間與可用性探測
 * 留在外面**——「來源已失效，自 … 起」是 §2.10 第 9 項的平台降級自述，而 `fetched_at`
 * 是「這份東西有多舊」那條軸的證據。以前是兩個地方（一行明文 ＋ 一個只裝雜湊的
 * `<details>`），現在是一個。
 */
function SourceBlock({ source }: { source: SkillSource }) {
  if (source.type === "generated") {
    return <GeneratedSourceBlock source={source} />;
  }
  return (
    <>
      <p>匯入方式：{source.type === "git" ? "從 Git 來源擷取" : "使用者上傳"}</p>
      {source.url && (
        <p>
          來源網址：{" "}
          <a href={source.url} rel="noreferrer noopener">
            {source.url}
          </a>
        </p>
      )}
      {source.fetched_at && (
        <p>
          擷取時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}

      {/*
        The probe's two facts stay apart: "gone since when" is the one that
        matters, and "never probed" is rendered as unknown rather than quietly
        omitted, because an absent line reads as reassurance.
      */}
      {source.unavailable_since ? (
        <p className="badge badge-risk">
          {/* 這一句與它下面的兄弟句講同一種事實，卻用兩種寫法：`unavailable_since`
              是 `format: date-time`，所以這裡印的是「2026-08-01T18:00:00Z」，隔壁
              的 `last_checked_at` 走 `<Timestamp>` 印的是讀者自己的時鐘。設計 §3
              第 14 條。`design-system.test.ts` 的守門只比對 `_at` 結尾，所以這個
              欄位是從門縫走過去的——那條 regex 一併放寬到 `_since`。 */}
          來源已失效，自 <Timestamp at={source.unavailable_since} />{" "}
          起無法取得。目前顯示的是失效前保存的內容。
        </p>
      ) : !source.last_checked_at ? (
        /* 「從來沒探測過」是 §2.9 的缺席型別詞，留在外面（§2.10 第 10 項）。 */
        <p className="note">尚未檢查過來源是否仍可取得。</p>
      ) : null}

      {(source.source_version ||
        source.content_hash ||
        (!source.unavailable_since && source.last_checked_at)) && (
        <details>
          <summary>識別碼</summary>
          <ul>
            {/*
              2026-09-03（丙-142）：「最近一次來源可用性檢查：… （當時可取得）」收進
              這裡。它是一個**通過**的探測，也就是「這份東西有多舊」那條軸上的推導細節
              （§2.6／§2.13 的 F 類），不是判斷依據——它成立時畫面上什麼都不必改變。
              **`unavailable_since` 那一句沒有跟進來**：那是 §2.10 第 9 項的平台降級
              自述，永不折疊，而且它渲染時這一列不渲染（它們互斥），所以折疊區裡不會
              出現一句與外面那句互相矛盾的「當時可取得」。
            */}
            {!source.unavailable_since && source.last_checked_at && (
              <li>
                最近一次來源可用性檢查：
                <Timestamp at={source.last_checked_at} />
                （當時可取得）
              </li>
            )}
            {source.source_version && (
              <li>
                來源版本／Commit：<code>{source.source_version}</code>
              </li>
            )}
            {source.content_hash && (
              <li>
                內容雜湊：<code>{source.content_hash}</code>
              </li>
            )}
          </ul>
        </details>
      )}
    </>
  );
}

/**
 * GEN-002's provenance block: a package with no upstream, whose entire source
 * record is the words the owner typed.
 *
 * A separate branch rather than three more optional lines in SourceBlock,
 * because almost every line there is about an upstream that does not exist
 * here — the availability probe, the commit, the URL. Rendering those as
 * unknown would describe this package as one whose origin nobody recorded, and
 * 02:GEN-002 forbids exactly that: **不得顯示為未知來源**. It is known. It is
 * not a URL.
 */
function GeneratedSourceBlock({ source }: { source: SkillSource }) {
  const diagram = source.generation_inputs?.diagram;
  const references = source.generation_inputs?.references;
  return (
    <>
      <p>
        {source.task_description
          ? "來源：由平台依你的任務描述生成"
          : diagram
            ? "來源：由平台依你上傳的流程圖生成"
            : "來源：由平台生成"}
      </p>
      {source.task_description && (
        <details>
          <summary>你當時輸入的任務描述</summary>
          <p>{source.task_description}</p>
        </details>
      )}
      {source.generation_inputs && (
        <details>
          <summary>這一次生成用到的輸入</summary>
          {diagram && (
            <p>
              流程圖：{diagram.media_type}，{diagram.bytes} bytes，sha256{" "}
              <code>{diagram.sha256}</code>
              <span className="note">（平台沒有保留圖片本身，只留下這個雜湊）</span>
            </p>
          )}
          {references && references.length > 0 && (
            <>
              <p>參考的 Skill：</p>
              <ul>
                {references.map((r) => (
                  <li key={r.version_id}>{r.name}</li>
                ))}
              </ul>
            </>
          )}
        </details>
      )}
      {source.fetched_at && (
        <p>
          生成時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}
      {source.generator_model && (
        <p>
          模型：<code>{source.generator_model}</code>
        </p>
      )}
      {source.generator_prompt_version && (
        <p>
          提示詞版本：<code>{source.generator_prompt_version}</code>
        </p>
      )}
      {source.content_hash && (
        <details>
          <summary>內容雜湊</summary>
          <code>{source.content_hash}</code>
        </details>
      )}
    </>
  );
}

/**
 * 「試跑」, which until 丙-116 rendered for anybody signed in with no ownership
 * test at all — the condition was `me &&` and nothing else.
 *
 * What that produced was a corridor of three screens, each individually right:
 * the link went to `/lab/test-cases?skill=<id>`, where the filter banner names
 * the skill from `rows[0]?.skill_name` and so fell back to the literal 「這一個
 * Skill」 because there are no rows; the empty state read as an invitation; and
 * the create form's picker is `GET /skills`, so **the skill the reader had just
 * come from was structurally absent from it**. Nothing on the way said the one
 * thing that was true: this one is not yours yet.
 *
 * The signal was already on this page and already worded correctly in three
 * other places: `GET /skills/{id}/versions` is workspace-scoped and answers an
 * empty list for somebody else's skill (ADR-011). Measured 2026-09-01 against a
 * live catalogue skill: 0 rows for a stranger, 1 for its owner.
 *
 * **`skill.version` is NOT the signal**, and it is the one a reader of this file
 * would reach for first: `GET /api/skills/{id}` carries `version` for every
 * caller, catalogue included, so keying off it calls every visitor an owner.
 *
 * A child component, not inlined, for the same reason `VersionHistory` is one:
 * the page returns early on loading and on 410, so a hook added to `SkillDetail`
 * itself would change hook order between renders. `useSkillVersions` is already
 * called by `VersionHistory`, and React Query dedupes on the key — this is the
 * same request, not a second one.
 */
function TrialEntry({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const versions = useSkillVersions(skillId);

  // Anonymous readers never reach the ownership question: the version list is
  // session-scoped, so asking it would only produce a 401 to explain. But
  // 「不問」 is not 「不說」, and this used to `return null` — so one of the
  // product's three core actions was simply absent from the page for every
  // visitor, with nothing saying why. 設計 §2.4 calls that the second failure
  // shape (the control is removed rather than disabled, and the replacement
  // text does not even say 目前不提供), and §2.10 第 5 項 puts the reason for a
  // removed control on the never-fold list. The answer is the same for every
  // visitor and costs no request, so it is stated rather than asked.
  if (!isLoggedIn)
    return (
      <section>
        <h3>試跑</h3>
        <p>
          試跑屬於你的工作區。先登入並 Fork 一份，才會有屬於你的版本可以跑。 <SignInAction />
        </p>
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
            {/*
              設計 §4.6.3（ADR-064）：這一頁在此之前有**兩個** `.action`——這一個和
              `PackagingEntry` 的「打包並下載這個版本」——而它們同時渲染（兩者的條件
              都是「這個 Skill 在你的工作區」）。兩個都強調等於都不強調。§4.6.3 的表
              指名保留的是打包那一個，所以這一條降為普通連結：文字與目的地一字未改。
            */}
            <Link to="/lab/test-cases" search={{ skill: skillId }}>
              此 Skill 的 Test Case
            </Link>
          </p>
          <p className="note" data-role="teaching">
            Test Case 是試跑用的草稿：User Prompt、測試資料與驗收條件。
          </p>
        </>
      ) : (
        // 設計 §2.2「顯示與強制成對」: said before the reader spends three
        // screens finding out, and it names both consequences rather than only
        // the first — the list AND the picker, because the picker is where the
        // corridor actually ended.
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

/** 04 丙-150：按 status 選這一頁自己的句子，從不印 `error.message`（見 ForkAction）。 */
function forkErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403)
      return "這個帳號還沒有封測邀請，所以 Fork 沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
    if (error.status === 409) return "你的工作區已經有同名的 Skill。";
  }
  return "Fork 沒有成功，可以再按一次。";
}

function ForkAction({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const fork = useForkSkill();
  /*
    設計 §4.6.3（ADR-064）的表，`/skills/$id` 那一列：主要動作是「打包並下載這個
    版本」，**不可打包時退為 Fork**。所以這顆按鈕要知道打包那個入口有沒有畫出來。

    問的是同一個問題、同一把尺：`PackagingEntry` 也是用「你的工作區裡有沒有這個
    Skill 的版本」決定要不要畫連結（`skill.version` 不是訊號——見那個元件的檔頭）。
    React Query 同 key 去重，所以這**不是**第三個請求，是 `VersionHistory` 與
    `PackagingEntry` 已經在發的那一個。

    `isSuccess` 而不是 `!isPending`：只有在清單真的回來且是空的時候才確定「不可
    打包」。載入中與讀取失敗時兩邊都不填色——一頁**零個**主要動作是合法的，而在
    還不知道答案時把填色亂放一顆，比晚半秒才出現更糟。
  */
  const versions = useSkillVersions(skillId);
  const cannotPackage = versions.isSuccess && versions.data.versions.length === 0;

  if (!isLoggedIn) {
    // `login-prompt` carried no CSS rule and no test selected it, so it was a
    // class that only looked like a hook. Removed rather than left to be read
    // as one (same call as `badge-risk-none` in RiskIndicator).
    //
    // 設計 §2.2 第三向: a sentence that blocks somebody has to carry the next
    // step, and this one blocks the visitor's ONLY way forward (see TrialEntry
    // above: Fork is what turns 探索 into 試跑). The login action was on this
    // page already — but only inside the version-history read failure, i.e. the
    // one box a reader has no reason to open. `SignInAction` is the single
    // login entry in the app on purpose (components/SignIn.tsx): on the machine
    // 02:PORT-005 describes, a hardcoded GitHub link leaves the product.
    return (
      <p>
        登入後即可 Fork 這個 Skill 到你的工作區。 <SignInAction />
      </p>
    );
  }

  return (
    <div>
      {/*
        04 丙-153：`CreateHub.tsx:76-82` 對「到目錄挑一個來改」那張卡說了同一句
        （Fork 需要封測邀請，由平台強制），但這一頁的 Fork 按鈕在此之前沒有這句
        話——按下去才知道要有邀請。§2.2 第三向：擋住人的限制要在撞上之前說。
      */}
      <p className="note">
        Fork 需要封測邀請，這道限制由平台強制；還沒有邀請的話，那一步會被擋下來。
      </p>
      {/*
        r4 B2：按鈕上的字從「Fork 這個 Skill」改成「以這個 Skill 為起點建立我自己的」。
        動詞沒有變、端點沒有變、繼承的東西沒有變（`redistribution` 與
        `access_restriction` 仍然逐字繼承）——變的是這顆按鈕現在說得出它產生什麼。
        「Fork」對一個沒有用過 GitHub 的個人創作者不是一個動作，是一個名詞；而這一顆
        對非擁有者是整頁**唯一**能往前的東西。標題保留「Fork 到你的工作區」，因為同頁
        另外兩段（試跑、打包）用那個名字指路。
      */}
      <button
        type="button"
        className={cannotPackage ? "action" : undefined}
        onClick={() => fork.mutate(skillId)}
        disabled={fork.isPending}
      >
        {fork.isPending ? "建立中…" : "以這個 Skill 為起點建立我自己的"}
      </button>
      {/*
        04 丙-150：「Fork 沒有成功：{err.message}」 was one sentence for every
        refusal, printing whatever `ApiError.message` held — the server's own
        wording for 409, but `betaNotInvited`'s English paragraph for 403.
        `POST /skills/{id}/fork` refuses four ways: 401 (→ `ReadFailure`'s
        default), 403 (封測邀請名單 — ADR-028 決策 1 names fork, run and download
        as the three actions the gate covers), 404, and 409 (a skill with this
        name already exists in your workspace). Each non-401 status gets this
        page's own Chinese sentence — never `fork.error.message` — so a 403
        never surfaces `betaNotInvited`'s English text.
      */}
      {fork.isError && (
        <ReadFailure error={fork.error} what="Fork 這個 Skill">
          <p role="alert">{forkErrorMessage(fork.error)}</p>
        </ReadFailure>
      )}
      {fork.isSuccess && (
        <p>
          已建立 Fork：
          <Link to="/skills/$skillId" params={{ skillId: fork.data.skill_id }}>
            {fork.data.name}
          </Link>
        </p>
      )}
    </div>
  );
}

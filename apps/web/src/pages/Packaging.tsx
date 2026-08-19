import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import {
  createDownloadArtifact,
  downloadHref,
  usePackagingPreview,
  usePackagingTargets,
  type CreatedDownloadArtifact,
  type PackageValidation,
  type PackagingBlockedReason,
  type PackagingPreview,
  type PackagingTarget,
  type PackagingTargetId,
} from "../api/packaging";
import { useSkillDetail } from "../api/skills";
import { SkillVersionPicker } from "./RunPreflight";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { DownloadArtifactFacts } from "../components/DownloadArtifactFacts";
import type { Finding, SkillDetail } from "../api/types";

/**
 * 02:PACK-001 / PACK-002 — pick a target, see what packaging would produce,
 * build it, take the bytes.
 *
 * The rules this screen exists to keep:
 *
 * 1. **The preview and the build answer from the same server-side plan.** The
 *    build button is offered only when the preview said `allowed`, and a build
 *    that is refused anyway re-reads the preview instead of arguing with it — a
 *    preview that says yes and a packaging that refuses must not both stand.
 * 2. **A refusal always names which of the four locks closed** (PACK-001 的
 *    「被阻擋時必須回可讀的理由，且理由要分得出是哪一道鎖」). Whether the user can do
 *    anything about it depends entirely on that.
 * 3. **`unverified` is stated, never softened.** Passing format validation is
 *    not permission to say a package installs (ADR-012, 02:PACK-002 第 2 條).
 * 4. **A dataset the user uploaded never travels and there is no checkbox for
 *    it** (02:PACK-001 第二層). The excluded list says so rather than the package
 *    quietly being smaller than expected.
 * 5. **The three compatibility axes are shown here too, and kept apart**
 *    (02:PACK-002 via DESIGN-012). They belong to the Skill version being
 *    packaged, not to the target: passing spec validation is not a statement
 *    about whether the agent picked it up, and neither is a statement about
 *    whether its scripts ran. Packaging changes none of the three, which is
 *    exactly why they must not be re-stated in the packager's own words.
 */

type PackagingSearch = { version?: string };

/**
 * One sentence per blocked reason, saying what it means for the reader — the
 * server's own `blocked_message` is displayed beside it and says what the
 * platform decided. Exported because the entry point on the skill page refuses
 * with the same words: two copies of this table would drift, and the drift would
 * be in the direction of whichever surface was edited last.
 */
export const PACKAGING_BLOCKED_LABEL: Record<PackagingBlockedReason, string> = {
  license_hold:
    "這個 Skill 正在授權審查中（人工暫時保留）。審查期間平台不產出任何套件，標準套件也不例外。",
  not_redistributable:
    "這個 Skill 的授權不允許再散布，平台不會把它交出去。授權已人工確認，不等於可以再散布。",
  license_unknown:
    "沒有人確認過這個 Skill 可不可以再散布。授權未知一律當成不可散布處理——這不是等待中的暫時狀態，是預設就擋。",
  validation_blocked:
    "用這些設定打出來的套件，過不了平台自己匯入時要過的驗證，因此不能標示為有效套件。下面的錯誤清單就是要修的東西。",
};

/**
 * The two independent locks (ADR-027 決策 4) as the skill detail reports them, so
 * the entry point and this page refuse for the same reason. Returns null when
 * neither is closed — which is not a promise that packaging succeeds: the server
 * checks again, and `validation_blocked` is not knowable from here at all.
 *
 * Everything that is not exactly `allowed` closes it, a field that did not
 * arrive included. The contract requires `redistribution` on every skill, so an
 * absent one is a platform that failed to answer and not a permission — and of
 * the two ways to be wrong, showing a refusal for content that turns out to be
 * fine is the recoverable one.
 */
export function packagingGate(skill: SkillDetail): PackagingBlockedReason | null {
  if (skill.access_restriction) return "license_hold";
  switch (skill.redistribution?.value) {
    case "allowed":
      return null;
    case "blocked":
      return "not_redistributable";
    default:
      return "license_unknown";
  }
}

export function Packaging() {
  const { skillId } = useParams({ from: "/skills/$skillId/package" });
  const { version } = useSearch({ strict: false }) as PackagingSearch;
  const client = useQueryClient();
  const skill = useSkillDetail(skillId);
  const targets = usePackagingTargets();

  const [chosen, setChosen] = useState<PackagingTargetId | "">("");
  const [includeTestCases, setIncludeTestCases] = useState(false);
  const [built, setBuilt] = useState<CreatedDownloadArtifact | null>(null);
  const [message, setMessage] = useState("");
  const [picked, setPicked] = useState("");
  useEffect(() => {
    setPicked("");
    setBuilt(null);
    setMessage("");
  }, [skillId, version]);

  // The version being packaged: the one the reader picked, else the one in the
  // URL, else the skill's latest. Never invented — with none of the three, the
  // page says so instead of guessing.
  const versionId = picked || version || skill.data?.version?.version_id || "";
  const target = chosen || (targets.data?.targets[0]?.id ?? "");
  const preview = usePackagingPreview(skillId, versionId, target, includeTestCases);

  const build = useMutation({
    mutationFn: () =>
      createDownloadArtifact(skillId, versionId, target as PackagingTargetId, includeTestCases),
    onSuccess: async (artifact) => {
      setBuilt(artifact);
      setMessage("");
      await client.invalidateQueries({ queryKey: ["downloads"] });
    },
    onError: async (err) => {
      // The server re-checks the four gates and it is the one that decides. Read
      // the preview again so the page stops offering what was just refused.
      setMessage(err instanceof Error ? err.message : "打包失敗。");
      await preview.refetch();
    },
  });

  if (skill.isLoading) return <p>載入中…</p>;
  if (skill.error || !skill.data) return <p role="alert">找不到這個 Skill，或載入失敗。</p>;

  const gate = packagingGate(skill.data);

  return (
    <section className="page">
      <h1>打包與下載</h1>
      <p>
        <Link to="/skills/$skillId" params={{ skillId }}>
          {skill.data.name}
        </Link>
        {skill.data.version && versionId === skill.data.version.version_id
          ? `（v${skill.data.version.version_number}，最新版本）`
          : ""}
      </p>
      {versionId === "" ? (
        <p role="alert">這個 Skill 還沒有已保存的版本內容，沒有東西可以打包。</p>
      ) : (
        <>
          <SkillVersionPicker
            skillId={skillId}
            value={versionId}
            onPick={(id) => {
              setPicked(id);
              // The built artifact belongs to the version it was built from;
              // leaving it on screen beside another version's preview would
              // offer bytes nobody asked for.
              setBuilt(null);
              setMessage("");
            }}
          />
          <p className="note">
            打包的是這一個不可變版本 <code>{versionId}</code>
            ；打包不會建立也不會修改任何版本，每按一次得到的是一筆 Download Artifact。
          </p>

          {gate && <BlockedNotice reason={gate} />}

          <h2>這個版本的相容性</h2>
          <CompatibilityStatus compatibility={skill.data.compatibility} />
          <p className="note">
            三軸分開看：規格驗證是對套件的靜態分析，能力相容與實測相容是沙箱裡量到的。
            打包不會改變其中任何一項，也不會把其中一項推論成另一項——
            <strong>「規格驗證通過」不等於「裝得起來」，更不等於「腳本跑得動」</strong>。
          </p>

          <h2>打包目標</h2>
          {targets.isPending && <p>載入打包目標中…</p>}
          {targets.error && <p role="alert">無法讀取打包目標：{targets.error.message}</p>}
          {targets.data && (
            <ul className="packaging-targets">
              {targets.data.targets.map((t) => (
                <TargetOption
                  key={t.id}
                  target={t}
                  selected={t.id === target}
                  onSelect={() => setChosen(t.id)}
                />
              ))}
            </ul>
          )}

          <h2>要不要一起帶走 Test Case</h2>
          <p>
            <label>
              <input
                type="checkbox"
                checked={includeTestCases}
                onChange={(e) => setIncludeTestCases(e.target.checked)}
              />{" "}
              包含可散布的 Test Case 與範例資料
            </label>
          </p>
          <p className="note">
            只有平台策展產生的範例資料會進包。
            <strong>你自己上傳的 Dataset 一律不會進包，也刻意不提供這個選項</strong>
            ——那些檔案的授權判斷不該丟給拿不到判斷材料的人。評估報告、改善建議、Trace 與 Run
            產出同樣不進包：它們是 Run 資料，不是 Skill 內容。
          </p>

          <h2>打包預覽</h2>
          {preview.isPending && target !== "" && <p>計算這些設定會產生什麼…</p>}
          {preview.error && <p role="alert">無法讀取打包預覽：{preview.error.message}</p>}
          {preview.data && <PreviewReport preview={preview.data} />}

          {message && <p role="alert">{message}</p>}

          <p>
            <button
              type="button"
              disabled={!preview.data?.allowed || build.isPending}
              onClick={() => build.mutate()}
            >
              {build.isPending ? "打包中…" : "建立下載套件"}
            </button>
          </p>

          {built && (
            <div className="notice">
              <p>
                {built.duplicate
                  ? "已有相同套件：同一個版本、同一個目標、同一個 Test Case 選項先前就打過，這就是那一份，不是第二份。"
                  : "套件已建立。"}
              </p>
              <DownloadArtifactFacts artifact={built} />
              <p>
                <a href={downloadHref(built.artifact_id)}>下載 {built.file_name}</a>
                {" ｜ "}
                <Link to="/workspace/downloads">到下載紀錄</Link>
              </p>
            </div>
          )}
        </>
      )}
    </section>
  );
}

/**
 * A refusal in two voices, deliberately: the platform's own sentence (served, so
 * the preview and the 422 cannot disagree) and what it means for the reader.
 */
export function BlockedNotice({
  reason,
  message,
}: {
  reason: PackagingBlockedReason;
  message?: string;
}) {
  return (
    <div className="notice" role="status">
      <p>
        <strong>不能打包</strong>：{PACKAGING_BLOCKED_LABEL[reason]}
      </p>
      {message && <p className="note">平台的說法：{message}</p>}
      <p className="note">
        原因代碼 <code>{reason}</code>。
      </p>
    </div>
  );
}

function TargetOption({
  target,
  selected,
  onSelect,
}: {
  target: PackagingTarget;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <li className="packaging-target">
      <label>
        <input type="radio" name="packaging-target" checked={selected} onChange={onSelect} />{" "}
        <strong>{target.display_name}</strong>{" "}
        <span className="badge">
          {target.kind === "standard_package" ? "標準套件" : "安裝 Profile"}
        </span>{" "}
        {/* ADR-012: 未驗證要寫未驗證。格式驗證通過不得暗示裝得起來。 */}
        <span className={`badge badge-${target.support_status}`}>
          {target.support_status === "verified" ? "已驗證" : "未驗證"}
        </span>
      </label>
      <p className="note">
        {target.support_status === "verified"
          ? "Skill Hub 實際把套件裝進這個目標跑過。"
          : "Skill Hub 沒有把套件裝進這個目標跑過，這裡不保證它裝得起來或能正常運行。"}
        {` 設定版本 ${target.version}。`}
      </p>
      <p className="note">
        安裝位置：
        {target.install_location ??
          "不指定——這個目標不指名任何 Agent，也就不假裝知道你的 Agent 把 Skill 放哪。"}
      </p>
      <EnvVars target={target} />
      {target.notes.length > 0 && (
        <details>
          <summary>已知限制與安裝時要注意的事（{target.notes.length}）</summary>
          <ul className="note">
            {target.notes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        </details>
      )}
      <Verification target={target} />
    </li>
  );
}

/**
 * 02:PACK-002 第 1 條「環境變數需求」, on the page where the target is chosen and
 * not only inside the package's INSTALL.md — the same reason the verification
 * steps are here. A property of the target and never of the Skill: what the SDK
 * needs is what it needs, whichever Skill is inside.
 *
 * The contract requires the field, so an empty list is the target stating it
 * needs none — a fact, printed as one, rather than a row silently missing.
 */
function EnvVars({ target }: { target: PackagingTarget }) {
  if (target.env_vars.length === 0) {
    return <p className="note">環境變數需求：這個目標不需要任何環境變數。</p>;
  }
  return (
    <details>
      <summary>環境變數需求（{target.env_vars.length}）</summary>
      <ul className="note">
        {target.env_vars.map((v) => (
          <li key={v.name}>
            <code>{v.name}</code> {v.required ? "（必要）" : "（選用）"} {v.description}
            {v.example ? (
              <>
                {" "}
                範例值：<code>{v.example}</code>
              </>
            ) : (
              ""
            )}
          </li>
        ))}
      </ul>
      <p className="note">
        這些值要由你自己在你的環境裡設定。Skill Hub 產生的套件裡不會有任何金鑰。
      </p>
    </details>
  );
}

/**
 * 02:PACK-002 第 3 條 — 至少一個安裝後的驗證 Prompt 或檢查步驟，在這個頁面上。
 *
 * 這些步驟同樣會隨套件內的 INSTALL.md 一起下載，但那要先打包、先下載才讀得到；
 * 一個還在挑目標的人需要的是現在就知道等一下要怎麼確認它真的裝好了。兩邊是同一份
 * 伺服器提供的文字，不是這裡另外寫一份。
 *
 * 每個目標至少有兩者其一（由 Profile 的 schema 保證），所以「兩個都沒有」是設定
 * 壞掉，會照實說，不會靜靜地少一段。
 */
function Verification({ target }: { target: PackagingTarget }) {
  const steps = target.verification_steps ?? [];
  const count = steps.length + (target.verification_prompt ? 1 : 0);
  if (count === 0) {
    return <p className="note">這個目標沒有提供安裝後的驗證方式。</p>;
  }
  return (
    <details>
      <summary>裝好之後怎麼確認（{count}）</summary>
      {target.verification_prompt && (
        <p className="note">
          對你的 Agent 下這個 Prompt：<q>{target.verification_prompt}</q>
        </p>
      )}
      {steps.length > 0 && (
        <ol className="note">
          {steps.map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ol>
      )}
      <p className="note">同一份說明也隨套件內的 INSTALL.md 一起下載。</p>
    </details>
  );
}

function PreviewReport({ preview }: { preview: PackagingPreview }) {
  return (
    <>
      {preview.allowed ? (
        <p>這些設定可以打包。以下是打包前重新跑一次規格驗證的結果。</p>
      ) : preview.blocked_reason ? (
        <BlockedNotice reason={preview.blocked_reason} message={preview.blocked_message} />
      ) : (
        <p role="alert">伺服器說不能打包，但沒有給原因代碼。</p>
      )}

      <Findings validation={preview.validation} />
      <Dependencies preview={preview} />

      <h3>會一起打包的 Test Case</h3>
      {preview.included_test_cases.length === 0 ? (
        <p className="note">
          沒有 Test Case 會進包。這不代表這個 Skill 沒有 Test
          Case——下面那份清單說明哪些被排除、為什麼。
        </p>
      ) : (
        <ul className="risk-list">
          {preview.included_test_cases.map((tc) => (
            <li key={tc.test_case_id}>
              {tc.name} <code>test-cases/{tc.slug}/</code>
            </li>
          ))}
        </ul>
      )}

      <h3>不會進包的 Test Case</h3>
      {preview.excluded_test_cases.length === 0 ? (
        <p className="note">沒有被排除的項目。</p>
      ) : (
        <ul className="risk-list">
          {preview.excluded_test_cases.map((tc) => (
            <li key={tc.test_case_id}>
              {tc.name}
              <span className="note">：{tc.reason}</span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

/**
 * 02:PACK-002 第 1 條「依賴需求」. The lines come from the server, which takes them
 * from the same `skillpkg` findings it assembles INSTALL.md from — this page does
 * not derive its own list, because two derivations of one fact drift and the
 * drift would be between the page and the document inside the package.
 *
 * An empty list is two different answers, so it is never printed as one: with a
 * gate closed no bytes were read and there is nothing to have dependencies, while
 * an allowed preview with none is a package that really declares and imports
 * nothing.
 */
function Dependencies({ preview }: { preview: PackagingPreview }) {
  return (
    <>
      <h3>依賴需求</h3>
      {preview.dependencies.length === 0 ? (
        <p className="note">
          {preview.allowed
            ? "這個套件沒有宣告依賴檔，程式碼裡也沒有掃到第三方 import。這是靜態掃描的結果，不是作者的保證——掃描不執行套件裡的任何東西。"
            : "還沒有讀到套件內容（上面那道鎖先擋下了），所以這裡不是「沒有依賴」，是還沒有東西可以看。"}
        </p>
      ) : (
        <>
          <ul className="risk-list">
            {preview.dependencies.map((d) => (
              <li key={d}>{d}</li>
            ))}
          </ul>
          <p className="note">
            同一份清單會寫進套件內的 INSTALL.md。Skill Hub 不會替你安裝這些，
            打包與掃描階段也不執行套件內的任何程式碼——你的環境有沒有這些依賴，要你自己確認。
          </p>
        </>
      )}
    </>
  );
}

/**
 * 02:SKILL-002's three severities, kept apart. Warnings never block and are
 * still shown: hiding them would be the same mistake as hiding an import's.
 */
function Findings({ validation }: { validation: PackageValidation }) {
  const groups: Array<{ key: string; label: string; items: Finding[] }> = [
    { key: "errors", label: "阻擋級錯誤", items: validation.errors },
    { key: "warnings", label: "警告（不阻擋，但要知道）", items: validation.warnings },
    { key: "infos", label: "資訊", items: validation.infos },
  ];
  const total = groups.reduce((n, g) => n + g.items.length, 0);

  return (
    <>
      <h3>打包後的規格驗證</h3>
      {total === 0 ? (
        <p className="note">這次重驗沒有產生任何發現。</p>
      ) : (
        groups
          .filter((g) => g.items.length > 0)
          .map((g) => (
            <div key={g.key}>
              <p>
                {g.label}（{g.items.length}）
              </p>
              <ul className="risk-list">
                {g.items.map((f, i) => (
                  <li key={`${f.code}-${i}`}>
                    <span className="risk-code">{f.code}</span> {f.message}
                    {f.path && (
                      <>
                        {" "}
                        <code>{f.path}</code>
                      </>
                    )}
                    {f.details && f.details.length > 0 && (
                      <ul className="note">
                        {f.details.map((d) => (
                          <li key={d}>{d}</li>
                        ))}
                      </ul>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))
      )}
    </>
  );
}

import { ApiError } from "../api/client";
import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
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
import { useEmbeddedSkillDetail } from "../api/skills";
import { SkillVersionPicker } from "./RunPreflight";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { LabelledBadge } from "../components/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../components/LicenseBadge";
import { RiskIndicator } from "../components/RiskIndicator";
import { DownloadArtifactFacts } from "../components/DownloadArtifactFacts";
import type {
  Finding,
  FindingSeverity,
  Redistribution,
  SeverityCounts,
  SkillCompatibility,
  SkillDetail,
  SkillRisk,
} from "../api/types";

type PackagingSearch = { version?: string };

export const PACKAGING_BLOCKED_LABEL: Record<PackagingBlockedReason, string> = {
  license_hold:
    "這個 Skill 正在授權審查中（人工暫時保留）。審查期間平台不產出任何套件，標準套件也不例外。",
  not_redistributable:
    "這個 Skill 的授權不允許再散布，平台不會把它交出去。授權已人工確認，不等於可以再散布。沒有讓你自己解除這道鎖的路徑——它擋的是授權本身說的話。",
  license_unknown:
    "沒有人確認過這個 Skill 可不可以再散布。授權未知一律當成不可散布處理——這不是等待中的暫時狀態，是預設就擋。目前沒有讓你自己解除它的路徑：放行需要具名的授權來源證據，只有平台管理者改得動（ADR-057）。",
  validation_blocked:
    "用這些設定打出來的套件，過不了平台自己匯入時要過的驗證，因此不能標示為有效套件。下面的錯誤清單就是要修的東西。",
  file_removed_by_packager:
    "SKILL.md 指向的檔案被打包器排除了，所以這一份下載回去會缺少它自己說明要用的東西——平台不交出一個自己弄殘的套件。下面「平台的說法」會指名是哪個檔；把它移出被排除的目錄、或用實體檔案取代連結，就可以再打包一次。",
};

export const REDISTRIBUTION_GATE: Record<Redistribution, PackagingBlockedReason | null> = {
  allowed: null,
  self_supplied: null,
  generated: null,
  blocked: "not_redistributable",
  unknown: "license_unknown",
};

export function packagingGate(skill: SkillDetail): PackagingBlockedReason | null {
  if (skill.access_restriction) return "license_hold";
  const value = skill.redistribution?.value;
  if (value === undefined || !Object.prototype.hasOwnProperty.call(REDISTRIBUTION_GATE, value)) {
    return "license_unknown";
  }
  return REDISTRIBUTION_GATE[value as Redistribution];
}

const DEAD_REASON_ID = "packaging-build-disabled-reason";

const SEVERITY_LABEL: Record<FindingSeverity, string> = {
  error: "錯誤",
  warning: "警告",
  info: "提示",
};

function highestSeverity(counts: SeverityCounts): FindingSeverity | null {
  if (counts.errors > 0) return "error";
  if (counts.warnings > 0) return "warning";
  if (counts.infos > 0) return "info";
  return null;
}

function RiskVerdict({ risk }: { risk: SkillRisk }) {
  if (risk.scan_status === "unavailable") {
    return <p className="badge badge-risk">風險掃描結果未知：無法讀取已保存的套件內容。</p>;
  }
  const total = risk.counts.errors + risk.counts.warnings + risk.counts.infos;
  const highest = highestSeverity(risk.counts);
  return (
    <p className="risk-counts">
      {highest
        ? `有 ${total} 項風險，最高為${SEVERITY_LABEL[highest]}。`
        : "靜態掃描未發現錯誤、警告或提示。"}
    </p>
  );
}

function CompatibilityVerdict({ compatibility }: { compatibility: SkillCompatibility }) {
  const axes: Array<[string, string]> = [
    ["規格驗證", compatibility.spec_validation.label],
    ["能力相容", compatibility.capability.label],
    ["執行環境相容", compatibility.runtime.label],
  ];
  return (
    <p className="compat-list">{axes.map(([label, value]) => `${label}：${value}`).join("／")}</p>
  );
}

function buildButtonReason({
  pending,
  target,
  targetsPending,
  preview,
}: {
  pending: boolean;
  target: string;
  targetsPending: boolean;
  preview: { isPending: boolean; error: unknown; data: PackagingPreview | undefined };
}): string {
  if (pending) return "";
  if (target === "") {
    return targetsPending
      ? "還在讀打包目標清單。讀到之前沒有目標可以送出，所以按鈕還不能按。"
      : "讀不到任何打包目標，所以沒有設定可以送出——上面那行錯誤就是原因。這不是「這個版本不能打包」，是平台這一刻答不出來。";
  }
  if (preview.error) {
    return "打包預覽讀不到，因此無法確認這些設定能不能打包。沒有確認過就不會打包。";
  }
  if (preview.isPending) return "打包預覽還在計算，算完才知道這些設定能不能打包。";
  if (!preview.data) return "還沒有打包預覽可以依據，所以按鈕還不能按。";
  return "";
}

export function Packaging() {
  const { skillId } = useParams({ from: "/skills/$skillId/package" });
  const { version } = useSearch({ strict: false }) as PackagingSearch;
  const client = useQueryClient();
  const skill = useEmbeddedSkillDetail(skillId);
  const targets = usePackagingTargets();

  const [chosen, setChosen] = useState<PackagingTargetId | "">("");
  const [includeTestCases, setIncludeTestCases] = useState(false);
  const [built, setBuilt] = useState<CreatedDownloadArtifact | null>(null);
  const navigate = useNavigate();
  useEffect(() => {
    setBuilt(null);
  }, [skillId, version]);

  const versionId = version || skill.data?.version?.version_id || "";
  const target = chosen || (targets.data?.targets[0]?.id ?? "");
  const preview = usePackagingPreview(skillId, versionId, target, includeTestCases);

  const build = useMutation({
    mutationFn: () =>
      createDownloadArtifact(skillId, versionId, target as PackagingTargetId, includeTestCases),
    onSuccess: async (artifact) => {
      setBuilt(artifact);
      await client.invalidateQueries({ queryKey: ["downloads"] });
    },
    onError: async () => {
      await preview.refetch();
    },
  });

  if (skill.isLoading) return <Loading what="這個 Skill" />;
  if (skill.error instanceof ApiError && skill.error.status === 410)
    return <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>;
  if (skill.error) return <ReadFailure error={skill.error} what="這個 Skill" />;
  if (!skill.data) return <p role="alert">找不到這個 Skill。</p>;

  const gate = packagingGate(skill.data);
  const deadReason = buildButtonReason({
    pending: build.isPending,
    target,
    targetsPending: targets.isPending,
    preview,
  });

  return (
    <section>
      <h1>打包與下載</h1>
      <p>
        <Link to="/skills/$skillId" params={{ skillId }}>
          {skill.data.name}
        </Link>
        {skill.data.version && versionId === skill.data.version.version_id
          ? `（v${skill.data.version.version_number}，最新版本）`
          : ""}
      </p>
      <p className="badge-row">
        <LabelledBadge kind="redistribution" value={skill.data.redistribution} />
        <LicenseBadge license={skill.data.license} />
      </p>
      <RiskVerdict risk={skill.data.risk} />
      <details>
        <summary>風險與 License 的逐項細節（與詳情頁同一次掃描結果）</summary>
        <LicenseNotes license={skill.data.license} />
        <RiskIndicator risk={skill.data.risk} />
      </details>
      {versionId === "" ? (
        <p role="alert">
          無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
          之後才會有屬於你的版本；這不代表它沒有版本。沒有版本內容就沒有東西可以打包。
        </p>
      ) : (
        <>
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

          {gate && <BlockedNotice reason={gate} />}

          <h2>這個版本的相容性</h2>
          <CompatibilityVerdict compatibility={skill.data.compatibility} />
          <details>
            <summary>相容性細項（每一軸的備註與實測環境）</summary>
            <CompatibilityStatus compatibility={skill.data.compatibility} />
          </details>
          <p className="note">
            <strong>「規格驗證通過」不等於「裝得起來」，更不等於「腳本跑得動」</strong>。
          </p>

          <h2>打包目標</h2>
          <p className="note" data-role="teaching">
            每個目標的安裝說明也隨套件內的 INSTALL.md 一起下載。
          </p>
          {targets.isPending && <Loading what="打包目標" />}
          <ReadFailure error={targets.error} what="打包目標" />
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
          <ReadFailure error={preview.error} what="打包預覽">
            {preview.error instanceof ApiError && preview.error.status === 404 ? (
              <p role="alert">這個版本讀不到，可能已經不屬於這個 Skill。回上一步重新挑一次版本。</p>
            ) : preview.error instanceof ApiError && preview.error.status === 503 ? (
              <p role="alert">這個部署沒有設定任何打包目標，所以沒有預覽。</p>
            ) : (
              <p role="alert">
                無法讀取打包預覽：
                {preview.error instanceof Error ? preview.error.message : String(preview.error)}
              </p>
            )}
          </ReadFailure>
          {preview.data && <PreviewReport preview={preview.data} />}

          {preview.data?.allowed && <RetentionNotice preview={preview.data} />}

          <p className="note">平台目前只讓有封測邀請的帳號建立下載套件。</p>
          <ReadFailure error={build.error} what="套件建立">
            {build.error instanceof ApiError && build.error.status === 403 ? (
              <p role="alert">
                這個帳號還沒有封測邀請，所以套件沒有建立。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。
              </p>
            ) : (
              <p role="alert">套件沒有建立成功，可以再按一次。</p>
            )}
          </ReadFailure>

          <p>
            <button
              type="button"
              className="action"
              disabled={!preview.data?.allowed || build.isPending}
              onClick={() => build.mutate()}
              aria-describedby={deadReason ? DEAD_REASON_ID : undefined}
            >
              {build.isPending ? "打包中…" : "建立下載套件"}
            </button>
          </p>
          {deadReason && (
            <p className="note" id={DEAD_REASON_ID}>
              {deadReason}
            </p>
          )}

          {built && (
            <div>
              <p role="status">
                {built.duplicate
                  ? "已有相同套件：同一個版本、同一個目標、同一個 Test Case 選項先前就打過，這就是那一份，不是第二份。"
                  : "套件已建立。"}
              </p>
              <DownloadArtifactFacts artifact={built} />
              <p className="note">
                上面折起來的那兩串是雜湊，不是簽章。
                <strong>MVP 的套件不帶數位簽章，平台也不驗簽</strong>
                （ADR-027 決策 3 是明文的「不做」）——它們證明得了「位元組沒有被改過」，
                證明不了「這份東西是誰做的」。
              </p>
              <p>
                <a
                  href={downloadHref(built.artifact_id)}
                  onClick={() => void client.invalidateQueries({ queryKey: ["downloads"] })}
                >
                  下載 {built.file_name}
                </a>
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

function RetentionNotice({ preview }: { preview: PackagingPreview }) {
  const days = preview.retention_days;
  if (typeof days !== "number" || !Number.isFinite(days) || days < 0) {
    return (
      <p className="note" role="status">
        這個部署沒有回答打包產物會保留多久，所以這裡不寫數字——
        寫一個沒有人裁定過的期限，比不寫更糟。
      </p>
    );
  }
  return (
    <p className="note" role="status">
      <strong>保留期限</strong>：打包完成後，這份下載套件會保留{" "}
      <strong>{days >= 1 ? `${days} 天` : "不到 1 天"}</strong>，到期後平台自動刪除它。
      <span data-role="teaching">
        <strong>過期不等於做白工</strong>——打包是冪等的，再打一次得到的是同一份內容。
      </span>
    </p>
  );
}

export function BlockedNotice({
  reason,
  message,
}: {
  reason: PackagingBlockedReason;
  message?: string;
}) {
  return (
    <div className="notice notice-danger" role="status">
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
    </details>
  );
}

function PreviewReport({ preview }: { preview: PackagingPreview }) {
  return (
    <>
      {preview.allowed ? (
        <p>這些設定可以打包。</p>
      ) : preview.blocked_reason ? (
        <BlockedNotice reason={preview.blocked_reason} message={preview.blocked_message} />
      ) : (
        <p role="alert">伺服器說不能打包，但沒有給原因代碼。</p>
      )}

      <Findings validation={preview.validation} />
      <Dependencies preview={preview} />

      <h3>會一起打包的 Test Case</h3>
      {preview.included_test_cases.length === 0 ? (
        <p className="note">沒有 Test Case 會進包。這不代表這個 Skill 沒有 Test Case。</p>
      ) : (
        <ul className="risk-list">
          {preview.included_test_cases.map((tc) => (
            <li key={tc.test_case_id}>
              {tc.name} <code>test-cases/{tc.slug}/</code>
            </li>
          ))}
        </ul>
      )}

      <h3>打包器拿掉的檔案</h3>
      {preview.excluded_files.length === 0 ? (
        <p className="note">沒有檔案被排除，這一份帶走的就是版本裡的全部內容。</p>
      ) : (
        <ul className="risk-list">
          {preview.excluded_files.map((f) => (
            <li key={f.path}>
              <code>{f.path}</code> {f.label}
              <span className="note">：{f.note}</span>
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
              {tc.name} {tc.label}
              <span className="note">：{tc.note}</span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function Dependencies({ preview }: { preview: PackagingPreview }) {
  // server may send null for an empty list (Go nil slice), not []
  const dependencies = preview.dependencies ?? [];
  return (
    <>
      <h3>依賴需求</h3>
      {dependencies.length === 0 ? (
        <p className="note">
          {preview.allowed
            ? "這個套件沒有宣告依賴檔，程式碼裡也沒有掃到第三方 import。這是靜態掃描的結果，不是作者的保證——掃描不執行套件裡的任何東西。"
            : "還沒有讀到套件內容（上面那道鎖先擋下了），所以這裡不是「沒有依賴」，是還沒有東西可以看。"}
        </p>
      ) : (
        <>
          <ul className="risk-list">
            {dependencies.map((d) => (
              <li key={d}>{d}</li>
            ))}
          </ul>
          <p className="note">
            Skill Hub 不會替你安裝這些，
            打包與掃描階段也不執行套件內的任何程式碼——你的環境有沒有這些依賴，要你自己確認。
          </p>
        </>
      )}
    </>
  );
}

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
      <p className="risk-counts">
        阻擋級錯誤 {validation.errors.length} 項／警告 {validation.warnings.length} 項／資訊{" "}
        {validation.infos.length} 項
      </p>
      {total === 0 ? (
        <p className="note">
          這次重驗沒有產生任何發現。這是「掃過了，沒掃到」，不是「沒掃」——它讀套件內容、
          不執行其中的 Script，既不是人工審查，也不是簽章驗證。簽章這一項不是還沒驗，
          是這裡永遠不會有人替你驗。
        </p>
      ) : (
        groups
          .filter((g) => g.items.length > 0)
          .map((g) => (
            <div key={g.key}>
              <h4>
                {g.label}（{g.items.length}）
              </h4>
              <ul className="risk-list">
                {g.items.map((f, i) => (
                  <li key={`${f.code}-${i}`}>
                    {f.message}（<span className="risk-code">{f.code}</span>）
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

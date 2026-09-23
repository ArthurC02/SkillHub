import { ApiError } from "../../../core/api/client";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import {
  downloadHref,
  useCreateDownload,
  usePackagingPreview,
  usePackagingTargets,
  useRefreshDownloads,
  type PackagingPreview,
  type PackagingTargetId,
} from "../packaging.service";
import { useEmbeddedSkillDetail } from "../../skill";
import { SkillVersionPicker } from "../../skill";
import { packagingGate } from "../packaging.model";
import { CompatibilityStatus } from "../../../shared/ui/CompatibilityStatus";
import { LabelledBadge } from "../../../shared/ui/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../../../shared/ui/LicenseBadge";
import { RiskIndicator } from "../../../shared/ui/RiskIndicator";
import { DownloadArtifactFacts } from "../components/DownloadArtifactFacts";
import { RiskVerdict } from "./components/RiskVerdict";
import { CompatibilityVerdict } from "./components/CompatibilityVerdict";
import { RetentionNotice } from "./components/RetentionNotice";
import { BlockedNotice } from "./components/BlockedNotice";
import { TargetOption } from "./components/TargetOption";
import { PreviewReport } from "./components/PreviewReport";

type PackagingSearch = { version?: string };

const DEAD_REASON_ID = "packaging-build-disabled-reason";

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
  const skill = useEmbeddedSkillDetail(skillId);
  const targets = usePackagingTargets();
  const refreshDownloads = useRefreshDownloads();

  const [chosen, setChosen] = useState<PackagingTargetId | "">("");
  const [includeTestCases, setIncludeTestCases] = useState(false);
  const navigate = useNavigate();

  const versionId = version || skill.data?.version?.version_id || "";
  const target = chosen || (targets.data?.targets[0]?.id ?? "");
  const preview = usePackagingPreview(skillId, versionId, target, includeTestCases);

  const build = useCreateDownload(skillId);
  const built =
    build.data?.skill_id === skillId && build.variables?.versionId === versionId
      ? build.data
      : null;
  const buildPackage = () =>
    build.mutate(
      { versionId, target: target as PackagingTargetId, includeTestCases },
      { onError: () => void preview.refetch() },
    );

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
          <p className="note" data-role="caveat">
            <strong>「規格驗證通過」不等於「裝得起來」，更不等於「腳本跑得動」</strong>。
          </p>

          <h2>打包目標</h2>
          <p className="note" data-role="teaching">
            每個目標的安裝說明也隨套件內的 INSTALL.md 一起下載。
          </p>
          {targets.isPending && <Loading what="打包目標" />}
          <ReadFailure error={targets.error} what="打包目標" />
          {targets.data && (
            <ul className="packaging-targets" data-role="evidence">
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
              onClick={buildPackage}
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
                （這是明文的「不做」）——它們證明得了「位元組沒有被改過」，
                證明不了「這份東西是誰做的」。
              </p>
              <p>
                <a href={downloadHref(built.artifact_id)} onClick={refreshDownloads}>
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

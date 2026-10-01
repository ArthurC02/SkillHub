import { ApiError } from "../../../core/api/client";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import {
  useCreateDownload,
  usePackagingPreview,
  usePackagingTargets,
  useRefreshDownloads,
  type PackagingPreview,
  type PackagingTargetId,
} from "../packaging.service";
import { SkillWorkspaceNav, useEmbeddedSkillDetail, useSkillVersions } from "../../skill";
import { packagingGate } from "../packaging.model";
import { matchingBuiltArtifact, resolveTarget } from "./build.model";
import { LabelledBadge } from "../../../shared/ui/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../../../shared/ui/LicenseBadge";
import { RiskIndicator } from "../../../shared/ui/RiskIndicator";
import { RiskVerdict } from "./components/RiskVerdict";
import { BlockedNotice } from "./components/BlockedNotice";
import { VersionPickerSection } from "./components/VersionPickerSection";
import { CompatibilitySection } from "./components/CompatibilitySection";
import { PackagingTargetsSection } from "./components/PackagingTargetsSection";
import { IncludeTestCasesSection } from "./components/IncludeTestCasesSection";
import { PackagingPreviewSection } from "./components/PackagingPreviewSection";
import { BuildControl } from "./components/BuildControl";
import { BuiltResultNotice } from "./components/BuiltResultNotice";

type PackagingSearch = { version?: string };

function PackagingVersionLabel({
  skillId,
  versionId,
  latestVersionId,
  latestVersionNumber,
}: {
  skillId: string;
  versionId: string;
  latestVersionId?: string;
  latestVersionNumber?: number;
}) {
  const versions = useSkillVersions(skillId);
  const selected = versions.data?.versions.find((item) => item.version_id === versionId);
  const versionNumber =
    selected?.version_number ?? (versionId === latestVersionId ? latestVersionNumber : undefined);

  return (
    <strong>
      正在打包 {versionNumber === undefined ? "所選版本" : `v${versionNumber}`}
      {versionId === latestVersionId ? "（最新版本）" : ""}
    </strong>
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
  const skill = useEmbeddedSkillDetail(skillId);
  const targets = usePackagingTargets();
  const refreshDownloads = useRefreshDownloads();

  const [chosen, setChosen] = useState<PackagingTargetId | "">("");
  const [includeTestCases, setIncludeTestCases] = useState(false);

  const versionId = version || skill.data?.version?.version_id || "";
  const target = resolveTarget(chosen, targets.data?.targets);
  const preview = usePackagingPreview(skillId, versionId, target, includeTestCases);

  const build = useCreateDownload(skillId);
  const built = matchingBuiltArtifact(build, skillId, versionId);
  const buildPackage = () =>
    build.mutate(
      { versionId, target: target as PackagingTargetId, includeTestCases },
      { onError: () => void preview.refetch() },
    );

  if (skill.isLoading) return <Loading what="這個小工具" />;
  if (skill.error instanceof ApiError && skill.error.status === 410)
    return <p role="alert">這個小工具已從目錄下架，內容不再提供。</p>;
  if (skill.error) return <ReadFailure error={skill.error} what="這個小工具" />;
  if (!skill.data) return <p role="alert">找不到這個小工具。</p>;

  const gate = packagingGate(skill.data);
  const deadReason = buildButtonReason({
    pending: build.isPending,
    target,
    targetsPending: targets.isPending,
    preview,
  });

  return (
    <section>
      <h1>小工具套件</h1>
      <SkillWorkspaceNav skillId={skillId} versionId={versionId || undefined} />
      <p>
        <Link to="/skills/$skillId" params={{ skillId }}>
          {skill.data.name}
        </Link>
        {versionId && (
          <>
            {" · "}
            <PackagingVersionLabel
              skillId={skillId}
              versionId={versionId}
              latestVersionId={skill.data.version?.version_id}
              latestVersionNumber={skill.data.version?.version_number}
            />
          </>
        )}
      </p>
      <p className="badge-row">
        <LabelledBadge kind="redistribution" value={skill.data.redistribution} />
        <LicenseBadge license={skill.data.license} />
      </p>
      <RiskVerdict risk={skill.data.risk} />
      <details>
        <summary>風險與 License 的逐項細節（與詳情頁同一次掃描結果）</summary>
        <LicenseNotes license={skill.data.license} />
        <RiskIndicator risk={skill.data.risk} cleanVerdict={false} />
      </details>
      {versionId === "" ? (
        <p role="alert">
          無權檢視——這個工作區看不到這個小工具的版本內容。別人的小工具要複製一份
          之後才會有屬於你的版本；這不代表它沒有版本。沒有版本內容就沒有東西可以打包。
        </p>
      ) : (
        <>
          <VersionPickerSection skillId={skillId} versionId={versionId} />

          {gate && <BlockedNotice reason={gate} />}

          <CompatibilitySection compatibility={skill.data.compatibility} />

          <PackagingTargetsSection targets={targets} selected={target} onSelect={setChosen} />

          <IncludeTestCasesSection checked={includeTestCases} onChange={setIncludeTestCases} />

          <PackagingPreviewSection preview={preview} target={target} />

          <BuildControl
            build={build}
            preview={preview}
            deadReason={deadReason}
            onBuild={buildPackage}
          />

          {built && (
            <BuiltResultNotice
              built={built}
              skillId={skillId}
              versionId={versionId}
              onDownload={refreshDownloads}
            />
          )}
        </>
      )}
    </section>
  );
}

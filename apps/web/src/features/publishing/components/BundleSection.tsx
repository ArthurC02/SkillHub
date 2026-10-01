import { useEffect, useId, useMemo, useRef, useState, type RefObject } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { useContinuationFocus } from "../../../shared/ui/useContinuationFocus";
import { API_BASE_URL } from "../../../core/api/client";
import type { SkillVersionSummary } from "../../../core/api/types";
import { useEmbeddedSkillDetails, useEmbeddedSkillVersions, useOwnSkills } from "../../skill";
import {
  useCreateBundleVersion,
  useDelistBundle,
  useExportBundle,
  useOwnBundleOverview,
  useOwnBundles,
  usePublishBundle,
  type BundleVersion,
  type OwnerBundlePublicationSummary,
  type OwnerBundleSummary,
} from "../publishing.service";
import { actionFailureSentence } from "../publishing.model";
import { DeliveryAudience } from "./DeliveryAudience";

const PLUGIN_SCOPE_NOTE = "Plugin 只含 Agent 小工具，不含 MCP 設定或宿主專屬元件。";

interface MemberChoice {
  skillId: string;
  name: string;
  versions: SkillVersionSummary[];
  needsAttestation: boolean;
}

interface ContinuationChoice {
  choice: MemberChoice;
  memberVersion: SkillVersionSummary;
}

function useMemberChoices(): {
  choices: MemberChoice[];
  isPending: boolean;
  error: Error | null;
} {
  const ownSkills = useOwnSkills();
  const skillIds = useMemo(
    () => ownSkills.data?.skills.map((s) => s.skill_id) ?? [],
    [ownSkills.data],
  );
  const details = useEmbeddedSkillDetails(skillIds);
  const versions = useEmbeddedSkillVersions(skillIds);

  const choices: MemberChoice[] = [];
  skillIds.forEach((skillId, i) => {
    const detail = details[i]?.data;
    if (!detail) return;
    const value = detail.redistribution?.value;
    choices.push({
      skillId,
      name: detail.name,
      versions: versions[i]?.data?.versions ?? [],
      needsAttestation: value === "self_supplied" || value === "generated",
    });
  });

  return {
    choices,
    isPending:
      ownSkills.isPending ||
      details.some((detail) => detail.isPending) ||
      versions.some((versionList) => versionList.isPending),
    error:
      ownSkills.error ??
      details.find((detail) => detail.error)?.error ??
      versions.find((versionList) => versionList.error)?.error ??
      null,
  };
}

export function BundleSection({ selectedVersion }: { selectedVersion?: string }) {
  const bundles = useOwnBundles();
  const overview = useOwnBundleOverview(Boolean(bundles.data?.bundles.length));
  const { choices, isPending: choicesPending, error: choicesError } = useMemberChoices();
  const groupedBundles = useMemo(() => {
    const groups = new Map<string, BundleVersion[]>();
    for (const bundle of bundles.data?.bundles ?? []) {
      const versions = groups.get(bundle.bundle) ?? [];
      versions.push(bundle);
      groups.set(bundle.bundle, versions);
    }
    return Array.from(groups, ([bundle, versions]) => ({ bundle, versions }));
  }, [bundles.data]);
  const overviewByBundle = useMemo(
    () => new Map((overview.data?.bundles ?? []).map((item) => [item.bundle, item])),
    [overview.data],
  );
  const needsAttestationOf = useMemo(() => {
    const bySkill = new Map(choices.map((choice) => [choice.skillId, choice.needsAttestation]));
    return (version: BundleVersion) =>
      version.members.some((member) => bySkill.get(member.skill_id) === true);
  }, [choices]);

  return (
    <section id="bundle-workspace">
      <h2>Bundle</h2>
      <p className="note">
        每個 Bundle 是一個持續營運的產品單位；版本是不可變快照，最新建立版本不一定等於目前公開的
        Release。
      </p>
      {bundles.isPending && <Loading what="Bundle 清單" />}
      <ReadFailure error={bundles.error} what="Bundle 清單" />
      {(bundles.data?.bundles.length ?? 0) > 0 && overview.isPending && (
        <Loading what="Bundle 發佈概覽" />
      )}
      <ReadFailure error={overview.error} what="Bundle 發佈概覽" />

      {bundles.data &&
        (groupedBundles.length === 0 ? (
          <p>目前還沒有 Bundle。先把一組可一起交付的小工具版本固定成第一個 Bundle Version。</p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {groupedBundles.map(({ bundle, versions }) => (
              <li key={bundle} className="download-item">
                <BundleOverviewCard
                  bundle={bundle}
                  versions={versions}
                  overview={overviewByBundle.get(bundle)}
                  overviewReady={overview.isSuccess}
                  needsAttestationOf={needsAttestationOf}
                />
              </li>
            ))}
          </ul>
        ))}

      <BundleCreationSection
        selectedVersion={selectedVersion}
        choices={choices}
        choicesPending={choicesPending}
        choicesError={choicesError}
      />
    </section>
  );
}

function BundleOverviewCard({
  bundle,
  versions,
  overview,
  overviewReady,
  needsAttestationOf,
}: {
  bundle: string;
  versions: BundleVersion[];
  overview?: OwnerBundleSummary;
  overviewReady: boolean;
  needsAttestationOf: (bundle: BundleVersion) => boolean;
}) {
  const delist = useDelistBundle(bundle);
  const latestVersion = versions[0];
  const publication = overview?.publication;

  return (
    <article aria-labelledby={`bundle-${bundle}`}>
      <header className="bundle-overview-header">
        <div>
          <p className="note">Bundle</p>
          <h3 id={`bundle-${bundle}`}>{bundle}</h3>
          <p>{latestVersion.description}</p>
        </div>
        {publication && (
          <span className={publication.status === "delisted" ? "badge badge-danger" : "badge"}>
            {publication.status === "published" ? "已發佈" : "已撤回"}
          </span>
        )}
      </header>

      {publication ? (
        <BundleOverviewPublication publication={publication} delist={delist} />
      ) : overviewReady && overview ? (
        <p className="note">尚未發佈。可從下方任一不可變版本建立第一個公開 Release。</p>
      ) : overviewReady ? (
        <p role="status" className="note">
          無法確認這個 Bundle 的發佈狀態；版本操作仍可使用，請重新整理後再確認。
        </p>
      ) : null}

      <h4>不可變版本</h4>
      <ul className="bundle-version-list">
        {versions.map((version, index) => (
          <li key={version.version} className="bundle-version-item">
            <BundleVersionOperations
              bundle={version}
              isLatest={index === 0}
              publication={publication}
              publicationKnown={overviewReady && Boolean(overview)}
              needsAttestation={needsAttestationOf(version)}
            />
          </li>
        ))}
      </ul>
      {delist.isError && (
        <p role="alert">
          {actionFailureSentence(delist.error, "撤回沒有完成；請確認狀態後再試一次。")}
        </p>
      )}
    </article>
  );
}

function BundleOverviewPublication({
  publication,
  delist,
}: {
  publication: OwnerBundlePublicationSummary;
  delist: ReturnType<typeof useDelistBundle>;
}) {
  return (
    <section className="bundle-publication" aria-label="Bundle 發佈狀態">
      <dl className="bundle-publication-grid">
        <div>
          <dt>公開頁</dt>
          <dd>
            <Link
              to="/p/$publisher/$name"
              params={{ publisher: publication.publisher, name: publication.name }}
            >
              {publication.address}
            </Link>
          </dd>
        </div>
        <div>
          <dt>目前 Release</dt>
          <dd>
            {publication.latest_release ? (
              <>
                v{publication.latest_release.bundle_version} ·{" "}
                <Timestamp at={publication.latest_release.released_at} />
              </>
            ) : (
              "尚無 Release"
            )}
          </dd>
        </div>
      </dl>
      <DeliveryAudience publication={publication} />
      <p className="note">
        發佈狀態更新於 <Timestamp at={publication.status_changed_at} />
        。公開取得的實際人數目前未量測；Artifact 下載紀錄不等於 Publication 採用數。
      </p>
      {publication.status === "published" && (
        <ConfirmDelete
          scopeId={`bundle-delist-scope-${publication.name}`}
          label="撤回"
          confirmLabel="確認撤回"
          pending={delist.isPending}
          onConfirm={() => delist.mutate()}
          scope={<>撤回會停止新的公開取得，但不會刪除 Bundle Version 或既有 Release 紀錄。</>}
        />
      )}
    </section>
  );
}

function BundleVersionOperations({
  bundle,
  isLatest,
  publication,
  publicationKnown,
  needsAttestation,
}: {
  bundle: BundleVersion;
  isLatest: boolean;
  publication?: OwnerBundlePublicationSummary;
  publicationKnown: boolean;
  needsAttestation: boolean;
}) {
  return (
    <div>
      <p>
        <strong>v{bundle.version}</strong> {isLatest && <span className="badge">最新建立</span>}
        {publication?.latest_release?.bundle_version === bundle.version && (
          <span className="badge">目前 Release</span>
        )}
      </p>
      <p>{bundle.description}</p>
      <p className="note">
        成員：{" "}
        {bundle.members.map((member, index) => (
          <span key={member.version_id}>
            {index > 0 && "、"}
            <Link
              to="/skills/$skillId/versions/$versionId"
              params={{ skillId: member.skill_id, versionId: member.version_id }}
            >
              {member.name} v{member.version_number}
            </Link>
          </span>
        ))}
      </p>
      <p className="note">{PLUGIN_SCOPE_NOTE}</p>
      <BundleVersionActions
        bundle={bundle}
        publication={publication}
        publicationKnown={publicationKnown}
        needsAttestation={needsAttestation}
      />
    </div>
  );
}

function BundleVersionActions({
  bundle,
  publication,
  publicationKnown,
  needsAttestation,
}: {
  bundle: BundleVersion;
  publication?: OwnerBundlePublicationSummary;
  publicationKnown: boolean;
  needsAttestation: boolean;
}) {
  const exportBundle = useExportBundle(bundle.bundle, bundle.version);
  const publish = usePublishBundle(bundle.bundle);
  const [attested, setAttested] = useState(false);
  const disabledReason =
    needsAttestation && !attested ? "發佈前需要確認你有權重新散布所有成員。" : undefined;
  const blockedReason = publicationKnown
    ? disabledReason
    : "發佈狀態尚未確認；重新整理並成功讀取概覽後才能發佈。";

  return (
    <>
      <BundleAttestation required={needsAttestation} checked={attested} onChange={setAttested} />
      <p className="bundle-version-actions">
        <button
          type="button"
          onClick={() => exportBundle.mutate()}
          disabled={exportBundle.isPending}
        >
          {exportBundle.isPending
            ? `正在匯出 v${bundle.version}…`
            : `匯出 v${bundle.version} Plugin`}
        </button>{" "}
        <button
          type="button"
          disabled={publish.isPending || Boolean(blockedReason)}
          aria-describedby={
            blockedReason ? `bundle-publish-disabled-${bundle.bundle}-${bundle.version}` : undefined
          }
          onClick={() =>
            publish.mutate({
              name: publication ? undefined : bundle.bundle,
              version: bundle.version,
              rightsAttested: attested,
            })
          }
        >
          {publish.isPending
            ? `正在發佈 v${bundle.version}…`
            : !publicationKnown
              ? "確認發佈狀態後可操作"
              : publication
                ? `發佈 v${bundle.version}`
                : `首次發佈 v${bundle.version}`}
        </button>
      </p>
      {blockedReason && (
        <p className="note" id={`bundle-publish-disabled-${bundle.bundle}-${bundle.version}`}>
          {blockedReason}
        </p>
      )}
      <BundleActionFeedback exportBundle={exportBundle} publish={publish} />
    </>
  );
}

function BundleAttestation({
  required,
  checked,
  onChange,
}: {
  required: boolean;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  if (!required) return null;
  return (
    <p>
      <label>
        <input
          type="checkbox"
          checked={checked}
          onChange={(event) => onChange(event.target.checked)}
        />{" "}
        我確認有權重新散布此版本的所有成員
      </label>
    </p>
  );
}

function BundleActionFeedback({
  exportBundle,
  publish,
}: {
  exportBundle: ReturnType<typeof useExportBundle>;
  publish: ReturnType<typeof usePublishBundle>;
}) {
  return (
    <>
      {exportBundle.isError && (
        <ReadFailure error={exportBundle.error} what="匯出 Plugin">
          <p role="alert">
            {actionFailureSentence(exportBundle.error, "匯出沒有完成；請確認版本狀態後再試一次。")}
          </p>
        </ReadFailure>
      )}
      {exportBundle.isSuccess && (
        <p>
          <a href={`${API_BASE_URL}${exportBundle.data.content_url}`}>
            下載 {exportBundle.data.file_name}
          </a>
          {" · "}
          <Link to="/workspace/downloads" search={{ artifact: exportBundle.data.artifact_id }}>
            查看這次交付紀錄
          </Link>
        </p>
      )}
      {publish.isError && (
        <p role="alert">
          {actionFailureSentence(publish.error, "發佈沒有完成；請確認版本狀態後再試一次。")}
        </p>
      )}
    </>
  );
}

function BundleCreationSection({
  selectedVersion,
  choices,
  choicesPending,
  choicesError,
}: {
  selectedVersion?: string;
  choices: MemberChoice[];
  choicesPending: boolean;
  choicesError: Error | null;
}) {
  const bundles = useOwnBundles();
  const createDetails = useRef<HTMLDetailsElement>(null);
  const hasBundles = (bundles.data?.bundles.length ?? 0) > 0;

  useEffect(() => {
    if (selectedVersion && createDetails.current) createDetails.current.open = true;
  }, [selectedVersion]);

  return (
    <section aria-label="建立 Bundle">
      <details className="bundle-create" ref={createDetails}>
        <summary>{hasBundles ? "建立另一個 Bundle" : "建立第一個 Bundle"}</summary>
        <CreateBundleForm
          choices={choices}
          choicesPending={choicesPending}
          choicesError={choicesError}
          selectedVersion={selectedVersion}
        />
      </details>
    </section>
  );
}

function CreateBundleForm({
  choices,
  choicesPending,
  choicesError,
  selectedVersion,
}: {
  choices: MemberChoice[];
  choicesPending: boolean;
  choicesError: Error | null;
  selectedVersion?: string;
}) {
  const create = useCreateBundleVersion();
  const [name, setName] = useState("");
  const [version, setVersion] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<Map<string, string>>(new Map());
  const appliedVersion = useRef<string | undefined>(undefined);
  const continuationSelect = useRef<HTMLSelectElement>(null);
  const nameId = useId();
  const versionId = useId();
  const descriptionId = useId();
  const noMembersId = useId();
  const continuation = useMemo(
    () =>
      choices
        .flatMap((choice) => choice.versions.map((memberVersion) => ({ choice, memberVersion })))
        .find(({ memberVersion }) => memberVersion.version_id === selectedVersion) as
        ContinuationChoice | undefined,
    [choices, selectedVersion],
  );
  const choicesReady = !choicesPending && !choicesError;

  useEffect(() => {
    if (
      !selectedVersion ||
      !continuation ||
      !choicesReady ||
      appliedVersion.current === selectedVersion
    )
      return;
    appliedVersion.current = selectedVersion;
    setSelected(new Map([[continuation.choice.skillId, selectedVersion]]));
  }, [choicesReady, continuation, selectedVersion]);

  useContinuationFocus(selectedVersion, Boolean(continuation && choicesReady), continuationSelect);

  function selectVersion(skillId: string, memberVersionId: string) {
    setSelected((current) => {
      const next = new Map(current);
      if (memberVersionId) next.set(skillId, memberVersionId);
      else next.delete(skillId);
      return next;
    });
  }

  return (
    <form
      className="bundle-form"
      onSubmit={(event) => {
        event.preventDefault();
        const memberVersionIds = Array.from(selected.values());
        create.mutate(
          {
            name: name.trim(),
            version: version.trim(),
            description: description.trim(),
            memberVersionIds,
          },
          {
            onSuccess: () => {
              setName("");
              setVersion("");
              setDescription("");
              setSelected(new Map());
            },
          },
        );
      }}
    >
      <h3>建立 Bundle Version</h3>
      <BundleIdentityFields
        nameId={nameId}
        name={name}
        onName={setName}
        versionId={versionId}
        version={version}
        onVersion={setVersion}
        descriptionId={descriptionId}
        description={description}
        onDescription={setDescription}
      />
      <BundleMemberChoices
        choices={choices}
        choicesPending={choicesPending}
        choicesError={choicesError}
        choicesReady={choicesReady}
        selectedVersion={selectedVersion}
        continuation={continuation}
        continuationSelect={continuationSelect}
        selected={selected}
        onSelect={selectVersion}
      />
      <p>
        <button
          type="submit"
          disabled={create.isPending || !choicesReady || selected.size === 0}
          aria-describedby={selected.size === 0 ? noMembersId : undefined}
        >
          {create.isPending ? "建立中…" : "建立"}
        </button>
      </p>
      {selected.size === 0 && (
        <p className="note" id={noMembersId}>
          先選擇至少一個要放入 Bundle 的小工具版本。
        </p>
      )}
      {create.isError && (
        <p role="alert">{actionFailureSentence(create.error, "建立沒有成功，可以再試一次。")}</p>
      )}
    </form>
  );
}

function BundleIdentityFields({
  nameId,
  name,
  onName,
  versionId,
  version,
  onVersion,
  descriptionId,
  description,
  onDescription,
}: {
  nameId: string;
  name: string;
  onName: (value: string) => void;
  versionId: string;
  version: string;
  onVersion: (value: string) => void;
  descriptionId: string;
  description: string;
  onDescription: (value: string) => void;
}) {
  return (
    <>
      <div className="field">
        <label htmlFor={nameId}>名稱</label>
        <input id={nameId} value={name} onChange={(event) => onName(event.target.value)} required />
      </div>
      <div className="field">
        <label htmlFor={versionId}>版本（semver，例如 1.0.0）</label>
        <input
          id={versionId}
          value={version}
          onChange={(event) => onVersion(event.target.value)}
          required
        />
      </div>
      <div className="field">
        <label htmlFor={descriptionId}>說明</label>
        <textarea
          id={descriptionId}
          value={description}
          onChange={(event) => onDescription(event.target.value)}
          required
        />
      </div>
    </>
  );
}

function BundleMemberChoices({
  choices,
  choicesPending,
  choicesError,
  choicesReady,
  selectedVersion,
  continuation,
  continuationSelect,
  selected,
  onSelect,
}: {
  choices: MemberChoice[];
  choicesPending: boolean;
  choicesError: Error | null;
  choicesReady: boolean;
  selectedVersion?: string;
  continuation?: ContinuationChoice;
  continuationSelect: RefObject<HTMLSelectElement | null>;
  selected: Map<string, string>;
  onSelect: (skillId: string, versionId: string) => void;
}) {
  return (
    <fieldset>
      <legend>成員版本</legend>
      <p className="note">每個小工具明確選一個不可變版本；平台不會替你改成最新版本。</p>
      {choicesPending && <Loading what="可加入 Bundle 的版本" />}
      <ReadFailure error={choicesError} what="可加入 Bundle 的版本">
        <p role="alert">暫時無法讀取可加入 Bundle 的版本。</p>
      </ReadFailure>
      {choicesReady && selectedVersion && !continuation && (
        <p role="status" className="note">
          無法確認要接續的 Version，因此沒有自動選擇其他版本。
        </p>
      )}
      {choicesReady && continuation && (
        <p role="status" className="note">
          從 v{continuation.memberVersion.version_number} 接續建立 Bundle。
        </p>
      )}
      {choicesReady && choices.length === 0 && (
        <p className="note">還沒有可以選的小工具——先建立至少一個有版本的小工具。</p>
      )}
      {choices.map((choice) => (
        <div className="field bundle-member" key={choice.skillId}>
          <label htmlFor={`bundle-member-${choice.skillId}`}>{choice.name}</label>
          <select
            id={`bundle-member-${choice.skillId}`}
            ref={continuation?.choice.skillId === choice.skillId ? continuationSelect : undefined}
            value={selected.get(choice.skillId) ?? ""}
            onChange={(event) => onSelect(choice.skillId, event.target.value)}
            disabled={!choicesReady}
            data-bundle-version={
              continuation?.choice.skillId === choice.skillId ? selectedVersion : undefined
            }
          >
            <option value="">不加入</option>
            {choice.versions.map((memberVersion) => (
              <option key={memberVersion.version_id} value={memberVersion.version_id}>
                v{memberVersion.version_number} · {memberVersion.content_hash}
              </option>
            ))}
          </select>
        </div>
      ))}
    </fieldset>
  );
}

import { useEffect, useId, useMemo, useRef, useState, type RefObject } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { useContinuationFocus } from "../../../shared/ui/useContinuationFocus";
import { usePaginationFocus } from "../../../shared/ui/usePaginationFocus";
import { API_BASE_URL, canRetainLoadedPage } from "../../../core/api/client";
import type { SkillDetail, SkillVersionSummary } from "../../../core/api/types";
import {
  useEmbeddedSkillDetails,
  useEmbeddedSkillVersions,
  useOwnSkillPages,
  ownSkillsFromPages,
} from "../../skill";
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
import {
  actionFailureSentence,
  PUBLISHING_REFUSAL_LABEL,
  writeFailureSentence,
} from "../publishing.model";
import { packagingGate } from "../../packaging";
import { AcquisitionFailure } from "./AcquisitionFailure";
import { DeliveryAudience } from "./DeliveryAudience";

const PLUGIN_SCOPE_NOTE = "Plugin 只含 Agent 小工具，不含 MCP 設定或宿主專屬元件。";

interface MemberChoice {
  skillId: string;
  name: string;
  versions: SkillVersionSummary[];
  needsAttestation: boolean;
  refusal?: string;
}

interface MemberGate {
  needsAttestation: boolean;
  refusal?: string;
}

function memberRefusal(detail: SkillDetail) {
  const gate = packagingGate(detail);
  if (gate !== "license_hold" && gate !== "not_redistributable" && gate !== "license_unknown")
    return undefined;
  return `成員「${detail.name}」：${PUBLISHING_REFUSAL_LABEL[gate]}`;
}

interface ContinuationChoice {
  choice: MemberChoice;
  memberVersion: SkillVersionSummary;
}

interface MemberFocus {
  skillId: string;
  bindTarget: (element: HTMLElement | null) => void;
}

function continuationFor(choices: MemberChoice[], selectedVersion?: string) {
  return choices
    .flatMap((choice) => choice.versions.map((memberVersion) => ({ choice, memberVersion })))
    .find(({ memberVersion }) => memberVersion.version_id === selectedVersion);
}

function useMemberChoices(): {
  query: ReturnType<typeof useOwnSkillPages>;
  choices: MemberChoice[];
  isPending: boolean;
  paused: boolean;
  error: Error | null;
  retryFailedReads: () => void;
  refresh: () => Promise<unknown>;
  fetching: boolean;
  hasNextPage: boolean;
  fetchingNextPage: boolean;
  loaded: number;
} {
  const ownSkills = useOwnSkillPages();
  const nextPageFailure = canRetainLoadedPage(ownSkills.error, ownSkills.isFetchNextPageError);
  const loadedSkills = useMemo(
    () =>
      ownSkillsFromPages(ownSkills.error && !nextPageFailure ? undefined : ownSkills.data?.pages),
    [ownSkills.error, nextPageFailure, ownSkills.data],
  );
  const skillIds = useMemo(() => loadedSkills?.skills.map((s) => s.skill_id) ?? [], [loadedSkills]);
  const details = useEmbeddedSkillDetails(skillIds);
  const versions = useEmbeddedSkillVersions(skillIds);
  const queries = [ownSkills, ...details, ...versions];

  const choices: MemberChoice[] = [];
  skillIds.forEach((skillId, i) => {
    const detail = details[i]?.error ? undefined : details[i]?.data;
    if (!detail) return;
    const value = detail.redistribution?.value;
    choices.push({
      skillId,
      name: detail.name,
      versions: versions[i]?.error ? [] : (versions[i]?.data?.versions ?? []),
      needsAttestation: value === "self_supplied" || value === "generated",
      refusal: memberRefusal(detail),
    });
  });

  return {
    query: ownSkills,
    choices,
    isPending: queries.some((query) => query.isPending || query.fetchStatus !== "idle"),
    paused: queries.some((query) => query.fetchStatus === "paused"),
    error:
      ownSkills.error ??
      details.find((detail) => detail.error)?.error ??
      versions.find((versionList) => versionList.error)?.error ??
      null,
    fetching: queries.some((query) => query.isFetching),
    refresh: () => Promise.all(queries.map((query) => query.refetch())),
    retryFailedReads: () => {
      if (nextPageFailure) {
        void ownSkills.fetchNextPage();
        return;
      }
      for (const query of queries) {
        if (query.error) void query.refetch();
      }
    },
    hasNextPage: ownSkills.hasNextPage,
    fetchingNextPage: ownSkills.isFetchingNextPage,
    loaded: loadedSkills?.skills.length ?? 0,
  };
}

export function BundleSection({ selectedVersion }: { selectedVersion?: string }) {
  const bundles = useOwnBundles();
  const readableBundles = bundles.error ? undefined : bundles.data;
  const overview = useOwnBundleOverview(Boolean(readableBundles?.bundles.length));
  const memberRead = useMemberChoices();
  const {
    choices,
    isPending: choicesPending,
    error: choicesError,
    hasNextPage: hasMoreChoices,
  } = memberRead;
  const { headingRef, loadMore, bindTarget, row } = usePaginationFocus(
    {
      ...memberRead.query,
      fetchStatus: memberRead.paused ? "paused" : choicesPending ? "fetching" : "idle",
      error: choicesError,
    },
    choices,
  );
  const groupedBundles = useMemo(() => {
    const groups = new Map<string, BundleVersion[]>();
    for (const bundle of readableBundles?.bundles ?? []) {
      const versions = groups.get(bundle.bundle) ?? [];
      versions.push(bundle);
      groups.set(bundle.bundle, versions);
    }
    return Array.from(groups, ([bundle, versions]) => ({ bundle, versions }));
  }, [readableBundles]);
  const overviewByBundle = useMemo(
    () =>
      new Map(
        (overview.error ? [] : (overview.data?.bundles ?? [])).map((item) => [item.bundle, item]),
      ),
    [overview.data, overview.error],
  );
  const memberGateOf = useMemo(() => {
    const bySkill = new Map(choices.map((choice) => [choice.skillId, choice]));
    return (version: BundleVersion): MemberGate => {
      const members = version.members.map((member) => bySkill.get(member.skill_id));
      return {
        needsAttestation: members.some((choice) => choice?.needsAttestation === true),
        refusal: members.find((choice) => choice?.refusal)?.refusal,
      };
    };
  }, [choices]);

  const memberPolicyKnownOf = (version: BundleVersion) =>
    !choicesPending &&
    !choicesError &&
    version.members.every((member) => choices.some((choice) => choice.skillId === member.skill_id));

  return (
    <section id="bundle-workspace">
      <h2 id="bundle-workspace-heading" tabIndex={-1} ref={headingRef}>
        Bundle
      </h2>
      <p className="note">
        每個 Bundle 是一個持續營運的產品單位；版本是不可變快照，最新建立版本不一定等於目前公開的
        Release。
      </p>
      <BundleReadStatus
        bundles={bundles}
        overview={overview}
        members={memberRead}
        loadMore={loadMore}
        buttonRef={bindTarget}
      />

      {readableBundles &&
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
                  memberGateOf={memberGateOf}
                  memberPolicyKnownOf={memberPolicyKnownOf}
                />
              </li>
            ))}
          </ul>
        ))}

      <BundleCreationSection
        memberFocus={row ? { skillId: row.skillId, bindTarget } : undefined}
        selectedVersion={selectedVersion}
        choices={choices}
        choicesPending={choicesPending}
        choicesError={choicesError}
        hasMoreChoices={hasMoreChoices}
      />
    </section>
  );
}

function BundleReadStatus({
  bundles,
  overview,
  members,
  loadMore,
  buttonRef,
}: {
  bundles: ReturnType<typeof useOwnBundles>;
  overview: ReturnType<typeof useOwnBundleOverview>;
  members: ReturnType<typeof useMemberChoices>;
  loadMore: () => void;
  buttonRef: (element: HTMLButtonElement | null) => void;
}) {
  const readableBundles = bundles.error ? undefined : bundles.data;
  const lists = [bundles, overview];
  return (
    <>
      {bundles.isPending && <Loading what="Bundle 清單" />}
      <ReadFailure
        error={bundles.error}
        what="Bundle 清單"
        retry={() => void bundles.refetch()}
        retrying={bundles.isFetching}
        focusTarget="bundle-workspace-heading"
      />
      {Boolean(readableBundles?.bundles.length) && overview.isPending && (
        <Loading what="Bundle 發佈概覽" />
      )}
      <ReadFailure
        error={overview.error}
        what="Bundle 發佈概覽"
        retry={() => void overview.refetch()}
        retrying={overview.isFetching}
        focusTarget="bundle-workspace-heading"
      />
      {(members.error || members.isPending) && (
        <div id="bundle-members-read-status">
          <ReadFailure
            error={members.error}
            what="可加入 Bundle 的版本"
            retry={members.retryFailedReads}
            retrying={members.fetching}
            focusTarget="bundle-workspace-heading"
          >
            <p role="alert">暫時無法讀取可加入 Bundle 的版本。</p>
          </ReadFailure>
          {!members.error && <Loading what="可加入 Bundle 的版本" paused={members.paused} />}
        </div>
      )}
      {members.loaded > 0 && (
        <p className="note" role="status">
          已載入 {members.loaded} 個小工具
          {members.hasNextPage ? "；仍有其他小工具尚未載入。" : "。"}
        </p>
      )}
      {members.hasNextPage && !members.error && (
        <button
          type="button"
          disabled={members.fetchingNextPage || members.paused}
          aria-describedby={members.paused ? "bundle-members-read-status" : undefined}
          onClick={loadMore}
          ref={buttonRef}
        >
          {members.fetchingNextPage ? "載入中…" : "載入更多小工具"}
        </button>
      )}
      {readableBundles && !bundles.error && (
        <ListFreshness
          inFlight={false}
          showWhenIdle
          subject="Bundle"
          updatedAt={bundles.dataUpdatedAt}
          fetching={lists.some((query) => query.isFetching) || members.fetching}
          paused={lists.some((query) => query.fetchStatus === "paused") || members.paused}
          refetch={() => Promise.all([bundles.refetch(), overview.refetch(), members.refresh()])}
        />
      )}
    </>
  );
}

function BundleOverviewCard({
  bundle,
  versions,
  overview,
  overviewReady,
  memberGateOf,
  memberPolicyKnownOf,
}: {
  bundle: string;
  versions: BundleVersion[];
  overview?: OwnerBundleSummary;
  overviewReady: boolean;
  memberGateOf: (bundle: BundleVersion) => MemberGate;
  memberPolicyKnownOf: (version: BundleVersion) => boolean;
}) {
  const delist = useDelistBundle(bundle);
  const publish = usePublishBundle(bundle);
  const pending = publish.isPending || delist.isPending;
  const focusTarget = `bundle-${bundle}`;
  const latestVersion = versions[0];
  const publication = overview?.publication;

  return (
    <article aria-labelledby={`bundle-${bundle}`}>
      <header className="bundle-overview-header">
        <div>
          <p className="note">Bundle</p>
          <h3 id={focusTarget} tabIndex={-1}>
            {bundle}
          </h3>
          <p>{latestVersion.description}</p>
        </div>
        {publication && (
          <span
            role="status"
            className={publication.status === "delisted" ? "badge badge-danger" : "badge"}
          >
            {publication.status === "published" ? "已發佈" : "已撤回"}
          </span>
        )}
      </header>

      {pending && (
        <p id={`${focusTarget}-pending`} role="status" className="note">
          正在送出或核對 Bundle 發佈狀態，完成前暫停發佈與撤回。目前先顯示上次讀取的結果。
        </p>
      )}

      {publication ? (
        <BundleOverviewPublication
          publication={publication}
          delist={delist}
          disabled={pending}
          focusTarget={focusTarget}
        />
      ) : overviewReady && overview ? (
        <p className="note">尚未發佈。可從下方任一不可變版本建立第一個公開 Release。</p>
      ) : overviewReady ? (
        <p role="status" className="note">
          無法確認這個 Bundle 的發佈狀態；版本操作仍可使用，請重新整理後再確認。
        </p>
      ) : null}

      {overviewReady && overview && !pending && (
        <>
          <ReadFailure error={publish.error} what="發佈">
            <p role="alert">
              v{publish.variables?.version}：
              {actionFailureSentence(publish.error, writeFailureSentence(publish.error, "發佈"))}
            </p>
          </ReadFailure>
          <ReadFailure error={delist.error} what="撤回">
            <p role="alert">
              {actionFailureSentence(delist.error, writeFailureSentence(delist.error, "撤回"))}
            </p>
          </ReadFailure>
        </>
      )}

      <h4>不可變版本</h4>
      <ul className="bundle-version-list">
        {versions.map((version, index) => (
          <li key={version.version} className="bundle-version-item">
            <BundleVersionOperations
              bundle={version}
              isLatest={index === 0}
              publication={publication}
              publicationKnown={overviewReady && Boolean(overview)}
              gate={memberGateOf(version)}
              memberPolicyKnown={memberPolicyKnownOf(version)}
              publish={publish}
              pending={pending}
              focusTarget={focusTarget}
            />
          </li>
        ))}
      </ul>
    </article>
  );
}

function BundleOverviewPublication({
  publication,
  delist,
  disabled,
  focusTarget,
}: {
  publication: OwnerBundlePublicationSummary;
  delist: ReturnType<typeof useDelistBundle>;
  disabled: boolean;
  focusTarget: string;
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
          <dd aria-live="polite" aria-atomic="true">
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
          disabled={disabled}
          disabledReasonId={`${focusTarget}-pending`}
          onConfirm={() => {
            delist.mutate();
            document.getElementById(focusTarget)?.focus();
          }}
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
  gate,
  memberPolicyKnown,
  publish,
  pending,
  focusTarget,
}: {
  bundle: BundleVersion;
  isLatest: boolean;
  publication?: OwnerBundlePublicationSummary;
  publicationKnown: boolean;
  gate: MemberGate;
  memberPolicyKnown: boolean;
  publish: ReturnType<typeof usePublishBundle>;
  pending: boolean;
  focusTarget: string;
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
        gate={gate}
        memberPolicyKnown={memberPolicyKnown}
        publish={publish}
        pending={pending}
        focusTarget={focusTarget}
      />
    </div>
  );
}

function BundleVersionActions({
  bundle,
  publication,
  publicationKnown,
  gate,
  memberPolicyKnown,
  publish,
  pending,
  focusTarget,
}: {
  bundle: BundleVersion;
  publication?: OwnerBundlePublicationSummary;
  publicationKnown: boolean;
  gate: MemberGate;
  memberPolicyKnown: boolean;
  publish: ReturnType<typeof usePublishBundle>;
  pending: boolean;
  focusTarget: string;
}) {
  const exportBundle = useExportBundle(bundle.bundle, bundle.version);
  const [attested, setAttested] = useState(false);
  const { needsAttestation, refusal } = gate;
  const disabledReason =
    refusal ??
    (needsAttestation && !attested ? "發佈前需要確認你有權重新散布所有成員。" : undefined);
  const blockedReason = publicationKnown
    ? memberPolicyKnown
      ? disabledReason
      : "散布需求尚未確認；請先重新讀取可加入 Bundle 的版本。"
    : "發佈狀態尚未確認；重新整理並成功讀取概覽後才能發佈。";

  return (
    <>
      <BundleAttestation
        required={needsAttestation}
        checked={attested}
        onChange={setAttested}
        disabled={pending}
        disabledReasonId={`${focusTarget}-pending`}
      />
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
          disabled={pending || Boolean(blockedReason)}
          aria-describedby={
            pending
              ? `${focusTarget}-pending`
              : blockedReason
                ? `bundle-publish-disabled-${bundle.bundle}-${bundle.version}`
                : undefined
          }
          onClick={() => {
            publish.mutate({
              name: publication ? undefined : bundle.bundle,
              version: bundle.version,
              rightsAttested: attested,
            });
            document.getElementById(focusTarget)?.focus();
          }}
        >
          {publish.isPending && publish.variables?.version === bundle.version
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
      <BundleActionFeedback exportBundle={exportBundle} />
    </>
  );
}

function BundleAttestation({
  required,
  checked,
  onChange,
  disabled,
  disabledReasonId,
}: {
  required: boolean;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled: boolean;
  disabledReasonId: string;
}) {
  if (!required) return null;
  return (
    <p>
      <label>
        <input
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={disabled ? disabledReasonId : undefined}
          onChange={(event) => onChange(event.target.checked)}
        />{" "}
        我確認有權重新散布此版本的所有成員
      </label>
    </p>
  );
}

function BundleActionFeedback({
  exportBundle,
}: {
  exportBundle: ReturnType<typeof useExportBundle>;
}) {
  return (
    <>
      <AcquisitionFailure
        error={exportBundle.error}
        what="匯出 Plugin"
        fallback="匯出沒有完成；請確認版本狀態後再試一次。"
      />
      {exportBundle.isSuccess && (
        <p role="status">
          <a href={`${API_BASE_URL}${exportBundle.data.content_url}`}>
            下載 {exportBundle.data.file_name}
          </a>
          （到期時間：
          <Timestamp at={exportBundle.data.expires_at} />）{" · "}
          <Link to="/workspace/downloads" search={{ artifact: exportBundle.data.artifact_id }}>
            查看這次交付紀錄
          </Link>
        </p>
      )}
    </>
  );
}

function BundleCreationSection({
  memberFocus,
  selectedVersion,
  choices,
  choicesPending,
  choicesError,
  hasMoreChoices,
}: {
  memberFocus?: MemberFocus;
  selectedVersion?: string;
  choices: MemberChoice[];
  choicesPending: boolean;
  choicesError: Error | null;
  hasMoreChoices: boolean;
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
        <summary>
          {bundles.error || bundles.isPending
            ? "建立 Bundle"
            : hasBundles
              ? "建立另一個 Bundle"
              : "建立第一個 Bundle"}
        </summary>
        <CreateBundleForm
          memberFocus={memberFocus}
          choices={choices}
          choicesPending={choicesPending}
          choicesError={choicesError}
          hasMoreChoices={hasMoreChoices}
          selectedVersion={selectedVersion}
        />
      </details>
    </section>
  );
}

function CreateBundleForm({
  memberFocus,
  choices,
  choicesPending,
  choicesError,
  hasMoreChoices,
  selectedVersion,
}: {
  memberFocus?: MemberFocus;
  choices: MemberChoice[];
  choicesPending: boolean;
  choicesError: Error | null;
  hasMoreChoices: boolean;
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
    () => continuationFor(choices, selectedVersion),
    [choices, selectedVersion],
  );
  const choicesReady = !choicesPending && !choicesError;

  function clearResult() {
    if (!create.isPending && (create.isSuccess || create.isError)) create.reset();
  }

  useEffect(() => {
    if (
      !selectedVersion ||
      !continuation ||
      !choicesReady ||
      create.isPending ||
      appliedVersion.current === selectedVersion
    )
      return;
    appliedVersion.current = selectedVersion;
    setSelected(new Map([[continuation.choice.skillId, selectedVersion]]));
  }, [choicesReady, continuation, create.isPending, selectedVersion]);

  useContinuationFocus(selectedVersion, Boolean(continuation && choicesReady), continuationSelect);

  function selectVersion(skillId: string, memberVersionId: string) {
    clearResult();
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
        if (create.isPending || !choicesReady) return;
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
        onEdit={clearResult}
        nameId={nameId}
        name={name}
        onName={setName}
        versionId={versionId}
        version={version}
        onVersion={setVersion}
        descriptionId={descriptionId}
        description={description}
        onDescription={setDescription}
        pending={create.isPending}
      />
      <BundleMemberChoices
        memberFocus={memberFocus}
        choices={choices}
        choicesReady={choicesReady}
        hasMoreChoices={hasMoreChoices}
        selectedVersion={selectedVersion}
        continuation={continuation}
        continuationSelect={continuationSelect}
        selected={selected}
        onSelect={selectVersion}
        pending={create.isPending}
      />
      <BundleCreationAction
        create={create}
        choicesReady={choicesReady}
        selectedCount={selected.size}
        noMembersId={noMembersId}
      />
    </form>
  );
}

function BundleCreationAction({
  create,
  choicesReady,
  selectedCount,
  noMembersId,
}: {
  create: ReturnType<typeof useCreateBundleVersion>;
  choicesReady: boolean;
  selectedCount: number;
  noMembersId: string;
}) {
  return (
    <>
      <p>
        <button
          type="submit"
          disabled={create.isPending || !choicesReady || selectedCount === 0}
          aria-describedby={
            !choicesReady
              ? "bundle-members-read-status"
              : selectedCount === 0
                ? noMembersId
                : undefined
          }
        >
          {create.isPending ? "建立中…" : "建立"}
        </button>
      </p>
      {selectedCount === 0 && (
        <p className="note" id={noMembersId}>
          先選擇至少一個要放入 Bundle 的小工具版本。
        </p>
      )}
      {create.isError && (
        <p role="alert">{actionFailureSentence(create.error, "建立沒有成功，可以再試一次。")}</p>
      )}
      {create.isSuccess && (
        <p role="status">
          已建立 {create.data.bundle} v{create.data.version}。
        </p>
      )}
    </>
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
  pending,
  onEdit,
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
  pending: boolean;
  onEdit: () => void;
}) {
  return (
    <>
      <div className="field">
        <label htmlFor={nameId}>名稱</label>
        <input
          id={nameId}
          value={name}
          onChange={(event) => {
            onEdit();
            onName(event.target.value);
          }}
          required
          disabled={pending}
        />
      </div>
      <div className="field">
        <label htmlFor={versionId}>版本（semver，例如 1.0.0）</label>
        <input
          id={versionId}
          value={version}
          onChange={(event) => {
            onEdit();
            onVersion(event.target.value);
          }}
          required
          disabled={pending}
        />
      </div>
      <div className="field">
        <label htmlFor={descriptionId}>說明</label>
        <textarea
          id={descriptionId}
          value={description}
          onChange={(event) => {
            onEdit();
            onDescription(event.target.value);
          }}
          required
          disabled={pending}
        />
      </div>
    </>
  );
}

function BundleMemberChoices({
  memberFocus,
  choices,
  choicesReady,
  hasMoreChoices,
  selectedVersion,
  continuation,
  continuationSelect,
  selected,
  onSelect,
  pending,
}: {
  memberFocus?: MemberFocus;
  choices: MemberChoice[];
  choicesReady: boolean;
  hasMoreChoices: boolean;
  selectedVersion?: string;
  continuation?: ContinuationChoice;
  continuationSelect: RefObject<HTMLSelectElement | null>;
  selected: Map<string, string>;
  onSelect: (skillId: string, versionId: string) => void;
  pending: boolean;
}) {
  return (
    <fieldset>
      <legend>成員版本</legend>
      <p className="note">每個小工具明確選一個不可變版本；平台不會替你改成最新版本。</p>
      {choicesReady && !hasMoreChoices && selectedVersion && !continuation && (
        <p role="status" className="note">
          無法確認要接續的 Version，因此沒有自動選擇其他版本。
        </p>
      )}
      {choicesReady && continuation && (
        <p role="status" className="note">
          從 v{continuation.memberVersion.version_number} 接續建立 Bundle。
        </p>
      )}
      {choicesReady && !hasMoreChoices && choices.length === 0 && (
        <p className="note">還沒有可以選的小工具——先建立至少一個有版本的小工具。</p>
      )}
      {choices.map((choice) => (
        <div className="field bundle-member" key={choice.skillId}>
          <label htmlFor={`bundle-member-${choice.skillId}`}>{choice.name}</label>
          <select
            id={`bundle-member-${choice.skillId}`}
            ref={
              memberFocus?.skillId === choice.skillId
                ? memberFocus.bindTarget
                : continuation?.choice.skillId === choice.skillId
                  ? continuationSelect
                  : undefined
            }
            value={selected.get(choice.skillId) ?? ""}
            onChange={(event) => onSelect(choice.skillId, event.target.value)}
            disabled={!choicesReady || pending}
            aria-describedby={!choicesReady ? "bundle-members-read-status" : undefined}
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

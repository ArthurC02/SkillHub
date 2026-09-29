import { useEffect, useId, useMemo, useRef, useState, type RefObject } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { Tip } from "../../../shared/ui/Tip";
import { useContinuationFocus } from "../../../shared/ui/useContinuationFocus";
import { API_BASE_URL, ApiError } from "../../../core/api/client";
import type { SkillVersionSummary } from "../../../core/api/types";
import { useEmbeddedSkillDetails, useEmbeddedSkillVersions, useOwnSkills } from "../../skill";
import {
  useCreateBundleVersion,
  useDelistBundle,
  useExportBundle,
  useOwnBundlePublication,
  useOwnBundles,
  usePublishBundle,
  type BundleVersion,
  type Publication,
} from "../publishing.service";
import { actionFailureSentence } from "../publishing.model";
import { DeliveryAudience } from "./DeliveryAudience";

const PLUGIN_SCOPE_NOTE = "Plugin 只含 Agent Skill，不含 MCP 設定或宿主專屬元件。";

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
  const { choices, isPending: choicesPending, error: choicesError } = useMemberChoices();
  const createDetails = useRef<HTMLDetailsElement>(null);
  const hasBundles = (bundles.data?.bundles.length ?? 0) > 0;
  const needsAttestationOf = useMemo(() => {
    const map = new Map(choices.map((c) => [c.skillId, c.needsAttestation]));
    return (bundle: BundleVersion) => bundle.members.some((m) => map.get(m.skill_id) === true);
  }, [choices]);

  useEffect(() => {
    if (selectedVersion && createDetails.current) createDetails.current.open = true;
  }, [selectedVersion]);

  return (
    <section>
      <h2>Bundle</h2>
      <Tip anchor="Bundle 是什麼">
        Bundle 把幾個你自己的 Skill 版本釘成一組，可以匯出成一個 Agent
        Plugin，或以一個公開位址發佈。
      </Tip>

      {bundles.isPending && <Loading what="Bundle 清單" />}
      <ReadFailure error={bundles.error} what="Bundle 清單" />

      {bundles.data &&
        (bundles.data.bundles.length === 0 ? (
          <p>還沒有建立過任何 Bundle。這裡是空的代表你還沒有建立過，不是清單讀取失敗。</p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {bundles.data.bundles.map((bundle) => (
              <li key={`${bundle.bundle}@${bundle.version}`} className="download-item">
                <BundleRow bundle={bundle} needsAttestation={needsAttestationOf(bundle)} />
              </li>
            ))}
          </ul>
        ))}

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

function BundleRow({
  bundle,
  needsAttestation,
}: {
  bundle: BundleVersion;
  needsAttestation: boolean;
}) {
  const exportBundle = useExportBundle(bundle.bundle, bundle.version);

  return (
    <div>
      <p>
        <strong>{bundle.bundle}</strong> v{bundle.version} — {bundle.description}
      </p>
      <p className="note">
        成員：
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
      <p>
        <button
          type="button"
          onClick={() => exportBundle.mutate()}
          disabled={exportBundle.isPending}
        >
          {exportBundle.isPending
            ? `正在匯出 v${bundle.version}…`
            : `匯出 v${bundle.version} 為 Plugin`}
        </button>
      </p>
      {exportBundle.isError && (
        <ReadFailure error={exportBundle.error} what="匯出 Plugin">
          <p role="alert">
            {actionFailureSentence(exportBundle.error, "匯出沒有成功，可以再試一次。")}
          </p>
        </ReadFailure>
      )}
      {exportBundle.isSuccess && (
        <p>
          <a href={`${API_BASE_URL}${exportBundle.data.content_url}`}>
            下載 {exportBundle.data.file_name}
          </a>
          {" ｜ "}
          <Link to="/workspace/downloads" search={{ artifact: exportBundle.data.artifact_id }}>
            在交付紀錄查看這一份
          </Link>
        </p>
      )}

      <BundlePublishArea bundle={bundle} needsAttestation={needsAttestation} />
    </div>
  );
}

function BundlePublishArea({
  bundle,
  needsAttestation,
}: {
  bundle: BundleVersion;
  needsAttestation: boolean;
}) {
  const publication = useOwnBundlePublication(bundle.bundle);
  const publish = usePublishBundle(bundle.bundle);
  const delist = useDelistBundle(bundle.bundle);
  const [name, setName] = useState(bundle.bundle);
  const [attested, setAttested] = useState(false);
  const nameInputId = useId();

  const notPublishedYet = publication.error instanceof ApiError && publication.error.status === 404;
  const disabledReason = needsAttestation && !attested ? "先勾選下面的聲明才能發佈。" : undefined;

  return (
    <div>
      {publication.isPending && <Loading what="發佈狀態" />}
      {publication.error && !notPublishedYet && (
        <ReadFailure error={publication.error} what="發佈狀態" />
      )}

      {publication.data ? (
        <PublishedBundleView
          publication={publication.data}
          bundleVersion={bundle.version}
          needsAttestation={needsAttestation}
          attested={attested}
          setAttested={setAttested}
          disabledReason={disabledReason}
          publish={publish}
          delist={delist}
        />
      ) : notPublishedYet ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            publish.mutate({
              name: name.trim(),
              version: bundle.version,
              rightsAttested: attested,
            });
          }}
        >
          <label htmlFor={nameInputId}>發佈名稱</label>
          <br />
          <input
            id={nameInputId}
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={64}
            required
          />
          {needsAttestation && (
            <p>
              <label>
                <input
                  type="checkbox"
                  checked={attested}
                  onChange={(event) => setAttested(event.target.checked)}
                />{" "}
                我有權散布這些內容
              </label>
            </p>
          )}
          <button
            type="submit"
            disabled={publish.isPending || Boolean(disabledReason)}
            aria-describedby={
              disabledReason ? `bundle-publish-disabled-${bundle.bundle}` : undefined
            }
          >
            {publish.isPending ? `正在發佈 v${bundle.version}…` : `發佈 v${bundle.version}`}
          </button>
          {disabledReason && (
            <p className="note" id={`bundle-publish-disabled-${bundle.bundle}`}>
              {disabledReason}
            </p>
          )}
          {publish.isError && (
            <p role="alert">
              {actionFailureSentence(publish.error, "發佈沒有成功，可以再試一次。")}
            </p>
          )}
        </form>
      ) : null}
    </div>
  );
}

function PublishedBundleView({
  publication,
  bundleVersion,
  needsAttestation,
  attested,
  setAttested,
  disabledReason,
  publish,
  delist,
}: {
  publication: Publication;
  bundleVersion: string;
  needsAttestation: boolean;
  attested: boolean;
  setAttested: (value: boolean) => void;
  disabledReason: string | undefined;
  publish: ReturnType<typeof usePublishBundle>;
  delist: ReturnType<typeof useDelistBundle>;
}) {
  const latest = publication.releases[0];

  return (
    <>
      <p>
        公開位址：
        <Link
          to="/p/$publisher/$name"
          params={{ publisher: publication.publisher, name: publication.name }}
        >
          {publication.address}
        </Link>
      </p>
      <p>
        狀態：{publication.status === "published" ? "已發佈" : "已撤回"}（
        <Timestamp at={publication.status_changed_at} />）
      </p>
      {latest && (
        <p>
          最新 Release：v{latest.bundle_version}，發佈於 <Timestamp at={latest.released_at} />
        </p>
      )}
      <DeliveryAudience publication={publication} />

      {needsAttestation && (
        <p>
          <label>
            <input
              type="checkbox"
              checked={attested}
              onChange={(event) => setAttested(event.target.checked)}
            />{" "}
            我有權散布這些內容
          </label>
        </p>
      )}
      <p>
        <button
          type="button"
          disabled={publish.isPending || Boolean(disabledReason)}
          onClick={() => publish.mutate({ version: bundleVersion, rightsAttested: attested })}
        >
          {publish.isPending ? `正在發佈 v${bundleVersion}…` : `發佈 v${bundleVersion}`}
        </button>
      </p>
      {disabledReason && <p className="note">{disabledReason}</p>}
      {publish.isError && (
        <p role="alert">{actionFailureSentence(publish.error, "發佈沒有成功，可以再試一次。")}</p>
      )}

      {publication.status === "published" && (
        <ConfirmDelete
          scopeId={`bundle-delist-scope-${publication.name}`}
          label="撤回"
          confirmLabel="確認撤回"
          pending={delist.isPending}
          onConfirm={() => delist.mutate()}
          scope={
            <>
              撤回後這個位址只會顯示「作者已撤回」，不再提供內容，也不能下載；名稱仍然是你的，之後可以用同一個名稱再發佈一次。
            </>
          }
        />
      )}
      {delist.isError && (
        <p role="alert">{actionFailureSentence(delist.error, "撤回沒有成功，可以再試一次。")}</p>
      )}
    </>
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
          先選擇至少一個要放入 Bundle 的 Skill 版本。
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
      <p className="note">每個 Skill 明確選一個不可變版本；平台不會替你改成最新版本。</p>
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
        <p className="note">還沒有可以選的 Skill——先建立至少一個有版本的 Skill。</p>
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

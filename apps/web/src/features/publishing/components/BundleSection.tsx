import { useId, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { API_BASE_URL, ApiError } from "../../../core/api/client";
import { useEmbeddedSkillDetails, useOwnSkills } from "../../skill";
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

const PLUGIN_SCOPE_NOTE = "Plugin 只含 Agent Skill，不含 MCP 設定或宿主專屬元件。";

interface MemberChoice {
  skillId: string;
  name: string;
  versionId: string;
  versionNumber: number;
  needsAttestation: boolean;
}

function useMemberChoices(): { choices: MemberChoice[]; isPending: boolean } {
  const ownSkills = useOwnSkills();
  const skillIds = useMemo(
    () => ownSkills.data?.skills.map((s) => s.skill_id) ?? [],
    [ownSkills.data],
  );
  const details = useEmbeddedSkillDetails(skillIds);

  const choices: MemberChoice[] = [];
  skillIds.forEach((skillId, i) => {
    const detail = details[i]?.data;
    if (!detail?.version) return;
    const value = detail.redistribution?.value;
    choices.push({
      skillId,
      name: detail.name,
      versionId: detail.version.version_id,
      versionNumber: detail.version.version_number,
      needsAttestation: value === "self_supplied" || value === "generated",
    });
  });

  return { choices, isPending: ownSkills.isPending || details.some((d) => d.isPending) };
}

export function BundleSection() {
  const bundles = useOwnBundles();
  const { choices, isPending: choicesPending } = useMemberChoices();
  const needsAttestationOf = useMemo(() => {
    const map = new Map(choices.map((c) => [c.skillId, c.needsAttestation]));
    return (bundle: BundleVersion) => bundle.members.some((m) => map.get(m.skill_id) === true);
  }, [choices]);

  return (
    <section>
      <h2>Bundle</h2>
      <p className="note" data-role="teaching">
        Bundle 把幾個你自己的 Skill 版本釘成一組，可以匯出成一個 Agent
        Plugin，或以一個公開位址發佈。
      </p>

      {bundles.isPending && <Loading what="Bundle 清單" />}
      <ReadFailure error={bundles.error} what="Bundle 清單" />

      {bundles.data &&
        (bundles.data.bundles.length === 0 ? (
          <p>還沒有建立過任何 Bundle。這裡是空的代表你還沒有建立過，不是清單讀取失敗。</p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {bundles.data.bundles.map((bundle) => (
              <li key={bundle.bundle} className="download-item">
                <BundleRow bundle={bundle} needsAttestation={needsAttestationOf(bundle)} />
              </li>
            ))}
          </ul>
        ))}

      <CreateBundleForm choices={choices} choicesPending={choicesPending} />
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
  const exportBundle = useExportBundle(bundle.bundle);

  return (
    <div>
      <p>
        <strong>{bundle.bundle}</strong> v{bundle.version} — {bundle.description}
      </p>
      <p className="note">
        成員：
        {bundle.members.map((m) => `${m.name} v${m.version_number}`).join("、")}
      </p>
      <p className="note">{PLUGIN_SCOPE_NOTE}</p>
      <p>
        <button
          type="button"
          onClick={() => exportBundle.mutate()}
          disabled={exportBundle.isPending}
        >
          {exportBundle.isPending ? "匯出中…" : "匯出為 Plugin"}
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
          <Link to="/workspace/downloads">到下載紀錄</Link>
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
            publish.mutate({ name: name.trim(), rightsAttested: attested });
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
            {publish.isPending ? "送出中…" : "發佈"}
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
  needsAttestation,
  attested,
  setAttested,
  disabledReason,
  publish,
  delist,
}: {
  publication: Publication;
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
          onClick={() => publish.mutate({ rightsAttested: attested })}
        >
          {publish.isPending ? "送出中…" : "發佈目前的版本"}
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
}: {
  choices: MemberChoice[];
  choicesPending: boolean;
}) {
  const create = useCreateBundleVersion();
  const [name, setName] = useState("");
  const [version, setVersion] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const nameId = useId();
  const versionId = useId();
  const descriptionId = useId();
  const noMembersId = useId();

  function toggle(skillId: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(skillId)) next.delete(skillId);
      else next.add(skillId);
      return next;
    });
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        const memberVersionIds = choices
          .filter((c) => selected.has(c.skillId))
          .map((c) => c.versionId);
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
              setSelected(new Set());
            },
          },
        );
      }}
    >
      <h3>建立 Bundle Version</h3>
      <p>
        <label htmlFor={nameId}>名稱</label>
        <br />
        <input id={nameId} value={name} onChange={(e) => setName(e.target.value)} required />
      </p>
      <p>
        <label htmlFor={versionId}>版本（semver，例如 1.0.0）</label>
        <br />
        <input
          id={versionId}
          value={version}
          onChange={(e) => setVersion(e.target.value)}
          required
        />
      </p>
      <p>
        <label htmlFor={descriptionId}>說明</label>
        <br />
        <textarea
          id={descriptionId}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          required
        />
      </p>
      <fieldset>
        <legend>成員（各取最新版本）</legend>
        {choicesPending && <Loading what="你的 Skill 清單" />}
        {!choicesPending && choices.length === 0 && (
          <p className="note">還沒有可以選的 Skill——先建立至少一個有版本的 Skill。</p>
        )}
        {choices.map((choice) => (
          <p key={choice.skillId}>
            <label>
              <input
                type="checkbox"
                checked={selected.has(choice.skillId)}
                onChange={() => toggle(choice.skillId)}
              />{" "}
              {choice.name}（v{choice.versionNumber}）
            </label>
          </p>
        ))}
      </fieldset>
      <p>
        <button
          type="submit"
          disabled={create.isPending || selected.size === 0}
          aria-describedby={selected.size === 0 ? noMembersId : undefined}
        >
          {create.isPending ? "建立中…" : "建立"}
        </button>
      </p>
      {selected.size === 0 && (
        <p className="note" id={noMembersId}>
          先勾選至少一個要放入的成員 Skill。
        </p>
      )}
      {create.isError && (
        <p role="alert">{actionFailureSentence(create.error, "建立沒有成功，可以再試一次。")}</p>
      )}
    </form>
  );
}

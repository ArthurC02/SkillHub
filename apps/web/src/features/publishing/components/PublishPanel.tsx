import { useId, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { SignInAction } from "../../../shared/ui/SignIn";
import { ApiError } from "../../../core/api/client";
import type { SkillDetail as SkillDetailModel, SkillVersionSummary } from "../../../core/api/types";
import {
  useOwnPublisher,
  useOwnPublication,
  usePublish,
  useDelist,
  type Publication,
} from "../publishing.service";
import { publishGateState, refusalSentence } from "../publishing.model";
import { PublishForm } from "./PublishForm";
import { PublisherRegistration } from "./PublisherSection";
import { VersionCatalogExposure } from "./CatalogExposure";
import { DeliveryAudience } from "./DeliveryAudience";

export function PublishPanel({
  skill,
  version = skill.version,
  isLoggedIn,
  isOwner,
}: {
  skill: SkillDetailModel;
  version?: SkillVersionSummary;
  isLoggedIn: boolean;
  isOwner: boolean;
}) {
  const enabled = isLoggedIn && isOwner;
  const publisher = useOwnPublisher(enabled);
  const publication = useOwnPublication(skill.skill_id, enabled);
  const publish = usePublish(skill.skill_id);
  const delist = useDelist(skill.skill_id);
  const [name, setName] = useState(skill.name);
  const [attested, setAttested] = useState(false);
  const nameInputId = useId();

  if (!isLoggedIn) {
    return (
      <div className="note">
        發佈需要登入，而且只能發佈你自己工作區裡的版本——別人的 Skill 要先 Fork 一份。{" "}
        <SignInAction />
      </div>
    );
  }
  if (!isOwner) return null;
  if (!version) return <p role="status">這個 Skill 還沒有可發佈的版本。</p>;

  const noPublisherYet = publisher.error instanceof ApiError && publisher.error.status === 404;
  const notPublishedYet = publication.error instanceof ApiError && publication.error.status === 404;

  const { needsAttestation, disabledReason, cannotSubmit } = publishGateState(skill, attested);

  return (
    <section>
      <h2>發佈</h2>

      {publisher.isPending && <Loading what="發佈者資訊" />}
      {publisher.error && !noPublisherYet && (
        <ReadFailure error={publisher.error} what="發佈者資訊" />
      )}

      {noPublisherYet ? (
        <>
          <p className="note">
            先在這裡註冊一個永久的發佈者名稱；確認後會回到這一版繼續建立 Publication。
          </p>
          <h3>發佈者名稱</h3>
          <PublisherRegistration />
        </>
      ) : publisher.data ? (
        <>
          {publication.isPending && <Loading what="發佈狀態" />}
          {publication.error && !notPublishedYet && (
            <ReadFailure error={publication.error} what="發佈狀態" />
          )}

          {publication.data ? (
            <PublishedView
              publication={publication.data}
              needsAttestation={needsAttestation}
              attested={attested}
              setAttested={setAttested}
              disabledReason={disabledReason}
              cannotSubmit={cannotSubmit}
              publish={publish}
              delist={delist}
              skillId={skill.skill_id}
              version={version}
            />
          ) : notPublishedYet ? (
            <PublishForm
              nameInputId={nameInputId}
              name={name}
              onNameChange={setName}
              needsAttestation={needsAttestation}
              attested={attested}
              onAttestedChange={setAttested}
              disabledReason={disabledReason}
              cannotSubmit={cannotSubmit}
              publish={publish}
              versionId={version.version_id}
              versionNumber={version.version_number}
            />
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function PublishedView({
  publication,
  needsAttestation,
  attested,
  setAttested,
  disabledReason,
  cannotSubmit,
  publish,
  delist,
  skillId,
  version,
}: {
  publication: Publication;
  needsAttestation: boolean;
  attested: boolean;
  setAttested: (value: boolean) => void;
  disabledReason: string | undefined;
  cannotSubmit: boolean;
  publish: ReturnType<typeof usePublish>;
  delist: ReturnType<typeof useDelist>;
  skillId: string;
  version: SkillVersionSummary;
}) {
  const latest = publication.releases[0];
  const selectedRelease = publication.releases.find(
    (release) => release.version_id === version.version_id,
  );

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
        <Link
          to="/workspace/downloads"
          search={{ publication: `${publication.publisher}/${publication.name}` }}
        >
          在發佈與交付中查看這一筆
        </Link>
      </p>
      <p>
        狀態：{publication.status === "published" ? "已發佈" : "已撤回"}（
        <Timestamp at={publication.status_changed_at} />）
      </p>
      {latest && (
        <p>
          最新 Release：v{latest.version_number}，發佈於 <Timestamp at={latest.released_at} />
        </p>
      )}
      <p className="note">
        v{version.version_number}
        {selectedRelease ? (
          <>
            {" "}
            已有 Release，建立於 <Timestamp at={selectedRelease.released_at} />。
          </>
        ) : (
          " 還沒有 Release。"
        )}
      </p>
      <DeliveryAudience publication={publication} />
      <VersionCatalogExposure skillId={skillId} publication={publication} version={version} />

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
          disabled={publish.isPending || cannotSubmit}
          aria-describedby={disabledReason ? "publish-disabled-reason" : undefined}
          onClick={() =>
            publish.mutate({ versionId: version.version_id, rightsAttested: attested })
          }
        >
          {publish.isPending ? "送出中…" : `發佈 v${version.version_number}`}
        </button>
      </p>
      {disabledReason && (
        <p className="note" id="publish-disabled-reason">
          {disabledReason}
        </p>
      )}
      {publish.isError && (
        <p role="alert">{refusalSentence(publish.error) ?? "發佈沒有成功，可以再試一次。"}</p>
      )}

      {publication.status === "published" && (
        <ConfirmDelete
          scopeId="publish-delist-scope"
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
        <p role="alert">{refusalSentence(delist.error) ?? "撤回沒有成功，可以再試一次。"}</p>
      )}
    </>
  );
}

import { useId, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { SignInAction } from "../../../shared/ui/SignIn";
import { ApiError } from "../../../core/api/client";
import { packagingGate } from "../../packaging";
import type { SkillDetail as SkillDetailModel } from "../../../core/api/types";
import {
  useOwnPublisher,
  useOwnPublication,
  usePublish,
  useDelist,
  type Publication,
} from "../publishing.service";
import { PUBLISHING_REFUSAL_LABEL, refusalSentence } from "../publishing.model";

export function PublishPanel({
  skill,
  isLoggedIn,
  isOwner,
}: {
  skill: SkillDetailModel;
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

  const noPublisherYet = publisher.error instanceof ApiError && publisher.error.status === 404;
  const notPublishedYet = publication.error instanceof ApiError && publication.error.status === 404;

  const redistValue = skill.redistribution?.value;
  const needsAttestation = redistValue === "self_supplied" || redistValue === "generated";
  const gate = packagingGate(skill);
  const blocked: "license_hold" | "not_redistributable" | "license_unknown" | undefined =
    gate === "license_hold" || gate === "not_redistributable" || gate === "license_unknown"
      ? gate
      : undefined;
  const disabledReason = blocked
    ? PUBLISHING_REFUSAL_LABEL[blocked]
    : needsAttestation && !attested
      ? "先勾選下面的聲明才能發佈。"
      : undefined;
  const cannotSubmit = Boolean(disabledReason);

  return (
    <section>
      <h3>發佈</h3>

      {publisher.isPending && <Loading what="發佈者資訊" />}
      {publisher.error && !noPublisherYet && (
        <ReadFailure error={publisher.error} what="發佈者資訊" />
      )}

      {noPublisherYet ? (
        <p className="note">
          要先在<Link to="/workspace/account">帳號頁</Link>註冊一個發佈者名稱，才能發佈這個 Skill。
        </p>
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
            />
          ) : notPublishedYet ? (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                publish.mutate({ name: name.trim(), rightsAttested: attested });
              }}
            >
              <p>
                <label htmlFor={nameInputId}>發佈名稱</label>
                <br />
                <input
                  id={nameInputId}
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  maxLength={64}
                  required
                />
              </p>
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
                disabled={publish.isPending || cannotSubmit}
                aria-describedby={disabledReason ? "publish-disabled-reason" : undefined}
              >
                {publish.isPending ? "送出中…" : "發佈"}
              </button>
              {disabledReason && (
                <p className="note" id="publish-disabled-reason">
                  {disabledReason}
                </p>
              )}
              {publish.isError && (
                <p role="alert">
                  {refusalSentence(publish.error) ?? "發佈沒有成功，可以再試一次。"}
                </p>
              )}
            </form>
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
}: {
  publication: Publication;
  needsAttestation: boolean;
  attested: boolean;
  setAttested: (value: boolean) => void;
  disabledReason: string | undefined;
  cannotSubmit: boolean;
  publish: ReturnType<typeof usePublish>;
  delist: ReturnType<typeof useDelist>;
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
          最新 Release：v{latest.version_number}，發佈於 <Timestamp at={latest.released_at} />
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
          disabled={publish.isPending || cannotSubmit}
          aria-describedby={disabledReason ? "publish-disabled-reason" : undefined}
          onClick={() => publish.mutate({ rightsAttested: attested })}
        >
          {publish.isPending ? "送出中…" : "發佈目前的版本"}
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

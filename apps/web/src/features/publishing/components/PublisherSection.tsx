import { useId, useState } from "react";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ApiError } from "../../../core/api/client";
import { useOwnPublisher, useRegisterPublisher } from "../publishing.service";
import {
  PUBLISHER_NAME_PERMANENT,
  PUBLISHER_NAME_RULE,
  writeFailureSentence,
} from "../publishing.model";

export function PublisherSection() {
  const publisher = useOwnPublisher();
  const headingId = useId();

  const notRegistered = publisher.error instanceof ApiError && publisher.error.status === 404;

  return (
    <section aria-labelledby={headingId}>
      <h2 id={headingId} tabIndex={-1}>
        發佈者名稱
      </h2>

      {publisher.isPending && <Loading what="發佈者名稱" />}
      {publisher.error && !notRegistered && (
        <ReadFailure
          error={publisher.error}
          what="發佈者名稱"
          retry={() => void publisher.refetch()}
          retrying={publisher.isFetching}
          focusTarget={headingId}
        />
      )}

      {publisher.data && !publisher.error ? (
        <>
          <p>
            {publisher.data.name}（註冊於 <Timestamp at={publisher.data.created_at} />）
          </p>
          <p className="note">{PUBLISHER_NAME_PERMANENT}</p>
        </>
      ) : null}
      <PublisherRegistration visible={notRegistered} focusTarget={headingId} />
    </section>
  );
}

export function PublisherRegistration({
  visible,
  focusTarget,
}: {
  visible: boolean;
  focusTarget: string;
}) {
  const register = useRegisterPublisher();
  const [name, setName] = useState("");
  const inputId = useId();

  if (!visible) return null;
  if (register.isSuccess) return <p role="status">已送出註冊，正在確認…</p>;

  return (
    <>
      <p className="note">這個帳號還沒有註冊發佈者名稱。</p>
      <p className="note">{PUBLISHER_NAME_RULE}</p>
      <p className="note">{PUBLISHER_NAME_PERMANENT}</p>
      <form
        className="publisher-form"
        onSubmit={(event) => {
          event.preventDefault();
          register.mutate(name.trim());
          document.getElementById(focusTarget)?.focus();
        }}
      >
        <div className="field">
          <label htmlFor={inputId}>發佈者名稱</label>
          <input
            id={inputId}
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={64}
            required
          />
        </div>
        <button type="submit" disabled={register.isPending}>
          {register.isPending ? "註冊中…" : "註冊"}
        </button>
      </form>
      <ReadFailure error={register.error} what="發佈者名稱">
        <p role="alert">{writeFailureSentence(register.error, "註冊")}</p>
      </ReadFailure>
    </>
  );
}

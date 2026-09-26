import { useId, useState } from "react";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ApiError } from "../../../core/api/client";
import { useOwnPublisher, useRegisterPublisher } from "../publishing.service";
import {
  PUBLISHER_NAME_PERMANENT,
  PUBLISHER_NAME_RULE,
  refusalSentence,
} from "../publishing.model";

export function PublisherSection() {
  const publisher = useOwnPublisher();
  const register = useRegisterPublisher();
  const [name, setName] = useState("");
  const inputId = useId();

  const notRegistered = publisher.error instanceof ApiError && publisher.error.status === 404;

  return (
    <section>
      <h2>發佈者名稱</h2>

      {publisher.isPending && <Loading what="發佈者名稱" />}
      {publisher.error && !notRegistered && (
        <ReadFailure error={publisher.error} what="發佈者名稱" />
      )}

      {publisher.data ? (
        <>
          <p>
            {publisher.data.name}（註冊於 <Timestamp at={publisher.data.created_at} />）
          </p>
          <p className="note">{PUBLISHER_NAME_PERMANENT}</p>
        </>
      ) : notRegistered ? (
        register.isSuccess ? (
          <p role="status">已送出註冊，正在確認…</p>
        ) : (
          <>
            <p className="note">這個帳號還沒有註冊發佈者名稱。</p>
            <p className="note">{PUBLISHER_NAME_RULE}</p>
            <p className="note">{PUBLISHER_NAME_PERMANENT}</p>
            <form
              onSubmit={(event) => {
                event.preventDefault();
                register.mutate(name.trim());
              }}
            >
              <label htmlFor={inputId}>發佈者名稱</label>{" "}
              <input
                id={inputId}
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={64}
                required
              />{" "}
              <button type="submit" disabled={register.isPending}>
                {register.isPending ? "註冊中…" : "註冊"}
              </button>
            </form>
            {register.isError && (
              <p role="alert">
                {refusalSentence(register.error) ?? "註冊沒有成功，可以再試一次。"}
              </p>
            )}
          </>
        )
      ) : null}
    </section>
  );
}

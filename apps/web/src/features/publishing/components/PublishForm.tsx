import type { usePublish } from "../publishing.service";
import { refusalSentence } from "../publishing.model";

export function PublishForm({
  nameInputId,
  name,
  onNameChange,
  needsAttestation,
  attested,
  onAttestedChange,
  disabledReason,
  cannotSubmit,
  publish,
}: {
  nameInputId: string;
  name: string;
  onNameChange: (name: string) => void;
  needsAttestation: boolean;
  attested: boolean;
  onAttestedChange: (attested: boolean) => void;
  disabledReason: string | undefined;
  cannotSubmit: boolean;
  publish: ReturnType<typeof usePublish>;
}) {
  return (
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
          onChange={(event) => onNameChange(event.target.value)}
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
              onChange={(event) => onAttestedChange(event.target.checked)}
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
        <p role="alert">{refusalSentence(publish.error) ?? "發佈沒有成功，可以再試一次。"}</p>
      )}
    </form>
  );
}

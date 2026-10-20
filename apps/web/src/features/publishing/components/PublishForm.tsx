import { useId } from "react";
import type { DeclarableLicense } from "@skillhub/api-client-ts";
import type { usePublish } from "../publishing.service";
import { DECLARABLE_LICENSES, refusalSentence } from "../publishing.model";

export type Declarations = {
  needsAttestation: boolean;
  attested: boolean;
  onAttestedChange: (attested: boolean) => void;
  declaresLicense: boolean;
  license: DeclarableLicense | "";
  onLicenseChange: (license: DeclarableLicense | "") => void;
};

export function PublishDeclarations({ declarations }: { declarations: Declarations }) {
  const {
    needsAttestation,
    attested,
    onAttestedChange,
    declaresLicense,
    license,
    onLicenseChange,
  } = declarations;
  return (
    <>
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
      {declaresLicense && <LicenseDeclaration value={license} onChange={onLicenseChange} />}
    </>
  );
}

function LicenseDeclaration({
  value,
  onChange,
}: {
  value: DeclarableLicense | "";
  onChange: (license: DeclarableLicense | "") => void;
}) {
  const id = useId();
  return (
    <p>
      <label htmlFor={id}>授權條款（選填）</label>
      <br />
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value as DeclarableLicense | "")}
        aria-describedby={`${id}-note`}
      >
        <option value="">不聲明</option>
        {DECLARABLE_LICENSES.map((license) => (
          <option key={license} value={license}>
            {license}
          </option>
        ))}
      </select>
      <br />
      <span className="note" id={`${id}-note`}>
        這個小工具是平台依你的描述寫出來的；選一個授權條款，營運者才能把這一次發佈放進公開目錄。
      </span>
    </p>
  );
}

export function PublishForm({
  nameInputId,
  name,
  onNameChange,
  declarations,
  disabledReason,
  cannotSubmit,
  publish,
  versionId,
  versionNumber,
}: {
  nameInputId: string;
  name: string;
  onNameChange: (name: string) => void;
  declarations: Declarations;
  disabledReason: string | undefined;
  cannotSubmit: boolean;
  publish: ReturnType<typeof usePublish>;
  versionId: string;
  versionNumber: number;
}) {
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        publish.mutate({
          name: name.trim(),
          versionId,
          rightsAttested: declarations.attested,
          declaredLicense: declarations.license,
        });
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
      <PublishDeclarations declarations={declarations} />
      <button
        type="submit"
        disabled={publish.isPending || cannotSubmit}
        aria-describedby={disabledReason ? "publish-disabled-reason" : undefined}
      >
        {publish.isPending ? "送出中…" : `發佈 v${versionNumber}`}
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

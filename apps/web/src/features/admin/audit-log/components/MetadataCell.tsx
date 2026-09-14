import { Fragment } from "react";

export function MetadataCell({ metadata }: { metadata: Record<string, unknown> }) {
  const entries = Object.entries(metadata);
  if (entries.length === 0) return <>不適用</>;
  return (
    <details>
      <summary>{entries.length} 項</summary>
      <dl>
        {entries.map(([key, value]) => (
          <Fragment key={key}>
            <dt>{key}</dt>
            <dd>{typeof value === "string" ? value : JSON.stringify(value)}</dd>
          </Fragment>
        ))}
      </dl>
    </details>
  );
}

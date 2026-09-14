import { ReadFailure } from "../../../shared/ui/LoginRequired";

const messageOf = (error: unknown) => (error instanceof Error ? error.message : String(error));

export function WriteFailure({ error }: { error: unknown }) {
  return (
    <ReadFailure error={error} what="這個動作的結果">
      <p role="alert">沒有完成，伺服器說：{messageOf(error)}</p>
    </ReadFailure>
  );
}

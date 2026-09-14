import { useVersionDiff } from "../evaluation.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Reveal } from "../../../shared/ui/Reveal";

export function VersionDiff({ url }: { url: string }) {
  const diff = useVersionDiff(url);
  if (diff.isPending) return <Loading what="版本差異" />;
  if (diff.error) return <ReadFailure error={diff.error} what="版本差異" />;
  if (diff.data.files.length === 0) return <p>兩個版本的檔案內容相同。</p>;

  return (
    <ul className="file-tree">
      {diff.data.files.map((f) => (
        <li key={f.path}>
          <code>{f.path}</code> · {f.status}
          {f.diff ? (
            <pre className="diff">
              <Reveal text={f.diff} />
            </pre>
          ) : (
            <p className="note">（二進位或過大，不顯示差異）</p>
          )}
        </li>
      ))}
    </ul>
  );
}

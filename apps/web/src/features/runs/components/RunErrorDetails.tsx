type RunErrorRecord = {
  category?: string;
  code?: string;
  message?: string;
};

function errorIdentifier(error: RunErrorRecord): string {
  return [error.category, error.code].filter(Boolean).join("/") || "錯誤代碼未測量";
}

export function RunErrorDetails({ errors }: { errors: RunErrorRecord[] }) {
  return (
    <div>
      <p className="note">這次試跑留有錯誤紀錄。</p>
      <details>
        <summary>查看技術細節</summary>
        <ul>
          {errors.map((error, index) => (
            <li key={`${error.category ?? ""}-${error.code ?? ""}-${index}`}>
              <code>{errorIdentifier(error)}</code> {error.message || "原始訊息未測量"}
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}

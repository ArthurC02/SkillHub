import { lazy, Suspense, useState } from "react";

const FeedbackEntry = lazy(async () => ({
  default: (await import("./FeedbackEntry")).FeedbackEntry,
}));

export function FeedbackLauncher({ pathname }: { pathname: string }) {
  const [requested, setRequested] = useState(false);

  return (
    <details
      className="feedback-entry"
      onToggle={(event) => {
        if (event.currentTarget.open) setRequested(true);
      }}
    >
      <summary>回報問題</summary>
      {requested && (
        <Suspense fallback={<p role="status">正在載入回報表單…</p>}>
          <FeedbackEntry pathname={pathname} embedded />
        </Suspense>
      )}
    </details>
  );
}

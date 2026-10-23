import { useEffect, useRef, type ReactNode } from "react";

export function SettingsResult({
  id,
  verified,
  children,
}: {
  id: string;
  verified: boolean;
  children: ReactNode;
}) {
  const result = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    result.current?.focus();
  }, []);
  return (
    <p
      id={id}
      ref={result}
      tabIndex={-1}
      role="status"
      className={`notice ${verified ? "notice-success" : "notice-warning"}`}
    >
      {children}
    </p>
  );
}

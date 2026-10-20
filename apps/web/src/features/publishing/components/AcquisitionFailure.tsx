import { useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AcquisitionResultUnknown } from "../acquisition.model";
import { actionFailureSentence } from "../publishing.model";

export function AcquisitionFailure({
  error,
  what,
  fallback,
}: {
  error: unknown;
  what: string;
  fallback: string;
}) {
  const message = useRef<HTMLParagraphElement>(null);
  useEffect(() => message.current?.focus(), [error]);
  if (!error) return null;
  return (
    <ReadFailure error={error} what={what}>
      <p role="alert" tabIndex={-1} ref={message}>
        {error instanceof AcquisitionResultUnknown
          ? error.message
          : actionFailureSentence(error, fallback)}
      </p>
      {error instanceof AcquisitionResultUnknown && (
        <p>
          <Link to="/workspace/downloads" target="_blank" rel="noopener noreferrer">
            另開交付紀錄核對
          </Link>
        </p>
      )}
    </ReadFailure>
  );
}

import { useEffect, useRef } from "react";
import { Link } from "@tanstack/react-router";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { focusFeedback } from "../../../shared/ui/focusFeedback";
import { AcquisitionResultUnknown } from "../acquisition.model";
import { actionFailureSentence } from "../publishing.model";

export function AcquisitionFailure({
  error,
  what,
  fallback,
  focusOnError = true,
}: {
  error: unknown;
  what: string;
  fallback: string;
  focusOnError?: boolean;
}) {
  const message = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (focusOnError) focusFeedback(message.current);
  }, [error, focusOnError]);
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

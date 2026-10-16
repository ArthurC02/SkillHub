import { useEffect, useRef, type ReactNode } from "react";
import { useMe } from "../../../core/session/me.service";
import { isTransientReadFailure } from "../../../core/api/client";
import { AdminNav } from "./AdminNav";
import { Loading } from "../../../shared/ui/Loading";
import { PageReadFailure } from "../../../shared/ui/LoginRequired";
import { RouteNotFound } from "../../../shared/ui/RouteNotFound";
import "./AdminPage.css";

export function AdminPage({
  heading,
  lede,
  children,
  headingRef,
}: {
  heading: string;
  lede?: ReactNode;
  children: ReactNode;
  headingRef?: (element: HTMLHeadingElement | null) => void;
}) {
  const me = useMe();
  const pageHeading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    if (me.data?.operator && !me.error && !window.location.hash) pageHeading.current?.focus();
  }, [me.data?.operator, me.error]);
  if (me.isPending) return <Loading what="" />;
  const failure = me.error ? (
    <PageReadFailure
      heading={heading}
      error={me.error}
      what="後台"
      retry={() => void me.refetch()}
      retrying={me.isFetching}
    />
  ) : null;
  if (failure && (me.data?.operator !== true || !isTransientReadFailure(me.error))) return failure;
  if (me.data?.operator !== true) return <RouteNotFound />;
  return (
    <>
      {failure}
      <section hidden={Boolean(failure)}>
        <h1
          ref={(element) => {
            pageHeading.current = element;
            headingRef?.(element);
          }}
          tabIndex={-1}
          className="admin-page-heading"
        >
          {heading}
        </h1>
        {lede && <p className="note">{lede}</p>}
        <AdminNav currentPage={heading} />
        {children}
      </section>
    </>
  );
}

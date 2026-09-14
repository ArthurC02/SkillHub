import type { ReactNode } from "react";
import { useMe } from "../../../core/session/me.service";
import { AdminNav } from "./AdminNav";
import { Loading } from "../../../shared/ui/Loading";
import { RouteNotFound } from "../../../shared/ui/RouteNotFound";

export function AdminPage({
  heading,
  lede,
  children,
}: {
  heading: string;
  lede?: ReactNode;
  children: ReactNode;
}) {
  const me = useMe();
  if (me.isPending) return <Loading what="" />;
  if (me.data?.operator !== true) return <RouteNotFound />;
  return (
    <section>
      <AdminNav />
      <h1>{heading}</h1>
      {lede && <p className="note">{lede}</p>}
      {children}
    </section>
  );
}

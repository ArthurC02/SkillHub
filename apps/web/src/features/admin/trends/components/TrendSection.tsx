import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { DailyCount, Trend } from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { TrendCharts } from "./TrendCharts";

export function TrendSection<B extends DailyCount, T extends Trend<B>>({
  heading,
  query,
  value,
  format,
  labels,
  valueHeading,
  children,
}: {
  heading: string;
  query: UseQueryResult<T>;
  value: (bucket: B) => number;
  format: (n: number) => string;
  labels: Record<string, string>;
  valueHeading?: string;
  children?: ReactNode;
}) {
  return (
    <section>
      <h2>{heading}</h2>
      {query.isPending && <Loading what={heading} />}
      <ReadFailure error={query.error} what={heading}>
        <p role="alert">
          暫時無法讀取{heading}。{query.data && "先前載入的趨勢已隱藏；"}請用上方的按鈕重新整理。
        </p>
      </ReadFailure>
      {query.data && !query.error && (
        <>
          {children}
          <TrendCharts
            trend={query.data}
            value={value}
            format={format}
            labels={labels}
            valueHeading={valueHeading}
          />
        </>
      )}
    </section>
  );
}

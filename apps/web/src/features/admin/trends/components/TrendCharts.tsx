import { BarChart } from "../../components/BarChart";
import { seriesOf, type DailyCount, type Trend } from "../../admin.service";
import "./TrendCharts.css";

const sum = (numbers: number[]) => numbers.reduce((total, n) => total + n, 0);

export function TrendCharts<B extends DailyCount>({
  trend,
  value,
  format,
  labels,
  valueHeading,
}: {
  trend: Trend<B>;
  value: (bucket: B) => number;
  format: (n: number) => string;
  labels: Record<string, string>;
  valueHeading?: string;
}) {
  const { days, series } = seriesOf(trend, value);
  const absent = Object.keys(labels).filter((key) => !series.some((s) => s.key === key));
  return (
    <>
      {series.length > 0 && (
        <div className="chart-grid">
          {series.map((s) => {
            const name = labels[s.key] ?? s.key;
            return (
              <figure key={s.key}>
                <figcaption>
                  {name}：{sum(s.counts)} 筆{valueHeading && `，合計 ${format(sum(s.values))}`}
                </figcaption>
                <BarChart label={name} days={days} values={s.values} format={format} />
                <details>
                  <summary>{name}的逐日數字</summary>
                  <div className="table-scroll">
                    <table>
                      <thead>
                        <tr>
                          <th scope="col">日期（UTC）</th>
                          <th scope="col">筆數</th>
                          {valueHeading && <th scope="col">{valueHeading}</th>}
                        </tr>
                      </thead>
                      <tbody>
                        {days.map((day, index) => (
                          <tr key={day}>
                            <th scope="row">{day}</th>
                            <td>{s.counts[index]}</td>
                            {valueHeading && <td>{format(s.values[index])}</td>}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </details>
              </figure>
            );
          })}
        </div>
      )}
      {absent.length > 0 && (
        <p>這段期間沒有事件：{absent.map((key) => labels[key]).join("、")}。</p>
      )}
    </>
  );
}

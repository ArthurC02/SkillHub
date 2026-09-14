import { useEffect, useRef } from "react";
import { BarController, BarElement, CategoryScale, Chart, LinearScale } from "chart.js";

Chart.register(BarController, BarElement, CategoryScale, LinearScale);

const token = (name: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(name).trim();

export function BarChart({
  label,
  days,
  values,
  format,
}: {
  label: string;
  days: string[];
  values: number[];
  format: (n: number) => string;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const context = canvas.current?.getContext("2d");
    if (!context) return;
    const draw = () =>
      new Chart(context, {
        type: "bar",
        data: {
          labels: days.map((day) => day.slice(5)),
          datasets: [{ label, data: values, backgroundColor: token("--accent") }],
        },
        options: {
          animation: false,
          responsive: true,
          maintainAspectRatio: false,
          scales: {
            x: {
              ticks: { color: token("--text") },
              grid: { display: false },
              border: { color: token("--border") },
            },
            y: {
              beginAtZero: true,
              ticks: {
                color: token("--text"),
                precision: 0,
                callback: (value) => format(Number(value)),
              },
              grid: { color: token("--border") },
              border: { color: token("--border") },
            },
          },
        },
      });
    let chart = draw();
    const scheme = window.matchMedia("(prefers-color-scheme: dark)");
    const redraw = () => {
      chart.destroy();
      chart = draw();
    };
    scheme.addEventListener("change", redraw);
    return () => {
      scheme.removeEventListener("change", redraw);
      chart.destroy();
    };
  }, [label, days, values, format]);

  return (
    <div className="chart-frame">
      <canvas ref={canvas} role="img" aria-label={`${label}：每日長條圖，逐日數字在下方的表`} />
    </div>
  );
}

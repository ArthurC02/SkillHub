import { useDataRetentionPolicy } from "../policy.service";
import { AnalyticsEventsSection } from "./components/AnalyticsEventsSection";
import { DeletionMap } from "./components/DeletionMap";

export function DataPolicy() {
  const policy = useDataRetentionPolicy();

  return (
    <section>
      <h1>資料保存政策</h1>
      <p className="note" data-role="teaching">
        這一頁講兩件事：平台在你沒有主動送出任何東西的情況下記了什麼，以及你要刪掉自己的東西時該去哪裡。
      </p>

      <h2>使用行為分析事件</h2>
      <AnalyticsEventsSection
        isPending={policy.isPending}
        error={policy.error}
        data={policy.data}
      />

      <h2>你的東西怎麼刪</h2>
      <p className="note" data-role="teaching">
        刪除都是分兩步的：按下去之後會先說明這一次刪掉的是什麼、什麼會留下、期限多長，確認才真的執行。
      </p>
      <DeletionMap />

      <h2>這一頁沒有回答的事</h2>
      <p className="note">
        每一類資料各自保存多久（上傳的資料集、Run
        產出、Trace、稽核事件……）仍在核定中，核定前不會寫在這裡當成承諾。 試跑會把你的 Prompt
        與相關內容送往模型供應商，那一段在試跑前的權限確認畫面上逐項列出。
      </p>
    </section>
  );
}

import { useId } from "react";
import type { Labelled } from "../../../core/api/types";
import type { PublicationNote } from "../publishing.service";
import "./DeliveryAudience.css";

export function DeliveryAudience({
  publication,
}: {
  publication: { availability?: Labelled; acquisition?: PublicationNote };
}) {
  const titleId = useId();
  const { availability, acquisition } = publication;

  return (
    <section className="delivery-audience" aria-labelledby={titleId}>
      <h3 id={titleId}>交付對象</h3>
      <dl className="delivery-audience-grid">
        <div>
          <dt>公開頁面</dt>
          <dd>
            <strong>任何人都能閱讀</strong>
            <span className="note">
              知道公開位址的人可以直接閱讀；能否從搜尋與 Catalog 找到，由曝光狀態另外決定。
            </span>
          </dd>
        </div>
        <div>
          <dt>套件取得</dt>
          <dd>
            {availability && acquisition ? (
              <>
                <span>
                  <strong>{acquisition.available ? "目前提供套件" : "目前不提供套件"}</strong>{" "}
                  <span className={acquisition.available ? "badge" : "badge badge-danger"}>
                    {availability.label}
                  </span>
                </span>
                {availability.note && <span className="note">{availability.note}</span>}
                <span className="note">{acquisition.note}</span>
              </>
            ) : (
              <span role="status" className="note">
                交付對象暫時無法確認。公開頁仍會顯示目前是否能取得；請稍後重新整理工作台。
              </span>
            )}
          </dd>
        </div>
      </dl>
    </section>
  );
}

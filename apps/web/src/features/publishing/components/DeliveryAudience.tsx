import { useId } from "react";
import type { Labelled } from "../../../core/api/types";
import type { PublicationNote } from "../publishing.service";
import "./DeliveryAudience.css";

export function DeliveryAudience({
  publication,
  level = 3,
}: {
  publication: {
    availability?: Labelled;
    acquisition?: PublicationNote;
    exposure?: PublicationNote;
  };
  level?: 2 | 3;
}) {
  const titleId = useId();
  const { availability, acquisition, exposure } = publication;
  const Heading = level === 2 ? "h2" : "h3";

  return (
    <section className="delivery-audience" aria-labelledby={titleId}>
      <Heading id={titleId}>{exposure ? "公開與取得" : "交付對象"}</Heading>
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
        {exposure && (
          <div>
            <dt>Catalog 探索</dt>
            <dd>
              <strong>
                {exposure.available ? "目前可從 Catalog 找到" : "目前無法從 Catalog 找到"}
              </strong>
              <span className="note">{exposure.note}</span>
            </dd>
          </div>
        )}
      </dl>
    </section>
  );
}

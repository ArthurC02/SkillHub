import "./RankingExplainer.css";
export function RankingExplainer() {
  return (
    <details className="ranking-explainer">
      <summary>排序依據（為什麼是這個順序？）</summary>
      <ul>
        <li>
          <strong>排的是語意相似度。</strong>
          系統把你輸入的描述和每個 Skill 的索引文字都轉成向量，兩者越接近就排越前面。每個結果標示的
          「相似度」就是這個分數，範圍 0 到 1，越大越接近。
        </li>
        <li>
          <strong>關鍵字命中只用來多找候選，不會改變名次。</strong>
          靠關鍵字被找出來的 Skill，一樣用它自己的語意相似度排序，不會因為字面命中而往前擠。
        </li>
        <li>
          {/* 0.25 must stay in sync with catalog.MaxCosineDistance by hand; the
              search response does not carry the value. */}
          <strong>相似度低於 0.25 的一律不顯示。</strong>
          這個 0.25 是平台目前的設定值，不是介面契約的一部分，調整了這一頁不會自己跟著改。全部都低於
          門檻時會直接說「沒有夠接近的 Skill」，而不是硬給一頁不相關的結果。這個門檻是實測
          出來的：在評測語料上，12 條離題查詢全部被正確拒答，同時 48 條正常查詢一條也沒漏掉。
        </li>
        <li>
          <strong>不看 Star 數、下載數或更新時間。</strong>
          排序完全不使用人氣或新舊，只有相關度。
        </li>
        <li>
          <strong>篩選條件不影響名次。</strong>
          篩選只是把不符合條件的結果整個拿掉，剩下的順序和沒篩之前完全一樣。
        </li>
        <li>
          <strong>例外一：只能用關鍵字比對時。</strong>
          語意服務不可用的話，整頁改用關鍵字分數排序。那個分數不是相似度、沒有固定範圍，上面那個
          門檻也不生效；這時不顯示相似度，改在每個結果旁註明原因。篩選條件在這個狀態下照樣生效。
        </li>
        <li>
          <strong>例外二：還沒建立語意索引的 Skill。</strong>
          這些 Skill
          算不出相似度，只能靠關鍵字被找到，固定排在最後，門檻也沒有判斷過它們；畫面上不顯示
          相似度，改註明原因。
        </li>
      </ul>
    </details>
  );
}

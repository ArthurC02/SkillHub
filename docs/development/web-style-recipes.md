# 前端樣式速用指南

這份短指南回答「新頁面或元件要用哪個現成樣式」。它不是新的設計規格；尺寸、色彩語意與 UX 優先序以[前端設計系統](../design/system.md)為準，樣式所有權與載入順序以[前端架構與樣式分層](../adr/README.md#前端架構與樣式分層)為準。

## 先選形狀，再寫 CSS

| 需要什麼 | 先用什麼 | 別混用 |
| --- | --- | --- |
| 一組各自可辨認、可操作的物件 | `<ul className="card-list">`，每列 `<li className="surface-card">` | 不因為是卡片就借 `.download-item`、`.criterion` 等別的領域名稱；Catalog 商品畫廊有自己的版型，不套通用卡片清單。 |
| 這一頁唯一的主要動作 | 原生 `<button className="action">` 或導頁的 `<a className="action">` | 不把徽章、刪除或第二個按鈕填成主色；導頁仍用連結。 |
| 一般按鈕或長得像按鈕的連結 | 原生 `<button>`；連結用 `.action-secondary` | 不為每個按鈕重寫內距、邊框和 hover。 |
| 需警告的操作 | `.caution`；不可逆操作用 `.destructive` 並保留確認步驟 | 顏色不能代替文字、理由和確認範圍。 |
| 一個狀態主張、分類導覽、補充文字 | `.badge`、`.chip`、`.note` 各司其職 | Badge 是主張，chip 是控制項，note 不是錯誤或成功提示。 |
| 平台對目前頁面的持續訊息 | `.notice`，必要時加 `.notice-danger`／`.notice-warning`／`.notice-success` | 不把短暫操作回饋永遠留在頁上；完成色只給有明文結果的狀態。 |
| 標籤在上、控制項在下的欄位 | `.field`，裡面放 `<label>` 和原生控制項 | 不用新元件重畫瀏覽器已經可及的欄位。 |

例如，一個普通的物件清單不必自造卡片 CSS：

```tsx
<ul className="card-list">
  {items.map((item) => (
    <li className="surface-card" key={item.id}>
      <h3>{item.name}</h3>
      <p>{item.summary}</p>
    </li>
  ))}
</ul>
```

只在這個元件才有的佈局，放在元件旁同名的 `.css`，由 `.tsx` 自己 import；每條選擇器要帶該元件獨有的 class，避免延遲載入後改到別頁。全站共用的基本元素、外框與跨功能配方分別在 `src/styles/base.css`、`layout.css`、`patterns.css`；顏色字面值與 `:root` token 只在 `tokens.css`。先找現有 token，不用 `opacity` 把文字壓淡，也不在元件樣式表另造一個顏色。

## 新元件交付前

1. 對照[逐頁 checklist](../design/system.md#3-評估準則checklist)：第一屏的答案、必要證據、停用原因、未知與空態不能為了整齊而消失。
2. 在桌面、375px 手機、鍵盤與暗色模式操作；檢查焦點、橫向溢出、長文案及等待／失敗／完成狀態。截圖只證明一個瞬間。
3. 跑前端的 `typecheck`、`format:check` 和受影響的測試；改共用樣式至少跑 `src/guards/design-system.test.ts` 與相關瀏覽器流程。看到成功訊息、exit code 和 skipped 數後，才說「通過」。

若現有配方不夠，先問是**哪一個使用者決定**需要不同的形狀。只屬於一個元件的差異留在旁邊；確實跨功能重複後，再由共用樣式層收納，並讓兩個以上的實際呼叫點證明它值得成為新配方。

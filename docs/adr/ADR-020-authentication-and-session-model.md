# ADR-020:身分驗證與 Session 模式(GitHub OAuth＋Postgres Session)

- 狀態:Proposed
- 日期:2026-08-14
- 決策者:產品負責人、架構規劃

## 背景

CORE-005(基本登入、登出與工作區存取控制)是 M1 私有功能(Fork、Workspace、試跑)的前置。ADR-011 已定義授權原則(Workspace Scope、不信任 UI 傳入的 `workspace_id`),但把「身分供應商與服務身分模式」列為待決策。決策驅動因素:

1. 單人開發、E1 自架(ADR-018):每多一個外部 SaaS 依賴或自建密碼系統,都是持續維運成本。
2. 目標使用者是 Agent Skill 個人創作者:Skill 來源主要是 GitHub Repo(PDM-002),此族群 GitHub 帳號覆蓋率接近全員;後續匯入自有 Repo 也能重用同一授權。
3. 資料層是 Postgres 中心(ADR-018):Session 儲存不應為此引入新元件。
4. NFR 要求私有內容授權檢查與速率限制;身分層必須讓 Workspace Scope 推導自伺服器端 Session,而非用戶端宣稱。

## 評估選項

### 選項 A:自建 Email＋密碼

- 優點:無外部依賴,離線可測。
- 缺點:密碼雜湊、重設信、防爆破、洩漏監控全是自建責任;需要寄信服務(新外部依賴);對目標族群反而是較高摩擦的註冊路徑。

### 選項 B:外部身分 SaaS(Auth0、Clerk 類)

- 優點:功能最全(MFA、多 IdP、防濫用)。
- 缺點:與 ADR-018 自架方向相悖;免費層綁定供應商 UI 與定價;使用者資料出境到第三方,增加合規面。MVP 只需「登入」,採購整套 IdP 是過度配置。

### 選項 C:GitHub OAuth＋自管 Postgres Session(採用)

- 優點:無密碼儲存、無重設流程;身分供應商就是內容供應鏈的所在地;Session 落在既有 Postgres,零新元件;OAuth flow 用標準庫即可實作。
- 缺點:綁定單一 IdP,GitHub 停擺時無法登入(可接受:唯讀瀏覽本就不需登入,DISC-010);無 GitHub 帳號的使用者被排除(對目標族群比例極低,訊號出現再加第二個 Provider)。

## 決策

### 身分供應商

- MVP 唯一登入方式為 **GitHub OAuth(Authorization Code flow)**。
- 身分與帳號分離:`user_identities (provider, provider_user_id) → user_id`,新增 Provider(Google 等)只是加一列,不動 `users`。
- 首次登入自動建立 User 與個人 Workspace(ADR-011 的 1:1 個人工作區),同交易完成。
- Email 取自 GitHub primary verified email,僅作顯示與通知用;帳號比對鍵是 `provider_user_id`,不是 Email(Email 可變、可重複註冊)。

### Session 模式

- **伺服器端 Session,存 Postgres `sessions` 表**;不用 JWT——撤銷(登出、帳號刪除)必須立即生效,而 JWT 的撤銷清單就是一張 Session 表,不如直接用表。
- Token 為 256-bit 隨機值,DB 只存 SHA-256 雜湊;DB 外洩不等於 Session 劫持。
- Cookie:`HttpOnly`、`Secure`、`SameSite=Lax`、`Path=/`。
- 有效期:固定 30 天絕對過期,不做滑動續期(一個 `expires_at` 欄位即可;使用者留存訊號出現再議)。
- CSRF:`SameSite=Lax`＋狀態變更端點僅收 JSON body(非表單 content-type);OAuth `state` 以一次性 Cookie 驗證。
- 過期 Session 由既有背景清理路徑批次刪除(與 ADR-008 清理慣例一致,冪等)。

### 授權推導

- 請求的 `user_id` 與個人 `workspace_id` 一律由 Session 在伺服器端解析(ADR-011:不信任 UI 傳入值)。
- 公開讀取(搜尋、Skill 詳情)不要求 Session(DISC-010);私有路徑一律經 Session middleware。

### 部署形態:Auth 不獨立成服務

- 身分解析是每個私有請求的熱路徑,留在控制平面單體內(ADR-010 拆分準則逐項檢驗均不成立:拆出後 secret 仍在同一信任區、熱路徑多一個網路 hop、`users`/`sessions` 資料所有權被切開)。`internal/identity` 的套件邊界即未來拆分接縫。
- **離線 Demo 由 Provider 抽換承載,不由服務拆分承載**:拆出去的 Auth 服務跑 GitHub OAuth 一樣要連外網。伺服器以 `DEV_LOGIN=1` 啟動時開啟 `POST /auth/dev/login`(`dev` Provider),不出外網即可登入;生產環境不設此變數,路由不存在。此即 ADR-010 Local Development「Fake Provider」的落地。
- 多 Provider 路徑:LDAP/AD 直連＝新增 provider 值與一個憑證驗證入口,同套件、同資料模型;企業聯邦需求(對接客戶 IdP 的 SAML/OIDC、SCIM、群組同步)出現時,評估自架 Zitadel/Ory 類,不自寫。

### 服務身分(非人類主體)

- Go→Python 內部呼叫:僅限私網,附靜態 Bearer Token(環境變數注入),Python 側驗證。E1 單機 compose 網路下這已是信任邊界的全部;拆分部署時再升級 mTLS/服務網格。
- 模型出口沿用 ADR-017 的每 Run 短效 LiteLLM Virtual Key;Sandbox 的短效物件授權沿用 ADR-001,均不走使用者 Session。

## 影響

### 正面

- 零新基礎設施元件、零密碼責任;登入路徑與內容供應鏈(GitHub)重合。
- 撤銷即時:登出、封鎖、帳號刪除都是刪 Session 列。
- `user_identities` 讓多 IdP 成為資料問題而非架構問題。

### 成本與限制

- 需維護 GitHub OAuth App 憑證(Client Secret 屬 ADR-017 同級的閘道機密管理)。
- 本地開發需一組 OAuth App(或以假 Provider stub 測試);E2E 測試需處理外部依賴。
- 單 IdP 依賴:GitHub 帳號被停用等同無法登入本站。

## 待決策

- 第二身分供應商(Google)的啟用訊號與時點。
- 封閉測試(BETA-001)是否需要邀請碼/白名單閘門疊在 OAuth 之上。
- Local Runner 配對憑證是否重用 Session 機制(後 MVP,ADR-006)。

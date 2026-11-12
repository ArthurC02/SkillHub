# Run 的公開資料搜尋

`02:GEN-014`／`03:GEN-025` 要求 Skill 宣告、試跑前確認、經模型閘道搜尋，以及查詢、來源、費用進入私人 Trace。目前已接通宣告、確認、Run 政策、runtime 能力讀取與閘道授權，並有免費協定驗證；runtime 搜尋工具與 Trace 尚待交付，不能將這份局部證據當作功能完成。

## 宣告與確認

Skill 的 `compatibility` 必須以獨立一行寫出 `skillhub-web-search`；前後空白可省略，敘述中的提及、不同大小寫與其他別名都不算。Run 只讀取指定不可變版本保存的宣告，不執行套件內容。

單次生成與互動創作共用同一條提示規則：只有具體任務步驟需要在執行時取得公開事實才宣告，保留其他環境需求。參考 Skill、一般補 Context 清單與創作時已抓回的網頁都不構成宣告理由。本文教執行端使用 `mcp__skillhub__web_search`、核對來源相關性與日期、引用網址，證據不足或工具不可用時明說未知並補問；搜尋結果仍是不受信任資料。創作端不呼叫這個工具，不寫入 `allowed_tools`、新增 MCP Server、要求供應商金鑰或改用網路腳本繞過限制。

Python 與 Go 不從敘述或參考自行添加宣告。免費跨語言測試使用真正的 Python 服務與模型替身，核對單次生成和互動創作的回覆、Go adapter、套件 frontmatter、不可變版本與權限揭露。這證明提示設定及宣告傳遞；模型是否對每項任務選對宣告、來源與答案，仍待真實品質量測。

試跑前的權限摘要會列出搜尋、送往搜尋供應商的查詢、私人 Trace 的來源與費用，以及本次 Run 預算。這些文字納入既有摘要雜湊，必須先留下該摘要的確認紀錄才能建立 Run。需求保存於不可變政策快照；舊快照缺少欄位時不補授權。不載入 Skill 的對照 Run 保留相同 runtime 要求，但不取得搜尋工具。

Provider 的 runtime 項目以 `web_search` 回報 `supported`、`unsupported` 或 `unavailable`；支援必須涵蓋該項目中的每個版本。缺席視為不支援；讀不到證據與未知值暫不派工。平台與 Sandbox 都會核對相同版本及接法，前面的不合格項目不會遮住後面的合格項目。一般 Run 不依賴搜尋能力。

## 實際 runtime 能力

Sandbox 不採信設定檔手動填入的搜尋支援。能力讀取將設定中的版本拆開，只有 Driver 讀到的實際 runtime 與版本一致時才列為支援；沒有 reader 的 Driver 不宣告支援。新搜尋請求在建立工作負載前重讀證據；同一請求重送先回傳原有 Run，內容不同仍回衝突，不因後來的能力讀取失敗而重建工作負載。

Docker Driver 以有逾時的映像檢查讀取 `io.skillhub.runtime.web-search=1`、`SKILLHUB_RUNTIME`、`SKILLHUB_RUNTIME_VERSION` 與映像 ID。搜尋派工再次核對並使用該 ID 建立容器，避免標籤在核對後換到其他映像。缺標記代表不支援；標記存在但版本或 ID 不完整代表證據不可讀。

Local Driver 只對設定的受信任 runner 執行 `--capabilities`，不執行 Skill 套件。runner 須回傳 `runtime`、`version` 與明確布林值 `web_search` 的單一 JSON；輸出上限 4096 bytes、逾時 5 秒，接受呼叫端取消，不繼承模型金鑰、搜尋授權或 `NODE_OPTIONS`。新搜尋工作在準備輸入前再次核對。兩種 Driver 都只在固定授權成立時將 `SKILLHUB_SEARCH_TOOL` 傳給工作負載，普通 Run 建立工作負載時不做搜尋探詢。

預設 runtime 映像仍不支援搜尋；映像標記與 runner 能力回覆必須隨真正的搜尋工具一起交付，不能只補標記或修改設定便當成功能完成。映像發佈、預設映像切換與節點部署另行驗收。

## 閘道邊界

固定別名為 `skillhub-web-search`，Provider 契約的選填 `ModelGatewayGrant.search_tool` 缺席代表沒有搜尋授權。Go 只在已確認政策、版本宣告、模型閘道出口與載入 Skill 四項條件都成立時授予該別名，沿用同一把短效金鑰、模型範圍、預算、到期時間與撤銷方式。未宣告的 Run、Creation 與平台 Agent 的金鑰明確不授予搜尋工具。

閘道設定開啟 `search_tool_deny_by_default`；搜尋工具名稱必須同時通過金鑰與所屬團隊的授權。新建 Run 團隊只列入固定別名，既有團隊的權限由部署負責人明確設定，應用不改寫它；團隊建立失敗仍沿用既有的最佳努力政策，金鑰自身的限制仍在。

Responses、Chat 與 Messages 的原生搜尋工具由閘道攔截，仍須通過固定別名的授權；`web_search_options` 不直接送往模型供應商。攔截的供應商須涵蓋全部已設定的非 embedding 模型，每條路由也須移除原生參數；新增供應商或路由後需重新驗證。

搜尋服務的註冊與供應商金鑰只存在閘道。本批不選定或啟用正式搜尋供應商；別名尚未註冊或後端失敗不能回報搜尋成功。端點、權限與費用標頭以 [LiteLLM Search API](https://docs.litellm.ai/docs/search/) 為參考，實際支援以 Repo 釘住的映像驗證為準。

## 免費驗證

在 repo 根目錄執行：

```powershell
go -C tools/devctl run . gateway-search-proof
```

需要 Docker 與 Go。工具從 Compose 讀取已釘住的閘道與資料庫映像，在獨立內部網路建立資料庫、閘道與 Go 搜尋／模型替身，使用真正的 Go 簽發器；不連接付費搜尋或模型供應商。YAML 轉 JSON 使用該閘道映像已內建的解析器，主機不需安裝 Python。結束時只清除自己建立的容器、匿名磁碟、網路與兩個暫存執行檔，證據留在輸出的 scratchpad 目錄。

驗證核對團隊拒絕、正確別名、其他別名、未授權的三種入口、預算耗盡、撤銷後拒絕、來源網址、閘道費用標頭與同一 attempt 的實際費用紀錄。模型替身會實際提出搜尋工具呼叫；未授權的原生搜尋不可到達搜尋後端或直接轉給模型供應商，普通模型呼叫與 function 工具仍可成功。另核對所有已設定模型的供應商與參數保護。CI 的 `gateway-search` 執行相同驗證並保存證據。

原生探針依各入口的合法格式：Responses 測 `web_search` 與 `web_search_preview`，Chat 測 `web_search_options`，Messages 測其搜尋工具；空的 guardrails 清單也不能擴大權限。[Chat API 契約](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) 的 `tools` 只接受 function／custom，不把送錯入口的無效工具當成成功搜尋或授權證據。

這些是協定與授權證據。宣告／確認流程另有真實 PostgreSQL 測試，涵蓋不支援的 runtime、知道雜湊卻未確認、錯誤雜湊、政策保存與派工授權，以及未宣告和不載入 Skill 的拒授權案例。Driver 測試另涵蓋能力漂移、版本隔離、未知證據、重送、映像 ID、runner 輸出邊界與金鑰隔離；仍不替代搜尋工具、私人 Trace、真實搜尋品質、部署出口控制及模型品質驗收。

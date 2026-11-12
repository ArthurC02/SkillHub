# Run 的公開資料搜尋

`02:GEN-014`／`03:GEN-025` 要求 Skill 宣告、試跑前確認、經模型閘道搜尋，以及查詢、來源、費用進入私人 Trace。目前已接通宣告、確認、Run 政策與閘道授權，並有免費協定驗證；runtime 的實際能力讀取、搜尋工具與 Trace 尚待接線，不能將這份局部證據當作功能完成。

## 宣告與確認

Skill 的 `compatibility` 必須以獨立一行寫出 `skillhub-web-search`；前後空白可省略，敘述中的提及、不同大小寫與其他別名都不算。Run 只讀取指定不可變版本保存的宣告，不執行套件內容。

試跑前的權限摘要會列出搜尋、送往搜尋供應商的查詢、私人 Trace 的來源與費用，以及本次 Run 預算。這些文字納入既有摘要雜湊，必須先留下該摘要的確認紀錄才能建立 Run。需求保存於不可變政策快照；舊快照缺少欄位時不補授權。不載入 Skill 的對照 Run 保留相同 runtime 要求，但不取得搜尋工具。

Provider 的 runtime 項目以 `web_search` 回報 `supported`、`unsupported` 或 `unavailable`；支援必須涵蓋該項目中的每個版本。缺席視為不支援；讀不到證據與未知值暫不派工。平台與 Sandbox 都會核對相同版本及接法，前面的不合格項目不會遮住後面的合格項目。一般 Run 不依賴搜尋能力。實際映像與 runner 的能力讀取尚未接通，不能只靠手動填入 `supported` 當作 runtime 驗收；目前預設 runtime 不宣告支援。

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

這些是協定與授權證據。宣告／確認流程另有真實 PostgreSQL 測試，涵蓋不支援的 runtime、知道雜湊卻未確認、錯誤雜湊、政策保存與派工授權，以及未宣告和不載入 Skill 的拒授權案例；仍不替代 runtime 實際能力與工具、私人 Trace、真實搜尋品質、部署出口控制及模型品質驗收。

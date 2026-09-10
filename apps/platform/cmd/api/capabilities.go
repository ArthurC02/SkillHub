package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

func creationCapability(clean bool) envx.Capability {

	needs := []string{"CREATION_EXPOSED", "CREATION_LIMITS_JSON"}
	without := "刻意的狀態：/creation-sessions* 不掛載、GET /me 不列 creation_skill；LIMITS 缺任何一鍵時 API 拒絕開始會話（Limits.Valid）"
	if !clean {
		needs = append(needs, "CREATION_WORKER_INTERNAL_ADDR", "CREATION_WORKER_INTERNAL_URL", "CREATION_WORKER_INTERNAL_TOKEN")
		without += "，Worker 的內部 listener 不啟動、流程圖沒有地方送"
	}
	return envx.Capability{
		ID:      "interactive_creation",
		Name:    "互動創作會話（ADR-067）",
		Needs:   needs,
		Without: without,
		Fix: "值照 05 R-45 的裁定表（.env.example 帶著同一行 JSON）；CREATION_EXPOSED 與 GENERATE_SKILL_EXPOSED 一樣，" +
			"在 01 §10 的 M5 邊界解除前不要設成 on。淨測試模式不需要那三個 Worker 內部變數：" +
			"創作 worker 就跑在同一個行程裡（ADR-060 決策 6）",
	}
}

func capabilityTable(pool *pgxpool.Pool, packagingTargets int, servesWeb bool) *envx.Registry {
	client := &http.Client{Timeout: 2 * time.Second}
	caps := []envx.Capability{
		{
			ID:      "catalogue_search",
			Name:    "目錄搜尋（關鍵字）",
			Needs:   []string{"DATABASE_URL"},
			Without: "整個平台起不來",
			Fix:     "指向一個 PostgreSQL",
			Probe: func(ctx context.Context) error {
				if pool == nil {
					return errors.New("這個行程沒有資料庫連線池")
				}
				return pool.Ping(ctx)
			},
		},
		{
			ID:      "intent_search",
			Name:    "跨語言意圖搜尋、匯入增強、推薦理由",
			Needs:   []string{"LLM_SERVICE_URL", "LLM_SERVICE_TOKEN"},
			Without: "只剩 FTS：非英語的查詢多半回 0 筆（畫面會標 degraded）",
			Fix:     "另起 apps/llm 再設成它的位址與服務憑證，**並且要在 seed 之前**——種進去的內容不會事後補索引",
			Probe:   probeLLMService(client),
		},
		{
			ID:      "evaluation_judge",
			Name:    "評估判定（Judge）",
			Needs:   []string{"LLM_SERVICE_URL", "LLM_SERVICE_TOKEN"},
			Without: "Run 跑得完，但判定是 undetermined：沒有 judge 服務",
			Fix:     "同上，judge 在 Python 側",
			Probe:   probeLLMService(client),
		},
		{
			ID:    "run_dispatch",
			Name:  "試跑（模型出口）",
			Needs: []string{"SKILLHUB_MODEL_GATEWAY_URL", "SKILLHUB_MODEL_GATEWAY_KEY", "SKILLHUB_RUN_MODEL"},
			Without: "沒有模型出路，平台會擋下派送並說明（05 R-35）；" +
				"設了閘道卻沒設 SKILLHUB_RUN_MODEL 的話，每個 Run 會在一分鐘後死在 400 Invalid model name",
			Fix:   "起 infra/compose 的 litellm，URL 指向它，KEY 用該部署的 Virtual Key，MODEL 用它服務的模型名",
			Probe: probeModelGateway(client),
		},
		{
			ID:      "packaging_download",
			Name:    "打包下載",
			Needs:   []string{"DOWNLOAD_ARTIFACT_RETENTION"},
			Without: "打包一律 503",
			Fix:     "這個值刻意沒有預設——它是一句對使用者的保存期承諾，不是參數（GOV-RETENTION-001）",

			Probe: func(context.Context) error {
				if packagingTargets == 0 {
					return errors.New(
						"沒有載入任何打包目標（PACKAGING_PROFILES_DIR 是相對路徑，" +
							"從 repo 根目錄以外的地方啟動就會是空的）；打包路由一律 503")
				}
				return nil
			},
		},
		{
			ID:      "redistribution_release",
			Name:    "可散布性放行（operator）",
			Needs:   []string{"OPERATOR_USER_IDS"},
			Without: "沒有人能放行，所以每一個 Skill 的打包都會 422（04 丙-105）",
			Fix:     "設成要當 operator 的 user id（逗號分隔）",
		},
		{
			ID:      "funnel_analytics",
			Name:    "漏斗量測",
			Needs:   []string{"ANALYTICS_RETENTION"},
			Without: "不收集任何漏斗事件",
			Fix:     "設一個保存期，例如 4320h",
		},
		{
			ID:    "credit_pricing",
			Name:  "Credit 計價與開始門檻（ADR-068）",
			Needs: []string{"CREDIT_USD_PER_CREDIT", "CREDIT_MARKUP_BPS", "CREDIT_DEBT_FLOOR", "CREDIT_MIN_START_FALLBACK"},

			Without: "帳本照跑，但按 ADR-068 的預設值計價：1 credit = US$0.001、加成 1.3 倍、負債下限 −50 credit、" +
				"樣本不足時的開始門檻 70 credit。" +
				"CREDIT_MIN_START_FALLBACK 設得比封測發放額還高，拿到點數的人一樣開不了新創作；設成 0 則閘門①在量到 p95 之前形同不存在",
			Fix: "只有在這個部署的真實成本或加成與 ADR-068 不同時才設；" +
				"改動只影響之後寫入的 credit_entries（每一筆都記下當時的 markup_bps，舊帳不回頭改寫）",
		},
		{
			ID:      "github_login",
			Name:    "GitHub OAuth 登入",
			Needs:   []string{"GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET", "OAUTH_REDIRECT_URL"},
			Without: "沒有人能用 GitHub 帳號登入——除非 DEV_LOGIN=1 開著，這個部署完全沒有登入方式",
			Fix:     "在 GitHub 建一個 OAuth App，把用戶端 ID、密鑰與導回網址填進這三個變數",
		},
		{
			ID:    "dev_login",
			Name:  "離線登入（開發／clean test mode）",
			Needs: []string{"DEV_LOGIN"},
			Without: "沒有離線登入（ADR-020）：只能用 GitHub OAuth 登入——GITHUB_CLIENT_ID 等三個變數沒填的話，" +
				"這個部署完全登不進去",
			Fix: "設 DEV_LOGIN=1 供本機或 clean test mode 使用；同時要 COOKIE_INSECURE=1，" +
				"否則行程會在啟動時拒絕（DEV_LOGIN=1 與安全 Cookie 互相矛盾，見 devLoginRefusal）——" +
				"正式環境兩者都不可開，DEV_LOGIN=1 代表任何名字都能免密碼登入",
		},
		{
			ID:      "beta_gate",
			Name:    "封測名單管制（BETA-001 admission）",
			Needs:   []string{"BETA_ALLOWLIST"},
			Without: "沒有名單管制：任何登入的 GitHub 帳號都能 Fork／試跑／下載",
			Fix:     "填入允許的 provider_user_id（逗號分隔）",
		},
		{
			ID:    "object_store",
			Name:  "物件儲存（套件、產物、Trace 附件）",
			Needs: []string{"OBJSTORE_ENDPOINT", "OBJSTORE_ACCESS_KEY", "OBJSTORE_SECRET_KEY", "OBJSTORE_BUCKET"},
			Without: "ENDPOINT 與 BUCKET 有本機預設值（localhost:8333／skillhub），ACCESS_KEY／SECRET_KEY 留空代表" +
				"匿名存取——只在本機 SeaweedFS 這樣設。正式環境沒有指到真正的物件儲存或金鑰不對時，" +
				"EnsureBucket 會在啟動時失敗，整個行程結束",
			Fix: "指向一個 S3 相容服務（本機用 SeaweedFS）並帶入它的存取金鑰",
		},
		{
			ID:    "generation_entry",
			Name:  "M5 生成入口（ADR-052）",
			Needs: []string{"GENERATE_SKILL_EXPOSED"},

			Without: "刻意的狀態：POST /skills/generate 不掛載、GET /me 不列 generate_skill，畫面不畫出生成入口",
			Fix: "不要在 01 §11.2 第一段漏斗量到讀數之前設成 on——這是 M5 對封測使用者的曝光邊界（01 §10），" +
				"不是一個等著被打開的功能",
		},
		creationCapability(servesWeb),
	}
	if servesWeb {
		caps = append(caps, envx.Capability{
			ID:   "web_app",
			Name: "網頁介面（這個行程送出的 SPA）",

			Without: "index.html 送得出去，但它引用的 JavaScript 不在——瀏覽器拿到一個空白頁，" +
				"伺服器這邊每一條路由都還是 200",
			Fix: "重新 `task build:web`，然後**重啟這個行程**：index.html 在啟動時就讀進記憶體並烙上旗標，" +
				"而 asset 檔名帶 build hash，重建卻不重啟就會指向一個已經不存在的檔案",
			Probe: func(context.Context) error {
				distDir, err := webDistDir()
				if err != nil {
					return err
				}
				return probeWebAssetsUnder(distDir)
			},
		})
	}
	return envx.NewRegistry(caps)
}

var assetRef = regexp.MustCompile(`/assets/[A-Za-z0-9._-]+`)

func probeWebAssetsUnder(distDir string) error {
	index, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		return fmt.Errorf("讀不到這個行程要送出的 index.html：%w", err)
	}
	refs := assetRef.FindAllString(string(index), -1)
	if len(refs) == 0 {
		return errors.New("index.html 沒有引用任何 /assets/ 檔案——這不是一個 production build")
	}
	for _, ref := range refs {
		info, err := os.Stat(filepath.Join(distDir, filepath.FromSlash(strings.TrimPrefix(ref, "/"))))
		if err != nil {
			return fmt.Errorf("index.html 指向 %s，但那個檔案不在這個 build 裡（重建之後沒有重啟？）", ref)
		}
		if info.Size() == 0 {
			return fmt.Errorf("index.html 指向的 %s 是空的", ref)
		}
	}
	return nil
}

func probeLLMService(client *http.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		base := strings.TrimRight(os.Getenv("LLM_SERVICE_URL"), "/")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/readyz", nil)
		if err != nil {
			return fmt.Errorf("LLM_SERVICE_URL 不是一個可用的位址：%w", err)
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("LLM_SERVICE_TOKEN"))
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("連不上 apps/llm：%v", redactURL(err))
		}
		defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
		switch resp.StatusCode {
		case http.StatusOK:
		case http.StatusUnauthorized:

			return errors.New("apps/llm 拒絕了這個部署的服務憑證：兩邊的 LLM_SERVICE_TOKEN 不一樣")
		case http.StatusServiceUnavailable:
			return errors.New("apps/llm 自己沒有設定服務憑證（它會對每一個能力端點回 503）")
		default:
			return fmt.Errorf("apps/llm 的 /readyz 回 %d", resp.StatusCode)
		}
		var body struct {
			Status  string   `json:"status"`
			Missing []string `json:"missing"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fmt.Errorf("apps/llm 的 /readyz 回了讀不懂的內容：%w", err)
		}
		if body.Status != "ready" {
			return fmt.Errorf("apps/llm 起來了但還不能用，它自己缺：%s", strings.Join(body.Missing, "、"))
		}
		return nil
	}
}

func probeModelGateway(client *http.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		base := strings.TrimRight(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/")
		want := os.Getenv("SKILLHUB_RUN_MODEL")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
		if err != nil {
			return fmt.Errorf("SKILLHUB_MODEL_GATEWAY_URL 不是一個可用的位址：%w", err)
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY"))
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("連不上模型閘道：%v", redactURL(err))
		}
		defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return errors.New("模型閘道拒絕了 SKILLHUB_MODEL_GATEWAY_KEY")
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("模型閘道的 /models 回 %d", resp.StatusCode)
		}
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fmt.Errorf("模型閘道的 /models 回了讀不懂的內容：%w", err)
		}
		served := make([]string, 0, len(body.Data))
		for _, m := range body.Data {
			if m.ID == want {
				return nil
			}
			served = append(served, m.ID)
		}

		return fmt.Errorf("閘道沒有服務 SKILLHUB_RUN_MODEL=%q，每個 Run 都會死在 400 Invalid model name；它服務的是：%s",
			want, strings.Join(served, "、"))
	}
}

func redactURL(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return err.Error()
}

func reportCapabilities(ctx context.Context, reg *envx.Registry) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows := reg.Report(ctx, os.Getenv)
	slog.Info("這個部署現在有什麼、缺什麼（GET /readyz 是同一張表）")
	for _, s := range rows {
		switch s.Readiness {
		case envx.Ready:
			slog.Info("  ✓ 量到了，可以用", "能力", s.Name, "耗時", s.MeasuredFor)
		case envx.Unmeasured:

			slog.Info("  ? 前提齊全，但沒有人量過它", "能力", s.Name)
		case envx.Broken:
			slog.Warn("  ✗ 前提齊全，但量到它壞的", "能力", s.Name, "原因", s.Detail, "沒有它會怎樣", s.Without)
		case envx.Unavailable:
			slog.Warn("  ✗ 缺前提", "能力", s.Name, "缺", strings.Join(s.Missing, "、"),
				"沒有它會怎樣", s.Without, "怎麼補", s.Fix)
		}
	}
}

func printCapabilitiesJSON(w io.Writer) error {
	type row struct {
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		Needs []string `json:"needs"`
	}

	reg := capabilityTable(nil, 0, false)
	out := make([]row, 0, len(reg.Capabilities()))
	for _, c := range reg.Capabilities() {
		out = append(out, row{ID: c.ID, Name: c.Name, Needs: c.Needs})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

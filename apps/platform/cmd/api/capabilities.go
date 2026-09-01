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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

// This deployment's capability table (05 R-36 第二段).
//
// It lives here rather than in the launcher because of the hard condition R-36
// wrote: when this exists, the launcher reads THIS answer. Two lists of the same
// preconditions is the drift this repository keeps finding, and the launcher's
// copy was the one that could only guess — it owns no process it could ask.
//
// # What a probe is for, and the trap it must not repeat
//
// A probe measures the thing that actually breaks. It is not a liveness check
// wearing a different name: the gateway probe asks whether the model this
// deployment is configured to run is one the gateway serves, because that is the
// failure R-36's first段 already blocked at the launcher — a gateway that is up
// and a SKILLHUB_RUN_MODEL it does not serve kills every run a minute later with
// `400 Invalid model name`. Asking the gateway's liveness endpoint instead would
// be committing the defect this whole table exists to remove.
//
// Same reasoning for apps/llm: the probe calls its /readyz, which sits behind
// the service token, so one request measures reachability, credential match and
// the service's own configuration. On 2026-09-01 a restart without that
// credential produced a service that answered /healthz 200 and could do none of
// its four jobs (04 丙-118).
//
// Every probe here is free and read-only: /readyz is polled.
//
// packagingTargets is how many profiles LoadProfiles actually returned. It is a
// count and not a bool because zero is the only interesting value and the reader
// of a broken row deserves to know it was zero rather than "some".
func capabilityTable(pool *pgxpool.Pool, packagingTargets int) *envx.Registry {
	client := &http.Client{Timeout: 2 * time.Second}
	return envx.NewRegistry([]envx.Capability{
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
			// The retention value is a promise about how long an artifact lives; it
			// says nothing about whether one can be produced. The thing that actually
			// stops packaging is an empty profile directory, and PACKAGING_PROFILES_DIR
			// defaults to a RELATIVE path — so running the binary from anywhere but the
			// repository root loads zero targets and every PACK-001 route answers 503
			// (04 丙-102 ③).
			//
			// Measured on 2026-09-02 by exploratory testing: with zero profiles and
			// with three, this row read `unmeasured` both times. A row that is identical
			// whether the capability works or is completely dead carries no information,
			// which is the exact defect this table was built to remove — and this file's
			// own header says so: 「A probe measures the thing that actually breaks.」
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
	})
}

// probeLLMService asks apps/llm whether it can work, with this deployment's own
// credential. See capabilityTable's comment for why it is that endpoint.
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
			// The 2026-09-01 accident, named exactly: the service is reachable
			// and its credential is not this one's.
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

// probeModelGateway asks the gateway which models it serves and looks for the
// one this deployment is configured to run.
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
		// The failure R-36's first段 blocked at the launcher, now measured
		// rather than inferred from two variables both being non-empty.
		return fmt.Errorf("閘道沒有服務 SKILLHUB_RUN_MODEL=%q，每個 Run 都會死在 400 Invalid model name；它服務的是：%s",
			want, strings.Join(served, "、"))
	}
}

// redactURL keeps an address out of an error a probe puts on an
// unauthenticated endpoint. Go's transport wraps every failure in *url.Error,
// which carries the full request URL; the cause underneath it is the part a
// reader needs ("connection refused"), and the address is not ours to publish.
func redactURL(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return err.Error()
}

// reportCapabilities prints the table at boot. On the machine clean test mode
// exists for, this print is the only diagnostic the person in front of it gets,
// so every row that is not Ready says what a user will meet and how to fix it.
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
			// Printed differently from ✓ on purpose. This is the state that used
			// to be a tick.
			slog.Info("  ? 前提齊全，但沒有人量過它", "能力", s.Name)
		case envx.Broken:
			slog.Warn("  ✗ 前提齊全，但量到它壞的", "能力", s.Name, "原因", s.Detail, "沒有它會怎樣", s.Without)
		case envx.Unavailable:
			slog.Warn("  ✗ 缺前提", "能力", s.Name, "缺", strings.Join(s.Missing, "、"),
				"沒有它會怎樣", s.Without, "怎麼補", s.Fix)
		}
	}
}

// printCapabilitiesJSON serves `--capabilities`: the declared table with no
// probing and no database, so `devctl automation-check` can compare it against
// .env.example without standing a deployment up (R-36's checker).
func printCapabilitiesJSON(w io.Writer) error {
	type row struct {
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		Needs []string `json:"needs"`
	}
	reg := capabilityTable(nil, 0)
	out := make([]row, 0, len(reg.Capabilities()))
	for _, c := range reg.Capabilities() {
		out = append(out, row{ID: c.ID, Name: c.Name, Needs: c.Needs})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

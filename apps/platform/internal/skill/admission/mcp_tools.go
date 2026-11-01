package ingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission/mcptools"
)

func prepareCreated(ctx context.Context, db gen.DBTX, data []byte) (preparedPackage, error) {
	p, err := prepare(data)
	if err != nil {
		return p, err
	}
	return p, checkMCPTools(ctx, db, &p.report, skillpkg.SeverityError)
}

func checkMCPTools(ctx context.Context, db gen.DBTX, report *skillpkg.Report, severity skillpkg.Severity) error {
	if report.Blocked || report.Manifest == nil || !declaresMCPTool(report.Manifest.AllowedTools) {
		return nil
	}
	active, err := mcptools.Active(ctx, db)
	if err != nil {
		return fmt.Errorf("ingest: read the MCP tool list: %w", err)
	}
	markUnlisted(report, active, severity)
	return nil
}

func markUnlisted(report *skillpkg.Report, active []mcptools.Tool, severity skillpkg.Severity) {
	for _, name := range mcptools.Unlisted(report.Manifest.AllowedTools, active) {
		report.Findings = append(report.Findings, skillpkg.Finding{
			Severity: severity, Code: skillpkg.CodeMCPToolNotOffered, Path: "SKILL.md",
			Message: notOfferedMessage(name, active, severity),
		})
		if severity == skillpkg.SeverityError {
			report.Blocked = true
		}
	}
}

func declaresMCPTool(names []string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, mcptools.ReferencePrefix) {
			return true
		}
	}
	return false
}

func notOfferedMessage(name string, active []mcptools.Tool, severity skillpkg.Severity) string {
	if severity != skillpkg.SeverityError {
		return fmt.Sprintf("allowed-tools 宣告了 %s，但平台的 MCP 工具清單上沒有它；在這個平台上試跑時用不到這個工具。", name)
	}
	offered := make([]string, 0, len(active))
	for _, t := range active {
		offered = append(offered, t.Reference())
	}
	if len(offered) == 0 {
		return fmt.Sprintf("allowed-tools 只能引用平台 MCP 工具清單上的名稱，%s 不在清單上；目前清單是空的，請拿掉這個名稱。", name)
	}
	return fmt.Sprintf("allowed-tools 只能引用平台 MCP 工具清單上的名稱，%s 不在清單上；可用的有：%s。", name, strings.Join(offered, "、"))
}

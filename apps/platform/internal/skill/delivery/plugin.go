package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/textx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const (
	pluginManifestFile       = "plugin.json"
	claudePluginManifestFile = ".claude-plugin/plugin.json"
	pluginReadmeFile         = "README.md"
	pluginSkillsDir          = "skills/"
)

type PluginMember struct {
	SkillID   pgtype.UUID
	VersionID pgtype.UUID
}

type PluginSpec struct {
	Name        string
	Version     string
	Description string
	Author      string
	Members     []PluginMember
}

type PluginPlan struct {
	Allowed        bool
	BlockedReason  string
	BlockedMessage string
	BlockedMember  string

	Members  []PluginMemberView
	Zip      []byte
	FileName string

	ContentHash  string
	ManifestHash string
}

type PluginView struct {
	Name    string             `json:"name"`
	Version string             `json:"version"`
	Members []PluginMemberView `json:"members"`
}

type PluginMemberView struct {
	SkillID        string `json:"skill_id"`
	SkillVersionID string `json:"skill_version_id"`
	Name           string `json:"name"`
	VersionNumber  int32  `json:"version_number"`
}

type PluginResult struct {
	Artifact  Artifact
	Duplicate bool
	Plan      *PluginPlan
}

type pluginManifest struct {
	Schema      string        `json:"$schema"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	Author      *pluginAuthor `json:"author,omitempty"`
	License     string        `json:"license,omitempty"`
}

type pluginAuthor struct {
	Name string `json:"name"`
}

type exportedMember struct {
	files       []exportFile
	name        string
	description string
	license     *string
	declared    []string
	offered     []MCPTool
}

var errPluginShape = errors.New("the exported plugin does not read back as the same set of Agent Skills")

func (s *Service) PlanPlugin(ctx context.Context, source identity.Workspace, spec PluginSpec) (*PluginPlan, error) {
	if s.Store == nil {
		return nil, ErrNoStore
	}
	if err := s.requireOwnerReads(); err != nil {
		return nil, err
	}
	p := &PluginPlan{}
	var files []exportFile
	var members []exportedMember
	var declared []string
	var offered []MCPTool
	for _, member := range spec.Members {
		exported, err := s.exportPluginMember(ctx, source, member, p)
		if err != nil {
			return nil, err
		}
		if p.BlockedReason != "" {
			return p, nil
		}
		files = append(files, exported.files...)
		members = append(members, exported)
		declared, offered = append(declared, exported.declared...), append(offered, exported.offered...)
	}
	top, err := pluginTopFiles(spec, members, mcpNeedsOf(declared, offered))
	if err != nil {
		return nil, err
	}
	files = append(top, files...)

	zipped, err := writeZip(files, "")
	if err != nil {
		return nil, err
	}
	if err := checkProducedSize(len(zipped)); err != nil {
		return nil, err
	}
	produced, err := skillpkg.PackageFS(zipped)
	if err != nil {
		return nil, fmt.Errorf("the exported plugin could not be re-opened: %w", err)
	}
	if found := skillpkg.Discover(produced); found.Shape != skillpkg.ShapePlugin || len(found.Skills) != len(spec.Members) {
		return nil, errPluginShape
	}
	hash, err := manifestHash(files)
	if err != nil {
		return nil, err
	}
	p.Allowed = true
	p.Zip, p.ContentHash, p.ManifestHash = zipped, textx.SHA256Hex(zipped), hash
	p.FileName = fmt.Sprintf("%s-%s.zip", spec.Name, spec.Version)
	return p, nil
}

func (s *Service) exportPluginMember(
	ctx context.Context, source identity.Workspace, member PluginMember, p *PluginPlan,
) (exportedMember, error) {
	skill, found, err := s.ReadSkill(ctx, source.ID, member.SkillID)
	if err != nil {
		return exportedMember{}, err
	}
	if !found {
		return exportedMember{}, ErrNotFound
	}
	version, found, err := s.ReadVersion(ctx, source.ID, member.VersionID)
	if err != nil {
		return exportedMember{}, err
	}
	if !found || version.SkillID != skill.ID {
		return exportedMember{}, ErrNotFound
	}
	if reason, message := gate(skill); reason != "" {
		p.blockedBy(skill.Name, reason, message)
		return exportedMember{}, nil
	}
	src, err := s.readSource(ctx, version)
	if err != nil {
		return exportedMember{}, err
	}
	if src.blockedReason != "" {
		p.blockedBy(skill.Name, src.blockedReason, src.blockedMessage)
		return exportedMember{}, nil
	}
	out := exportedMember{
		name: exportedName(skill.Name, src.report), license: version.LicenseExpression,
		declared: declaredTools(src.report), offered: version.MCPTools,
	}
	if src.report.Manifest != nil {
		out.description = src.report.Manifest.Description
	}
	for _, f := range src.files {
		if f.path == ManifestFile {
			continue
		}
		out.files = append(out.files, exportFile{path: pluginSkillsDir + out.name + "/" + f.path, data: f.data, executable: f.executable})
	}
	p.Members = append(p.Members, PluginMemberView{
		SkillID: pgconv.UUIDString(skill.ID), SkillVersionID: pgconv.UUIDString(version.ID),
		Name: out.name, VersionNumber: version.VersionNumber,
	})
	return out, nil
}

func sharedLicense(members []exportedMember) string {
	if len(members) == 0 || members[0].license == nil {
		return ""
	}
	for _, m := range members[1:] {
		if m.license == nil || *m.license != *members[0].license {
			return ""
		}
	}
	return *members[0].license
}

func pluginTopFiles(spec PluginSpec, members []exportedMember, mcp mcpNeeds) ([]exportFile, error) {
	m := pluginManifest{
		Schema: skillpkg.PluginSchemaID, Name: spec.Name, Version: spec.Version, Description: spec.Description,
		License: sharedLicense(members),
	}
	if spec.Author != "" {
		m.Author = &pluginAuthor{Name: spec.Author}
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	manifest = append(manifest, '\n')
	config, err := mcp.configFiles()
	if err != nil {
		return nil, err
	}
	return append([]exportFile{
		{path: pluginManifestFile, data: manifest},
		{path: claudePluginManifestFile, data: manifest},
		{path: pluginReadmeFile, data: []byte(renderPluginReadme(spec, members, m.License, mcp))},
	}, config...), nil
}

func renderPluginReadme(spec PluginSpec, members []exportedMember, license string, mcp mcpNeeds) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s\n\n%s\n\n", spec.Name, spec.Version, spec.Description)
	if spec.Author != "" {
		fmt.Fprintf(&b, "Published by %s.\n\n", spec.Author)
	}
	b.WriteString("## Skills\n\nEach folder under `skills/` is one Agent Skill, pinned to the version listed here; " +
		"its `SKILL.md` says when your Agent should use it.\n\n")
	for _, m := range members {
		fmt.Fprintf(&b, "- `skills/%s/` — %s\n", m.name, strings.Join(strings.Fields(m.description), " "))
	}
	b.WriteString("\n")
	writeMCPServers(&b, mcp, pluginMCPIntro)
	b.WriteString("## License\n\n")
	if license != "" {
		fmt.Fprintf(&b, "Every Skill in this plugin is licensed %s, so `plugin.json` names it.\n\n", license)
	} else {
		b.WriteString("The Skills in this plugin do not share one known license, so `plugin.json` names none. " +
			"Read each Skill's own `license` field and license files before you redistribute it.\n\n")
	}
	fmt.Fprintf(&b, "Packaged by Skill Hub packager %s as an Agent Plugin (%s).\n", PackagerVersion, skillpkg.PluginSpecRevision)
	return b.String()
}

func (p *PluginPlan) blockedBy(member, reason, message string) {
	p.BlockedMember, p.BlockedReason = member, reason
	p.BlockedMessage = "成員 " + member + "：" + message
}

func (s *Service) CreatePluginForRecipient(
	ctx context.Context, recipient identity.Workspace, sourceWorkspaceID pgtype.UUID, spec PluginSpec,
) (PluginResult, error) {
	retention, err := s.Retention.Period()
	if err != nil {
		return PluginResult{}, err
	}
	p, err := s.PlanPlugin(ctx, identity.Workspace{ID: sourceWorkspaceID}, spec)
	if err != nil {
		return PluginResult{}, err
	}
	if !p.Allowed {
		return PluginResult{Plan: p}, nil
	}
	view := &PluginView{Name: spec.Name, Version: spec.Version, Members: p.Members}
	artifact, duplicate, err := s.store(ctx, recipient, retention, storedPackage{
		zip: p.Zip, fileName: p.FileName, contentHash: p.ContentHash,
		reuse: func(q *gen.Queries) (Artifact, bool, error) {
			rows, err := q.ListPluginArtifactsWithIdentity(ctx, gen.ListPluginArtifactsWithIdentityParams{
				WorkspaceID: recipient.ID, PluginName: &spec.Name, PluginVersion: &spec.Version,
				PackagerVersion: PackagerVersion, ContentHash: p.ContentHash,
			})
			if err != nil {
				return Artifact{}, false, err
			}
			for _, row := range rows {
				if servableAt(ScanStatus(row.ScanStatus), row.DeletedAt, row.PurgedAt, row.ExpiresAt, time.Now()) {
					return pluginArtifact(Artifact{
						ArtifactID: pgconv.UUIDString(row.ArtifactID), Target: row.Target, FileName: row.FileName,
						SizeBytes: row.SizeBytes, ContentHash: row.ContentHash, ManifestHash: row.ManifestHash,
						Status: row.ScanStatus, ExpiresAt: rfc3339(row.ExpiresAt), CreatedAt: rfc3339(row.CreatedAt),
						DownloadCount: row.DownloadCount, PackagerVersion: row.PackagerVersion, ProfileVersion: row.ProfileVersion,
					}, view).withServeState(row.ExpiresAt.Time, row.PurgedAt.Time), true, nil
				}
			}
			return Artifact{}, false, nil
		},
		record: func(q *gen.Queries, artifactID pgtype.UUID) error {
			if err := q.CreatePluginArtifactDetail(ctx, gen.CreatePluginArtifactDetailParams{
				ArtifactID: artifactID, WorkspaceID: recipient.ID, PluginName: &spec.Name, PluginVersion: &spec.Version,
				Target: StandardTargetID, ProfileVersion: skillpkg.PluginSpecRevision, PackagerVersion: PackagerVersion,
				ManifestHash: p.ManifestHash,
			}); err != nil {
				return err
			}
			for i, member := range spec.Members {
				if err := q.InsertDownloadArtifactMember(ctx, gen.InsertDownloadArtifactMemberParams{
					ArtifactID: artifactID, WorkspaceID: recipient.ID, SkillVersionID: member.VersionID, Position: int32(i),
				}); err != nil {
					return err
				}
			}
			return nil
		},
		fresh: func(row gen.Artifact) Artifact {
			return pluginArtifact(Artifact{
				ArtifactID: pgconv.UUIDString(row.ID), Target: StandardTargetID, FileName: p.FileName,
				SizeBytes: int64(len(p.Zip)), ContentHash: p.ContentHash, ManifestHash: p.ManifestHash,
				Status: string(ScanAvailable), ExpiresAt: rfc3339(row.ExpiresAt), CreatedAt: rfc3339(row.CreatedAt),
				PackagerVersion: PackagerVersion, ProfileVersion: skillpkg.PluginSpecRevision,
			}, view).withServeState(row.ExpiresAt.Time, time.Time{})
		},
	})
	if err != nil {
		return PluginResult{}, err
	}
	return PluginResult{Artifact: artifact, Duplicate: duplicate, Plan: p}, nil
}

func pluginArtifact(a Artifact, view *PluginView) Artifact {
	a.Plugin = view
	a.VersionState = labelled{"plugin", "Plugin " + view.Name + " " + view.Version,
		"這一份是一組 Skill 打成的 Agent Plugin，成員各自釘住一個版本，內容不會改變；含成員的 Agent Skill、說明檔，以及成員用到且列在平台 MCP 工具清單上的 MCP 伺服器設定；不含其他宿主專屬元件。"}
	return a
}

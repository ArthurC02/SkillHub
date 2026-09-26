package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const (
	pluginManifestFile = "plugin.json"
	pluginSkillsDir    = "skills/"
)

type PluginMember struct {
	SkillID   pgtype.UUID
	VersionID pgtype.UUID
}

type PluginSpec struct {
	Name        string
	Version     string
	Description string
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
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
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
	manifest, err := json.MarshalIndent(pluginManifest{
		Schema: skillpkg.PluginSchemaID, Name: spec.Name, Version: spec.Version, Description: spec.Description,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	files := []exportFile{{path: pluginManifestFile, data: append(manifest, '\n')}}
	for _, member := range spec.Members {
		skill, found, err := s.ReadSkill(ctx, source.ID, member.SkillID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrNotFound
		}
		version, found, err := s.ReadVersion(ctx, source.ID, member.VersionID)
		if err != nil {
			return nil, err
		}
		if !found || version.SkillID != skill.ID {
			return nil, ErrNotFound
		}
		if reason, message := gate(skill); reason != "" {
			return p.blockedBy(skill.Name, reason, message), nil
		}
		src, err := s.readSource(ctx, version)
		if err != nil {
			return nil, err
		}
		if src.blockedReason != "" {
			return p.blockedBy(skill.Name, src.blockedReason, src.blockedMessage), nil
		}
		name := skill.Name
		if src.report.Manifest != nil && src.report.Manifest.Name != "" {
			name = src.report.Manifest.Name
		}
		for _, f := range src.files {
			files = append(files, exportFile{path: pluginSkillsDir + name + "/" + f.path, data: f.data})
		}
		p.Members = append(p.Members, PluginMemberView{
			SkillID: pgconv.UUIDString(skill.ID), SkillVersionID: pgconv.UUIDString(version.ID),
			Name: name, VersionNumber: version.VersionNumber,
		})
	}

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
	p.Zip, p.ContentHash, p.ManifestHash = zipped, sha256Hex(zipped), hash
	p.FileName = fmt.Sprintf("%s-%s.zip", spec.Name, spec.Version)
	return p, nil
}

func (p *PluginPlan) blockedBy(member, reason, message string) *PluginPlan {
	p.BlockedMember, p.BlockedReason = member, reason
	p.BlockedMessage = "成員 " + member + "：" + message
	return p
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
		"這一份是一組 Skill 打成的 Agent Plugin，成員各自釘住一個版本，內容不會改變；只含 Agent Skill，不含 MCP 設定或宿主專屬元件。"}
	return a
}

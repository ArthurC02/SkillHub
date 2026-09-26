package publishing

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type Handler struct {
	Svc      *Service
	Identity *identity.Service

	DescribeRedistribution func(value string) (label, note string)

	DownloadsOpenToUninvited bool
	InviteRosterConfigured   func() bool
}

type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

type publisherView struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type releaseView struct {
	VersionID      string                       `json:"version_id,omitempty"`
	VersionNumber  int32                        `json:"version_number,omitempty"`
	BundleVersion  string                       `json:"bundle_version,omitempty"`
	ContentHash    string                       `json:"content_hash"`
	ReleasedAt     string                       `json:"released_at"`
	RightsAttested bool                         `json:"rights_attested"`
	Findings       skillpkg.CategorizedFindings `json:"findings"`
}

type publicationView struct {
	Kind            string        `json:"kind"`
	Publisher       string        `json:"publisher"`
	Name            string        `json:"name"`
	Address         string        `json:"address"`
	Status          string        `json:"status"`
	StatusChangedAt string        `json:"status_changed_at"`
	Releases        []releaseView `json:"releases"`
}

type publicReleaseView struct {
	VersionNumber int32              `json:"version_number,omitempty"`
	Version       string             `json:"version,omitempty"`
	ContentHash   string             `json:"content_hash"`
	ReleasedAt    string             `json:"released_at"`
	Changes       []memberChangeView `json:"changes,omitempty"`
}

type licenseView struct {
	Expression string `json:"expression"`
	Source     string `json:"source"`
}

type currentReleaseView struct {
	publicReleaseView
	Findings       skillpkg.CategorizedFindings `json:"findings"`
	License        licenseView                  `json:"license"`
	Redistribution labelled                     `json:"redistribution"`
}

type publicSkillView struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type noteView struct {
	Available bool   `json:"available"`
	Note      string `json:"note"`
}

type publicBundleMemberView struct {
	Name          string `json:"name"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
}

type publicBundleView struct {
	Version     string                   `json:"version"`
	Description string                   `json:"description"`
	Members     []publicBundleMemberView `json:"members"`
}

type bundleReleaseView struct {
	publicReleaseView
	Findings skillpkg.CategorizedFindings `json:"findings"`
}

type publicPublicationView struct {
	Kind          string              `json:"kind"`
	Publisher     string              `json:"publisher"`
	Name          string              `json:"name"`
	Address       string              `json:"address"`
	Availability  labelled            `json:"availability"`
	DelistedAt    string              `json:"delisted_at,omitempty"`
	Skill         *publicSkillView    `json:"skill,omitempty"`
	Release       *currentReleaseView `json:"release,omitempty"`
	Bundle        *publicBundleView   `json:"bundle,omitempty"`
	BundleRelease *bundleReleaseView  `json:"bundle_release,omitempty"`
	Releases      []publicReleaseView `json:"releases"`
	Exposure      noteView            `json:"exposure"`
	Acquisition   noteView            `json:"acquisition"`
}

const (
	exposedNote           = "這個發佈物的這一版已經過目錄審核：它會出現在搜尋與目錄裡。"
	notListedNote         = "這個發佈物還沒有經過目錄審核：它不會出現在搜尋與目錄裡，只有拿到這個連結的人看得到。"
	notOfferedNote        = "這個發佈物目前不提供下載，原因見上方。"
	downloadNote          = "登入後可以下載這一版的標準 Agent Skill 套件；下載會記在你自己的工作區，保存期限與下載紀錄照你自己打包的套件一樣。"
	bundleDownloadNote    = "登入後可以下載這一版的 Agent Plugin：只含成員的 Agent Skill，不含 MCP 設定或宿主專屬元件；下載會記在你自己的工作區，保存期限與下載紀錄照你自己打包的套件一樣。"
	invitedOnlyNote       = "這個部署目前只開放受邀者下載：沒有封測邀請的帳號按下下載會被拒絕。"
	downloadContentPrefix = "/downloads/"
	kindSkill             = "skill"
	kindBundle            = "bundle"
)

type acquisitionView struct {
	ArtifactID  string `json:"artifact_id"`
	FileName    string `json:"file_name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	ExpiresAt   string `json:"expires_at"`
	Duplicate   bool   `json:"duplicate"`
	ContentURL  string `json:"content_url"`
}

var availabilityWords = map[Availability][2]string{
	AvailabilityAvailable:        {"提供中", ""},
	AvailabilityDelisted:         {"作者已撤回", "作者撤回了這個發佈物，這一頁不再提供它的內容。"},
	AvailabilityWithdrawn:        {"已不提供", "這個發佈物指向的 Skill 已經被作者刪除。"},
	AvailabilityTakenDown:        {"已不提供", "這個 Skill 已被平台下架。"},
	AvailabilityHeld:             {"已不提供", "這個 Skill 的內容因授權問題被保留，釐清之前不提供。"},
	AvailabilityNotRedistributed: {"已不提供", "這個 Skill 目前的授權判定不允許再散布。"},
}

func address(publisher, name string) string {
	return "/p/" + publisher + "/" + name
}

func timestamp(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return identity.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return identity.Workspace{}, false
	}
	return ws, true
}

func writeReason(w http.ResponseWriter, code int, reason, message string) {
	httpx.WriteJSON(w, code, map[string]string{"error": message, "reason": reason})
}

func writePublishingError(w http.ResponseWriter, err error) {
	var nameErr *NameError
	var refused *RefusedError
	var unavailable *UnavailableError
	var bundleErr *BundleError
	switch {
	case errors.As(err, &unavailable):
		writeReason(w, http.StatusConflict, string(unavailable.Availability), unavailableNote(unavailable.Availability, unavailable.Member))
	case errors.As(err, &bundleErr) && bundleErr.Problem == BundleVersionExists:
		writeReason(w, http.StatusConflict, string(bundleErr.Problem), bundleErr.Error())
	case errors.As(err, &bundleErr):
		writeReason(w, http.StatusUnprocessableEntity, string(bundleErr.Problem), bundleErr.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
	case errors.Is(err, ErrNoPublisher):
		writeReason(w, http.StatusConflict, "no_publisher", err.Error())
	case errors.Is(err, ErrPublisherExists):
		writeReason(w, http.StatusConflict, "already_registered", err.Error())
	case errors.Is(err, ErrNameTaken):
		writeReason(w, http.StatusConflict, "name_taken", err.Error())
	case errors.Is(err, ErrNameIsPermanent):
		writeReason(w, http.StatusConflict, "name_is_permanent", err.Error())
	case errors.As(err, &nameErr):
		writeReason(w, http.StatusUnprocessableEntity, "name_"+string(nameErr.Problem), nameErr.Error())
	case errors.As(err, &refused):
		writeReason(w, http.StatusUnprocessableEntity, string(refused.Reason), refused.Message)
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "publishing failed")
	}
}

func skillIDFrom(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return pgtype.UUID{}, false
	}
	return skillID, true
}

func (h *Handler) OwnPublisher(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	publisher, found, err := h.Svc.Publisher(r.Context(), ws)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "publisher lookup failed")
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "this account has no publisher name yet")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, publisherView{Name: publisher.Name, CreatedAt: timestamp(publisher.CreatedAt)})
}

func (h *Handler) RegisterPublisher(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the body must be JSON with a name")
		return
	}
	publisher, err := h.Svc.RegisterPublisher(r.Context(), ws, req.Name)
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, publisherView{Name: publisher.Name, CreatedAt: timestamp(publisher.CreatedAt)})
}

func (h *Handler) OwnPublication(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, ok := skillIDFrom(w, r)
	if !ok {
		return
	}
	publication, found, err := h.Svc.OwnPublication(r.Context(), ws, skillID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "publication lookup failed")
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "this Skill has not been published")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ownView(publication))
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, ok := skillIDFrom(w, r)
	if !ok {
		return
	}
	var req struct {
		Name           string `json:"name"`
		VersionID      string `json:"version_id"`
		RightsAttested bool   `json:"rights_attested"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the body must be a JSON object")
		return
	}
	in := PublishInput{Name: req.Name, RightsAttested: req.RightsAttested}
	if req.VersionID != "" {
		if err := in.VersionID.Scan(req.VersionID); err != nil {
			httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}
	}
	publication, err := h.Svc.Publish(r.Context(), ws, skillID, in)
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ownView(publication))
}

func (h *Handler) Delist(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, ok := skillIDFrom(w, r)
	if !ok {
		return
	}
	publication, err := h.Svc.Delist(r.Context(), ws, skillID)
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ownView(publication))
}

func (h *Handler) PublicPublication(w http.ResponseWriter, r *http.Request) {
	publication, found, err := h.Svc.PublicPublication(r.Context(), r.PathValue("publisher"), r.PathValue("name"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "publication lookup failed")
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "no publication has this address")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.publicView(publication))
}

func (h *Handler) Acquire(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	acquisition, err := h.Svc.Acquire(r.Context(), ws, r.PathValue("publisher"), r.PathValue("name"))
	if err != nil {
		writePublishingError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, acquisitionViewOf(acquisition))
}

func acquisitionViewOf(acquisition Acquisition) acquisitionView {
	return acquisitionView{
		ArtifactID: acquisition.ArtifactID, FileName: acquisition.FileName, SizeBytes: acquisition.SizeBytes,
		ContentHash: acquisition.ContentHash, ExpiresAt: acquisition.ExpiresAt, Duplicate: acquisition.Duplicate,
		ContentURL: downloadContentPrefix + acquisition.ArtifactID + "/content",
	}
}

func unavailableNote(availability Availability, member string) string {
	note := availabilityWords[availability][1]
	if member == "" {
		return note
	}
	return "成員 " + member + "：" + note
}

func exposureNote(exposed bool) noteView {
	if exposed {
		return noteView{Available: true, Note: exposedNote}
	}
	return noteView{Available: false, Note: notListedNote}
}

func kindOf(p Publication) string {
	if p.BundleID.Valid {
		return kindBundle
	}
	return kindSkill
}

func (h *Handler) acquisitionNote(availability Availability, kind string) noteView {
	if availability != AvailabilityAvailable {
		return noteView{Available: false, Note: notOfferedNote}
	}
	note := downloadNote
	if kind == kindBundle {
		note = bundleDownloadNote
	}
	if !h.DownloadsOpenToUninvited && h.InviteRosterConfigured() {
		return noteView{Available: true, Note: note + invitedOnlyNote}
	}
	return noteView{Available: true, Note: note}
}

func ownView(p Publication) publicationView {
	releases := make([]releaseView, 0, len(p.Releases))
	for _, release := range p.Releases {
		view := releaseView{
			ContentHash: release.ContentHash, ReleasedAt: timestamp(release.ReleasedAt),
			RightsAttested: release.RightsAttested, Findings: release.Findings,
		}
		if release.Bundle != nil {
			view.BundleVersion = release.Bundle.Version
		} else {
			view.VersionID, view.VersionNumber = pgconv.UUIDString(release.VersionID), release.VersionNumber
		}
		releases = append(releases, view)
	}
	return publicationView{
		Kind: kindOf(p), Publisher: p.Publisher, Name: p.Name, Address: address(p.Publisher, p.Name),
		Status: string(p.Status), StatusChangedAt: timestamp(p.StatusChangedAt), Releases: releases,
	}
}

func (h *Handler) publicView(p PublicPublication) publicPublicationView {
	words := availabilityWords[p.Availability]
	view := publicPublicationView{
		Kind: kindOf(p.Publication), Publisher: p.Publisher, Name: p.Name, Address: address(p.Publisher, p.Name),
		Availability: labelled{Value: string(p.Availability), Label: words[0], Note: unavailableNote(p.Availability, p.UnavailableMember)},
		Releases:     make([]publicReleaseView, 0, len(p.Releases)),
		Exposure:     exposureNote(p.Exposed),
		Acquisition:  h.acquisitionNote(p.Availability, kindOf(p.Publication)),
	}
	if p.Status == StatusDelisted {
		view.DelistedAt = timestamp(p.StatusChangedAt)
	}
	if p.Availability != AvailabilityAvailable || len(p.Releases) == 0 {
		return view
	}
	if p.BundleID.Valid {
		return publicBundle(view, p.Releases)
	}
	for _, release := range p.Releases {
		view.Releases = append(view.Releases, publicReleaseView{
			VersionNumber: release.VersionNumber, ContentHash: release.ContentHash, ReleasedAt: timestamp(release.ReleasedAt),
		})
	}
	current := p.Releases[0]
	label, note := h.DescribeRedistribution(p.Skill.Redistribution)
	view.Skill = &publicSkillView{Name: p.Skill.Name, Summary: p.Skill.Summary}
	view.Release = &currentReleaseView{
		publicReleaseView: view.Releases[0],
		Findings:          current.Findings,
		License:           licenseView{Expression: p.Version.LicenseExpression, Source: p.Version.LicenseSource},
		Redistribution:    labelled{Value: p.Skill.Redistribution, Label: label, Note: note},
	}
	return view
}

func publicBundle(view publicPublicationView, releases []Release) publicPublicationView {
	for i, release := range releases {
		entry := publicReleaseView{
			Version: release.Bundle.Version, ContentHash: release.ContentHash, ReleasedAt: timestamp(release.ReleasedAt),
		}
		if i+1 < len(releases) {
			entry.Changes = memberChanges(*release.Bundle, *releases[i+1].Bundle)
		}
		view.Releases = append(view.Releases, entry)
	}
	current := releases[0]
	members := make([]publicBundleMemberView, 0, len(current.Bundle.Members))
	for _, m := range current.Bundle.Members {
		members = append(members, publicBundleMemberView{Name: m.Name, VersionNumber: m.VersionNumber, ContentHash: m.ContentHash})
	}
	view.Bundle = &publicBundleView{Version: current.Bundle.Version, Description: current.Bundle.Description, Members: members}
	view.BundleRelease = &bundleReleaseView{publicReleaseView: view.Releases[0], Findings: current.Findings}
	return view
}

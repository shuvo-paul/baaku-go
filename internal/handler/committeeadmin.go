package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/committee"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// CommitteeAdmin is the committee-members CRUD surface (reference
// CommitteeController), behind "manage committee" at the route layer.
type CommitteeAdmin struct {
	svc     *committee.Service
	appName string
}

func NewCommitteeAdmin(svc *committee.Service, appName string) *CommitteeAdmin {
	return &CommitteeAdmin{svc: svc, appName: appName}
}

// Index renders the members table (reference CommitteeController@index).
func (h *CommitteeAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	members, err := h.svc.Members(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	rows := make([]views.CommitteeMemberRow, 0, len(members))
	for _, m := range members {
		rows = append(rows, committeeRow(m))
	}
	flash := middleware.FlashFromContext(r.Context())
	views.CommitteeIndexPage(views.CommitteeIndexData{
		Sidebar: committeeSidebar(h.appName, r, u),
		Rows:    rows,
		Flash:   flash["status"],
		CSRF:    middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Create renders the create-member form (reference CommitteeController@create).
func (h *CommitteeAdmin) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	form := views.CommitteeMemberForm{Title: "New Member", MemberType: "non_registered"}
	views.CommitteeMemberFormPage(h.formData(r, u, form)).Render(r.Context(), w)
}

// Store handles POST /dashboard/committee (reference CommitteeController@store).
func (h *CommitteeAdmin) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseMultipartForm(maxPhotoBytes); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in, form := h.inputFrom(r)
	if _, err := h.svc.Store(r.Context(), in); err != nil {
		var fieldErrs committee.FieldErrors
		if errors.As(err, &fieldErrs) {
			form.Errors = fieldErrs
			views.CommitteeMemberFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/committee", "status", "member-created")
}

// Edit renders the edit-member form (reference CommitteeController@edit).
func (h *CommitteeAdmin) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, committee.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	form := views.CommitteeMemberForm{
		Title:         "Edit Member",
		ID:            m.ID,
		PositionID:    int64PtrString(m.PositionID),
		UserIDValue:   int64PtrString(m.UserID),
		Name:          deref(m.Name),
		ExistingPhoto: m.PhotoURL(),
		MemberType:    "non_registered",
	}
	if m.UserID != nil {
		form.MemberType = "registered"
		// The picker shows "name — email" for a selected member (reference
		// UserSearch@mount).
		if m.UserName != nil && m.UserEmail != nil {
			form.SelectedLabel = *m.UserName + " — " + *m.UserEmail
		} else {
			form.SelectedLabel = m.DisplayName()
		}
	}
	views.CommitteeMemberFormPage(h.formData(r, u, form)).Render(r.Context(), w)
}

// Update handles PUT /dashboard/committee/{id} (reference
// CommitteeController@update).
func (h *CommitteeAdmin) Update(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(maxPhotoBytes); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in, form := h.inputFrom(r)
	if path, stored := h.storePhoto(r); stored {
		in.PhotoPath = &path
		// Reference deletes the old photo when a replacement arrives.
		if old, err := h.svc.Get(r.Context(), id); err == nil && old.PhotoPath != nil {
			_ = os.Remove(filepath.Join(committeePhotoDir(), filepath.Base(*old.PhotoPath)))
		}
	} else {
		in.KeepPhoto = true
	}
	if err := h.svc.Update(r.Context(), id, in); err != nil {
		if errors.Is(err, committee.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		var fieldErrs committee.FieldErrors
		if errors.As(err, &fieldErrs) {
			form.ID = id
			form.Errors = fieldErrs
			views.CommitteeMemberFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/committee", "status", "member-updated")
}

// Destroy handles DELETE /dashboard/committee/{id} (reference
// CommitteeController@destroy): removes the stored photo then the row.
func (h *CommitteeAdmin) Destroy(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, committee.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if err := h.svc.Destroy(r.Context(), id); err != nil {
		if errors.Is(err, committee.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if m.PhotoPath != nil && *m.PhotoPath != "" {
		_ = os.Remove(filepath.Join(committeePhotoDir(), filepath.Base(*m.PhotoPath)))
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/committee", "status", "member-deleted")
}

// Reorder handles POST /dashboard/committee/reorder (reference
// CommitteeController@reorder): assigns each id its list index as the new sort
// order.
func (h *CommitteeAdmin) Reorder(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserFromContext(r.Context()); !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	ids, err := decodeReorderIDs(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	for i, id := range ids {
		if err := h.svc.Reorder(r.Context(), id, int32(i)); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w)
}

// SearchUsers handles GET /dashboard/committee/users/search — the member
// picker's live search (reference Livewire UserSearch updatedQuery).
func (h *CommitteeAdmin) SearchUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserFromContext(r.Context()); !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	users, err := h.svc.SearchUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}

// inputFrom parses the member form into the service input and the view form
// (reference Store/UpdateCommitteeMemberRequest + prepareForValidation, which
// turns a blank user_id into null).
func (h *CommitteeAdmin) inputFrom(r *http.Request) (committee.StoreInput, views.CommitteeMemberForm) {
	in := committee.StoreInput{
		PositionID: int64Ptr(r.FormValue("position_id")),
		UserID:     int64Ptr(r.FormValue("user_id")),
		Name:       optString(r.FormValue("name")),
	}
	form := views.CommitteeMemberForm{
		Title:      "New Member",
		PositionID: r.FormValue("position_id"),
		MemberType: r.FormValue("member_type"),
		Name:       r.FormValue("name"),
	}
	if form.MemberType == "" {
		form.MemberType = "non_registered"
	}
	if in.UserID != nil {
		form.MemberType = "registered"
		form.UserIDValue = strconv.FormatInt(*in.UserID, 10)
	}
	if path, stored := h.storePhoto(r); stored {
		in.PhotoPath = &path
		form.ExistingPhoto = "/media/committee-photos/" + filepath.Base(path)
	}
	form.Errors = map[string]string{}
	return in, form
}

// formData builds the member form view model.
func (h *CommitteeAdmin) formData(r *http.Request, u user.User, form views.CommitteeMemberForm) views.CommitteeMemberForm {
	form.Sidebar = committeeSidebar(h.appName, r, u)
	if form.Title == "" {
		form.Title = "New Member"
	}
	if form.MemberType == "" {
		form.MemberType = "non_registered"
	}
	positions, err := h.svc.Positions(r.Context())
	if err != nil {
		positions = nil
	}
	opts := make([]views.Option, 0, len(positions)+1)
	opts = append(opts, views.Option{Value: "", Label: "Select a position"})
	for _, p := range positions {
		opts = append(opts, views.Option{Value: strconv.FormatInt(p.ID, 10), Label: p.Name})
	}
	form.Positions = opts
	if form.Errors == nil {
		form.Errors = map[string]string{}
	}
	form.CSRF = middleware.TokenFromContext(r.Context())
	return form
}

// storePhoto persists an uploaded committee photo to the public disk and
// returns its relative path (ok=false when no file was sent).
func (h *CommitteeAdmin) storePhoto(r *http.Request) (string, bool) {
	file, header, err := r.FormFile("photo")
	if err != nil {
		return "", false
	}
	defer file.Close()

	if header.Size > maxPhotoBytes {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isImageExt(ext) {
		return "", false
	}
	dir := committeePhotoDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	name := randomFileName(ext)
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", false
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return "", false
	}
	return filepath.Join("committee-photos", name), true
}

// committeeSidebar builds the dashboard chrome for a committee page.
func committeeSidebar(appName string, r *http.Request, u user.User) views.SidebarData {
	return views.NewSidebarData(appName, middleware.TokenFromContext(r.Context()), u.Name, u.Email, r.URL.Path, string(u.State), middleware.PermissionsFromContext(r.Context()))
}

// committeePhotoDir is the public-disk committee-photos folder (relative to
// CWD), mirroring the reference Storage::disk('public') path.
func committeePhotoDir() string {
	return filepath.Join("storage", "app", "public", "committee-photos")
}

// committeeRow maps a domain member to the dashboard table row.
func committeeRow(m committee.Member) views.CommitteeMemberRow {
	position := "—"
	if m.PositionName != nil && *m.PositionName != "" {
		position = *m.PositionName
	}
	return views.CommitteeMemberRow{
		ID:           m.ID,
		PositionName: position,
		DisplayName:  m.DisplayName(),
		PhotoURL:     m.PhotoURL(),
	}
}

// int64Ptr parses a form value to *int64 (nil when blank/invalid).
func int64Ptr(s string) *int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// int64PtrString renders a *int64 as a form value ("" when nil).
func int64PtrString(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

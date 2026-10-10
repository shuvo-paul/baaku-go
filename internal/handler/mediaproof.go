package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MediaMembershipProof serves GET /media/membership-proofs/{file}: streams a
// stored payment proof from the public disk (reference
// PaymentController@proof via Storage::disk('public')).
type MediaMembershipProof struct {
	dir string
}

func NewMediaMembershipProof() *MediaMembershipProof {
	return &MediaMembershipProof{dir: proofDir()}
}

func (h *MediaMembershipProof) Show(w http.ResponseWriter, r *http.Request) {
	// The name can't traverse directories, but basename anyway — defense in
	// depth.
	name := filepath.Base(r.PathValue("file"))
	if name == "." || name == string(filepath.Separator) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(h.dir, name)
	if !strings.HasPrefix(full, h.dir) {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(full); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(name))
	http.ServeFile(w, r, full)
}

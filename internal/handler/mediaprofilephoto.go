package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MediaProfilePhoto serves GET /media/profile-photos/{file} (reference
// MediaController@profilePhoto): streams a stored photo from the public disk,
// no storage:link required.
type MediaProfilePhoto struct {
	dir string
}

func NewMediaProfilePhoto() *MediaProfilePhoto {
	return &MediaProfilePhoto{dir: photoDir()}
}

func (h *MediaProfilePhoto) Show(w http.ResponseWriter, r *http.Request) {
	// The route regexp is [\w.\-]+ so the name can't traverse directories, but
	// basename anyway — defense in depth.
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

func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "image/jpeg"
	}
}

package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MediaCommitteePhoto serves GET /media/committee-photos/{file} (reference
// MediaController@committeePhoto).
type MediaCommitteePhoto struct {
	dir string
}

func NewMediaCommitteePhoto() *MediaCommitteePhoto {
	return &MediaCommitteePhoto{dir: committeePhotoDir()}
}

func (h *MediaCommitteePhoto) Show(w http.ResponseWriter, r *http.Request) {
	serveMediaFile(w, r, h.dir)
}

// serveMediaFile streams one file from the given public-disk subdirectory.
func serveMediaFile(w http.ResponseWriter, r *http.Request, dir string) {
	// The route regexp is [\w.\-]+ so the name can't traverse directories, but
	// basename anyway — defense in depth.
	name := filepath.Base(r.PathValue("file"))
	if name == "." || name == string(filepath.Separator) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(dir, name)
	if !strings.HasPrefix(full, dir) {
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

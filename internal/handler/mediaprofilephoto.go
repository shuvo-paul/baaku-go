package handler

import "net/http"

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
	serveMediaFile(w, r, h.dir)
}

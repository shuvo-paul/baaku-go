// Session-backed flash messages and the shared 302 redirect helper.
//
// Laravel semantics: ->with($key, $value) at redirect time stores the pair in
// the session payload; the next request reads it once, then it is gone.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
)

// sessionPayload is the JSON stored in the sessions.payload text column.
//
// ponytail: only the flash bucket and the pending-2FA flag exist today;
// other Laravel payload keys (e.g. _token) join this struct when a feature
// needs them.
type sessionPayload struct {
	Flash map[string]string `json:"flash,omitempty"`
	// LoginTwoFactor is the pending-2FA login flag (user id), written by
	// the twofactorchallenge service. Listed here so flash writes don't
	// drop it when re-marshaling the payload.
	LoginTwoFactor int64 `json:"login.two_factor,omitempty"`
}

func payloadFlash(raw string) map[string]string {
	var p sessionPayload
	_ = json.Unmarshal([]byte(raw), &p) // corrupt payload = no flash
	return p.Flash
}

func setPayloadFlash(raw string, flash map[string]string) string {
	var p sessionPayload
	_ = json.Unmarshal([]byte(raw), &p)
	p.Flash = flash
	out, err := json.Marshal(p)
	if err != nil {
		return raw // unreachable: struct is always marshalable
	}
	return string(out)
}

type flashCtxKey struct{}

func withFlash(ctx context.Context, flash map[string]string) context.Context {
	return context.WithValue(ctx, flashCtxKey{}, flash)
}

// FlashFromContext returns the flash messages aged into this request. Read-once:
// the session middleware already stripped them from the stored payload, so the
// following request sees nothing.
func FlashFromContext(ctx context.Context) map[string]string {
	flash, _ := ctx.Value(flashCtxKey{}).(map[string]string)
	return flash
}

// SetFlash writes key→value into the session payload for the next request
// (Laravel ->with()). No-op for guests — no session in context, nothing to
// write into.
func SetFlash(ctx context.Context, key, value string) {
	st, ok := ctx.Value(sessionCtxKey{}).(sessionState)
	if !ok {
		return
	}
	flash := payloadFlash(st.sess.Payload)
	if flash == nil {
		flash = map[string]string{}
	}
	flash[key] = value
	st.sess.Payload = setPayloadFlash(st.sess.Payload, flash)
	_ = st.saver.Save(ctx, upsertSessionParams(st.sess))
}

// Redirect 302s.
func Redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusFound)
}

// RedirectWithFlash 302s after setting a session flash (Laravel
// redirect()->with($key, $value)). A flash-save failure still redirects — the
// flash is a nicety, the redirect is the behaviour.
func RedirectWithFlash(w http.ResponseWriter, r *http.Request, to, key, value string) {
	SetFlash(r.Context(), key, value)
	Redirect(w, r, to)
}

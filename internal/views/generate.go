// Package views holds the templ components: the app layout and the auth
// pages mirroring reference/resources/views/auth/ (blade source of truth).
// Labels are English literals for now — swap in translations when an i18n
// layer lands, same stance as middleware.SuspendedErrorKey.
//
// Regenerate *_templ.go after editing .templ files: go generate ./...
package views

//go:generate go tool templ generate

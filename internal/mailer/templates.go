package mailer

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// Render funcs return the subject and HTML body for each auth email. Wording
// mirrors Laravel's default notifications: Fortify's ResetPassword and
// Illuminate's VerifyEmail (the reference app overrides neither).
type ResetPasswordData struct {
	Name           string
	URL            string
	ExpiresMinutes int
}

type VerifyEmailData struct {
	Name string
	URL  string
}

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// ResetPassword renders the password-reset email.
func ResetPassword(d ResetPasswordData) (subject, body string, err error) {
	return render("reset-password.html", "Reset Password Notification", d)
}

// VerifyEmail renders the email-verification email.
func VerifyEmail(d VerifyEmailData) (subject, body string, err error) {
	return render("verify-email.html", "Verify Email Address", d)
}

func render(name, subject string, data any) (string, string, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", "", fmt.Errorf("mailer: render %s: %w", name, err)
	}
	return subject, buf.String(), nil
}

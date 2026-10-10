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

// UserActivated renders the reference UserActivatedNotification email.
type UserActivatedData struct{ URL string }

// UserSuspended renders the reference UserSuspendedNotification email.
type UserSuspendedData struct{ Reason string }

// UserRejected renders the reference UserRejectedNotification email.
type UserRejectedData struct{ Reason string }

// UserActivated renders the account-activated email (no name in the Laravel
// MailMessage body).
func UserActivated(d UserActivatedData) (subject, body string, err error) {
	return render("user-activated.html", "Your account has been activated", d)
}

// UserSuspended renders the account-suspended email with the reason line.
func UserSuspended(d UserSuspendedData) (subject, body string, err error) {
	return render("user-suspended.html", "Your account has been suspended", d)
}

// UserRejected renders the application-rejected email with the reason line.
func UserRejected(d UserRejectedData) (subject, body string, err error) {
	return render("user-rejected.html", "Your membership application was not approved", d)
}

// MembershipActivated renders the reference MembershipActivatedNotification
// email (EndDate empty → lifetime line).
type MembershipActivatedData struct {
	EndDate string
	URL     string
}

// MembershipActivated renders the membership-activated email.
func MembershipActivated(d MembershipActivatedData) (subject, body string, err error) {
	return render("membership-activated.html", "Your membership is active", d)
}

// MembershipRejected renders the reference MembershipRejectedNotification
// email (Reason empty → the review-notes line is omitted).
type MembershipRejectedData struct {
	Reason string
	URL    string
}

// MembershipRejected renders the payment-rejected email with the reason line.
func MembershipRejected(d MembershipRejectedData) (subject, body string, err error) {
	return render("membership-rejected.html", "Your membership payment was not approved", d)
}

func render(name, subject string, data any) (string, string, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", "", fmt.Errorf("mailer: render %s: %w", name, err)
	}
	return subject, buf.String(), nil
}

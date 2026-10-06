package mailer_test

import (
	"strings"
	"testing"

	"github.com/shuvo-paul/baaku/internal/mailer"
)

func TestResetPasswordRender(t *testing.T) {
	subject, body, err := mailer.ResetPassword(mailer.ResetPasswordData{
		Name:           "Ada",
		URL:            "https://baaku.test/reset?token=abc123",
		ExpiresMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Reset Password Notification" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{
		"Hello!",
		"Please click the button below to reset your password:",
		`href="https://baaku.test/reset?token=abc123"`,
		">Reset Password</a>",
		"This password reset link will expire in 60 minutes.",
		"If you did not create an account, no further action is required.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

func TestVerifyEmailRender(t *testing.T) {
	subject, body, err := mailer.VerifyEmail(mailer.VerifyEmailData{
		Name: "Ada",
		URL:  "https://baaku.test/verify?signature=sig456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Verify Email Address" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{
		"Hello!",
		"Please click the button below to verify your email address:",
		`href="https://baaku.test/verify?signature=sig456"`,
		">Verify Email Address</a>",
		"If you did not create an account, no further action is required.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

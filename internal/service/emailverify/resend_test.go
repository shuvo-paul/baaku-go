package emailverify_test

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/service/emailverify"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

type resendStore struct{ users map[int64]user.User }

func (f *resendStore) GetByID(_ context.Context, id int64) (user.User, error) {
	u, ok := f.users[id]
	if !ok {
		return user.User{}, errors.New("user not found")
	}
	return u, nil
}

func (f *resendStore) SetEmailVerified(context.Context, int64) error { return nil }
func (f *resendStore) UpdateState(context.Context, int64, user.UserState) error {
	return nil
}

type resendMail struct{ to, subject, body string }

func TestResendSendsVerifiableLink(t *testing.T) {
	users := &resendStore{users: map[int64]user.User{
		5: {ID: 5, Name: "Ada", Email: "ada@example.com"},
	}}
	var sent []resendMail
	send := func(to, subject, body string) error {
		sent = append(sent, resendMail{to, subject, body})
		return nil
	}
	r := emailverify.NewResender(users, send, "https://baaku.test", testKey)

	if err := r.Resend(context.Background(), 5); err != nil {
		t.Fatalf("Resend = %v, want nil", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(sent))
	}
	if sent[0].to != "ada@example.com" {
		t.Errorf("to = %q", sent[0].to)
	}
	if sent[0].subject != "Verify Email Address" {
		t.Errorf("subject = %q", sent[0].subject)
	}

	// Pull the signed URL out of the rendered body and verify it end to end.
	re := regexp.MustCompile(`href="([^"]+)"`)
	m := re.FindStringSubmatch(sent[0].body)
	if m == nil {
		t.Fatalf("no href in body:\n%s", sent[0].body)
	}
	rawURL := m[1]
	if !strings.HasPrefix(rawURL, "https://baaku.test/email/verify/5/") {
		t.Fatalf("url = %q, want baaku.test /email/verify/5/ prefix", rawURL)
	}

	signed := strings.TrimPrefix(rawURL, "https://baaku.test")
	signed = strings.ReplaceAll(signed, "&amp;", "&") // HTML-escaped in template
	path, rest, ok := strings.Cut(signed, "?expires=")
	if !ok {
		t.Fatalf("missing expires: %s", signed)
	}
	expStr, sig, ok := strings.Cut(rest, "&signature=")
	if !ok {
		t.Fatalf("missing signature: %s", signed)
	}
	expires, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		t.Fatalf("expires not unix: %q", expStr)
	}
	// Window: roughly ResendValidity from now (allow clock skew in test).
	remain := expires - time.Now().Unix()
	if remain < int64(emailverify.ResendValidity.Seconds())-60 || remain > int64(emailverify.ResendValidity.Seconds())+60 {
		t.Errorf("expires in %ds, want ~%ds", remain, int64(emailverify.ResendValidity.Seconds()))
	}
	if err := emailverify.Verify(path, expires, sig, testKey); err != nil {
		t.Errorf("Verify(signed URL from mail) = %v, want nil", err)
	}
}

func TestResendUnknownUser(t *testing.T) {
	r := emailverify.NewResender(&resendStore{}, func(string, string, string) error { return nil }, "https://baaku.test", testKey)
	if err := r.Resend(context.Background(), 99); err == nil {
		t.Error("Resend(unknown) = nil, want error")
	}
}

func TestResendSendErrorPropagates(t *testing.T) {
	users := &resendStore{users: map[int64]user.User{5: {ID: 5, Email: "ada@example.com"}}}
	want := errors.New("smtp down")
	r := emailverify.NewResender(users, func(string, string, string) error { return want }, "https://baaku.test", testKey)
	if err := r.Resend(context.Background(), 5); !errors.Is(err, want) {
		t.Errorf("Resend = %v, want %v", err, want)
	}
}

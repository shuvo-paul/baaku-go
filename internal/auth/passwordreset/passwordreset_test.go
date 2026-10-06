package passwordreset

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/auth"
	"github.com/shuvo-paul/baaku/internal/auth/user"
)

const (
	email    = "user@example.com"
	goodPass = "Str0ng!Pass1"
)

// baseTime is the fixture clock: every seeded created_at is relative to it.
var baseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// clock is a shared mutable time source for the service and the fake token
// store, so window tests can advance time.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type fakeUsers struct {
	users     map[string]user.User
	passwords map[int64]string
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (user.User, error) {
	u, ok := f.users[email]
	if !ok {
		return user.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeUsers) SetPassword(_ context.Context, id int64, passwordHash string) error {
	f.passwords[id] = passwordHash
	return nil
}

type fakeTokens struct {
	now  func() time.Time
	rows map[string]Token
}

func (f *fakeTokens) Upsert(_ context.Context, email, token string) error {
	if f.rows == nil {
		f.rows = map[string]Token{}
	}
	t := f.now()
	f.rows[email] = Token{Email: email, Token: token, CreatedAt: &t}
	return nil
}

func (f *fakeTokens) GetByEmail(_ context.Context, email string) (Token, error) {
	tok, ok := f.rows[email]
	if !ok {
		return Token{}, pgx.ErrNoRows
	}
	return tok, nil
}

func (f *fakeTokens) DeleteByEmail(_ context.Context, email string) error {
	delete(f.rows, email)
	return nil
}

// seedToken stores raw at created, as Issue would.
func (f *fakeTokens) seedToken(email, raw string, created time.Time) {
	f.rows[email] = Token{Email: email, Token: raw, CreatedAt: &created}
}

func newFixture() (*Service, *fakeUsers, *fakeTokens, *clock) {
	c := &clock{t: baseTime}
	users := &fakeUsers{
		users:     map[string]user.User{email: {ID: 1, Email: email}},
		passwords: map[int64]string{},
	}
	tokens := &fakeTokens{now: c.now, rows: map[string]Token{}}
	svc := New(users, tokens)
	svc.now = c.now
	return svc, users, tokens, c
}

func TestIssue(t *testing.T) {
	svc, _, tokens, _ := newFixture()
	ctx := context.Background()

	raw, err := svc.Issue(ctx, email)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(raw) != TokenBytes*2 {
		t.Errorf("token length = %d, want %d (hex of %d bytes)", len(raw), TokenBytes*2, TokenBytes)
	}
	if _, err := hex.DecodeString(raw); err != nil {
		t.Errorf("token is not hex: %v", err)
	}
	row := tokens.rows[email]
	if row.Token != raw {
		t.Errorf("stored token = %q, want raw token as issued", row.Token)
	}
	if row.CreatedAt == nil || !row.CreatedAt.Equal(baseTime) {
		t.Errorf("created_at = %v, want fixture clock", row.CreatedAt)
	}

	// Upsert per email: a second Issue replaces the first.
	raw2, err := svc.Issue(ctx, email)
	if err != nil {
		t.Fatalf("second Issue: %v", err)
	}
	if ok, _ := svc.Lookup(ctx, email, raw); ok {
		t.Error("first token still valid after reissue; want replaced")
	}
	if ok, _ := svc.Lookup(ctx, email, raw2); !ok {
		t.Error("second token not valid after reissue")
	}
}

func TestIssueUnknownUser(t *testing.T) {
	svc, _, _, _ := newFixture()
	if _, err := svc.Issue(context.Background(), "ghost@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("got %v, want ErrUserNotFound", err)
	}
}

func TestLookup(t *testing.T) {
	const raw = "aabb"
	tests := []struct {
		name  string
		seed  func(*fakeTokens)
		token string
		want  bool
	}{
		{
			name:  "valid window",
			seed:  func(tk *fakeTokens) { tk.seedToken(email, raw, baseTime.Add(-30*time.Minute)) },
			token: raw,
			want:  true,
		},
		{
			name:  "wrong token",
			seed:  func(tk *fakeTokens) { tk.seedToken(email, raw, baseTime.Add(-30*time.Minute)) },
			token: "ffff",
			want:  false,
		},
		{
			name:  "expired window",
			seed:  func(tk *fakeTokens) { tk.seedToken(email, raw, baseTime.Add(-ValidityWindow-time.Minute)) },
			token: raw,
			want:  false,
		},
		{
			name:  "missing row",
			seed:  func(*fakeTokens) {},
			token: raw,
			want:  false,
		},
		{
			name: "null created_at",
			seed: func(tk *fakeTokens) {
				tk.seedToken(email, raw, time.Time{})
				row := tk.rows[email]
				row.CreatedAt = nil
				tk.rows[email] = row
			},
			token: raw,
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, tokens, _ := newFixture()
			tt.seed(tokens)
			got, err := svc.Lookup(context.Background(), email, tt.token)
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			if got != tt.want {
				t.Errorf("Lookup = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComplete(t *testing.T) {
	svc, users, tokens, _ := newFixture()
	ctx := context.Background()

	raw, err := svc.Issue(ctx, email)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := svc.Complete(ctx, email, raw, goodPass); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !auth.Compare(users.passwords[1], goodPass) {
		t.Error("users.password not updated to new password hash")
	}
	if _, ok := tokens.rows[email]; ok {
		t.Error("token row not consumed after successful Complete")
	}

	// Consume-once: same token again fails.
	if err := svc.Complete(ctx, email, raw, goodPass); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("second Complete: got %v, want ErrInvalidToken", err)
	}
}

func TestCompleteFailures(t *testing.T) {
	const raw = "aabb"
	tests := []struct {
		name      string
		seed      func(*fakeTokens)
		email     string
		token     string
		pass      string
		wantSent  error // sentinel to match with errors.Is; nil → substring check
		wantSub   string
		wantKeep  bool // token row must survive
		wantNoPwd bool // password must not change
	}{
		{
			name:      "wrong token",
			seed:      func(tk *fakeTokens) { tk.seedToken(email, raw, baseTime.Add(-30*time.Minute)) },
			email:     email,
			token:     "ffff",
			pass:      goodPass,
			wantSent:  ErrInvalidToken,
			wantKeep:  true,
			wantNoPwd: true,
		},
		{
			name:      "expired window",
			seed:      func(tk *fakeTokens) { tk.seedToken(email, raw, baseTime.Add(-ValidityWindow-time.Minute)) },
			email:     email,
			token:     raw,
			pass:      goodPass,
			wantSent:  ErrInvalidToken,
			wantKeep:  true,
			wantNoPwd: true,
		},
		{
			name:      "missing token row",
			seed:      func(*fakeTokens) {},
			email:     email,
			token:     raw,
			pass:      goodPass,
			wantSent:  ErrInvalidToken,
			wantNoPwd: true,
		},
		{
			name:      "unknown user",
			seed:      func(tk *fakeTokens) { tk.seedToken("ghost@example.com", raw, baseTime.Add(-30*time.Minute)) },
			email:     "ghost@example.com",
			token:     raw,
			pass:      goodPass,
			wantSent:  ErrUserNotFound,
			wantNoPwd: true,
		},
		{
			name:      "password fails rules",
			seed:      func(tk *fakeTokens) { tk.seedToken(email, raw, baseTime.Add(-30*time.Minute)) },
			email:     email,
			token:     raw,
			pass:      "short",
			wantSub:   "password rules failed",
			wantKeep:  true,
			wantNoPwd: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, users, tokens, _ := newFixture()
			tt.seed(tokens)
			err := svc.Complete(context.Background(), tt.email, tt.token, tt.pass)
			if tt.wantSent != nil {
				if !errors.Is(err, tt.wantSent) {
					t.Fatalf("Complete: got %v, want %v", err, tt.wantSent)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Complete: got %v, want error containing %q", err, tt.wantSub)
			}
			if tt.wantKeep {
				if _, ok := tokens.rows[tt.email]; !ok {
					t.Error("token row consumed on failed Complete; want kept")
				}
			}
			if tt.wantNoPwd {
				if _, ok := users.passwords[1]; ok && tt.wantSent != ErrUserNotFound {
					t.Error("password changed on failed Complete; want unchanged")
				}
			}
		})
	}
}

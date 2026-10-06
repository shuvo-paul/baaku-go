package passwordreset

import (
	"context"
	"time"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// Token is a password_reset_tokens row; Token holds the raw reset token.
type Token struct {
	Email     string
	Token     string
	CreatedAt *time.Time
}

// Repo is a thin wrapper over the sqlc-generated password_reset_tokens queries.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

func (r *Repo) Upsert(ctx context.Context, email, hashed string) error {
	return r.q.UpsertPasswordResetToken(ctx, generated.UpsertPasswordResetTokenParams{
		Email: email,
		Token: hashed,
	})
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (Token, error) {
	t, err := r.q.GetPasswordResetTokenByEmail(ctx, email)
	if err != nil {
		return Token{}, err
	}
	tok := Token{Email: t.Email, Token: t.Token}
	if t.CreatedAt.Valid {
		ts := t.CreatedAt.Time
		tok.CreatedAt = &ts
	}
	return tok, nil
}

func (r *Repo) DeleteByEmail(ctx context.Context, email string) error {
	return r.q.DeletePasswordResetTokenByEmail(ctx, email)
}

package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// openTestStore connects to the dev database, skipping the test when it is
// unreachable or the schema is not migrated.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := database.OpenPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		t.Skipf("db: %v", err)
	}
	t.Cleanup(pool.Close)
	var reg *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.sessions')").Scan(&reg); err != nil || reg == nil {
		t.Skipf("sessions table not migrated: %v", err)
	}
	return NewStore(generated.New(pool))
}

func TestStoreRoundTrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	ip, ua := "127.0.0.1", "test-agent"
	now := time.Now()

	err = st.Create(ctx, generated.UpsertSessionParams{
		ID:           id,
		IpAddress:    &ip,
		UserAgent:    &ua,
		Payload:      "payload-v1",
		LastActivity: int32(now.Unix()),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Destroy(context.Background(), id) })

	sess, err := st.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Payload != "payload-v1" || sess.IpAddress == nil || *sess.IpAddress != ip || sess.UserID != nil {
		t.Errorf("unexpected session: %+v", sess)
	}

	err = st.Save(ctx, generated.UpsertSessionParams{
		ID:           id,
		IpAddress:    &ip,
		UserAgent:    &ua,
		Payload:      "payload-v2",
		LastActivity: int32(now.Add(time.Minute).Unix()),
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err = st.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Payload != "payload-v2" {
		t.Errorf("Payload = %q after save, want payload-v2", sess.Payload)
	}

	if err := st.Destroy(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load after destroy err = %v, want ErrNotFound", err)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.Load(context.Background(), "no-such-session"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestStoreGC(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Now()

	oldID, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	freshID, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Destroy(context.Background(), oldID)
		st.Destroy(context.Background(), freshID)
	})

	for _, s := range []struct {
		id           string
		lastActivity int32
	}{
		{oldID, int32(now.Add(-3 * time.Hour).Unix())}, // older than 120min lifetime
		{freshID, int32(now.Unix())},
	} {
		if err := st.Create(ctx, generated.UpsertSessionParams{ID: s.id, Payload: "p", LastActivity: s.lastActivity}); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.GC(ctx, now, 120*time.Minute); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Load(ctx, oldID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session err = %v, want ErrNotFound", err)
	}
	if _, err := st.Load(ctx, freshID); err != nil {
		t.Errorf("fresh session err = %v, want nil", err)
	}
}

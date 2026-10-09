// Command seed ports the reference Laravel seeders that back the permission
// system (database/seeders/PermissionsSeeder.php, RolesSeeder.php,
// AdminUserSeeder.php). Idempotent — safe to run repeatedly.
//
// Usage: seed
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/permission"
	"github.com/shuvo-paul/baaku/internal/service/password"
)

// guard is the spatie guard every seeded role/permission belongs to.
const guard = "web"

// Defaults mirror reference/config/auth.php ('default_roles', 'permissions')
// and reference/config/app.php ('seeder' section).
var (
	defaultRoles = []string{"admin", "moderator", "member"}

	builtInPermissions = []string{
		"manage roles",
		"manage permissions",
		"manage members",
		"manage educations",
		"manage committee",
		"manage pages",
		"manage membership plans",
		"manage memberships",
		"view dashboard",
		"view activity log",
	}

	adminName     = envOr("ADMIN_NAME", "Admin")
	adminEmail    = envOr("ADMIN_EMAIL", "admin@example.com")
	adminPassword = envOr("ADMIN_PASSWORD", "password")
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := database.OpenPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := permission.NewRepo(generated.New(pool))

	// PermissionsSeeder: findOrCreate every built-in permission.
	perms := map[string]int64{}
	for _, name := range builtInPermissions {
		id, err := repo.CreateGrant(ctx, name, guard)
		if err != nil {
			return fmt.Errorf("permission %q: %w", name, err)
		}
		perms[name] = id
	}

	// RolesSeeder: findOrCreate roles; admin gets every permission,
	// moderator gets ['manage members', 'view dashboard'].
	roles := map[string]int64{}
	for _, name := range defaultRoles {
		id, err := repo.CreateRole(ctx, name, guard)
		if err != nil {
			return fmt.Errorf("role %q: %w", name, err)
		}
		roles[name] = id
	}
	for _, name := range builtInPermissions {
		if err := repo.GiveRolePermission(ctx, perms[name], roles[defaultRoles[0]]); err != nil {
			return fmt.Errorf("grant %q to admin: %w", name, err)
		}
	}
	for _, name := range []string{"manage members", "view dashboard"} {
		if err := repo.GiveRolePermission(ctx, perms[name], roles[defaultRoles[1]]); err != nil {
			return fmt.Errorf("grant %q to moderator: %w", name, err)
		}
	}

	// AdminUserSeeder: update-or-create the admin user, verified + active,
	// with the admin role assigned.
	if err := seedAdmin(ctx, generated.New(pool), repo, roles[defaultRoles[0]]); err != nil {
		return err
	}

	fmt.Println("seeded permissions, roles, admin user")
	return nil
}

func seedAdmin(ctx context.Context, q *generated.Queries, repo *permission.Repo, adminRoleID int64) error {
	hash, err := password.Hash(adminPassword)
	if err != nil {
		return err
	}
	u, err := q.GetUserByEmail(ctx, adminEmail)
	switch {
	case err == nil:
		if err := q.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{ID: u.ID, Password: hash}); err != nil {
			return err
		}
		if err := q.VerifyUserEmail(ctx, u.ID); err != nil {
			return err
		}
		if err := q.UpdateUserState(ctx, generated.UpdateUserStateParams{ID: u.ID, State: "active"}); err != nil {
			return err
		}
	case errors.Is(err, pgx.ErrNoRows):
		u, err = q.CreateUser(ctx, generated.CreateUserParams{Name: adminName, Email: adminEmail, Password: hash, Phone: nil})
		if err != nil {
			return err
		}
		if err := q.VerifyUserEmail(ctx, u.ID); err != nil {
			return err
		}
		if err := q.UpdateUserState(ctx, generated.UpdateUserStateParams{ID: u.ID, State: "active"}); err != nil {
			return err
		}
	default:
		return err
	}
	return repo.AssignRole(ctx, adminRoleID, u.ID)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

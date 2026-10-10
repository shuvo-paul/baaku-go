// Package config loads application configuration from the environment,
// optionally seeded from a .env file in the working directory.
package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App        App
	Postgres   Postgres
	Session    Session
	Mail       Mail
	Auth       Auth
	Features   Features
	Membership Membership
	Career     Career
	Education  Education
	// TODO: GoogleDrive section comes later — add fields here.
}

// Features mirrors reference/config/features.php, which is hardcoded (not
// env-driven): posts off, committee + memberships on.
type Features struct {
	Posts       bool
	Committee   bool
	Memberships bool
}

// Membership mirrors reference/config/membership.php.
type Membership struct {
	// Currency is the app-wide currency for membership money (e.g. "BDT").
	Currency string
	// GateableFeatures are the feature keys an admin can gate behind a plan
	// (each a toggle in the plan editor).
	GateableFeatures []string
	// ExpiryEnabled / ExpiryAt drive the scheduled membership-expiry sweep.
	ExpiryEnabled bool
	ExpiryAt      string
	// Proof configures the payment-proof upload: max size and allowed mimes.
	ProofMaxKB int64
	ProofMimes []string
}

// Career mirrors reference/config/career.php (hardcoded).
type Career struct {
	// EmploymentTypes maps employment_type value → display label.
	EmploymentTypes []Option
}

// Education mirrors reference/config/education.php (hardcoded) — the
// suggestion lists behind the education form's datalists.
type Education struct {
	Levels       []string
	Institutions []string
	Subjects     []string
}

// Option is one select/datalist entry: value + display label.
type Option struct {
	Value string
	Label string
}
type Mail struct {
	Host     string // MAIL_HOST
	Port     string // MAIL_PORT
	Username string // MAIL_USERNAME
	Password string // MAIL_PASSWORD
	From     string // MAIL_FROM
}

// App mirrors reference/config/app.php (APP_NAME, APP_KEY).
// App mirrors reference/config/app.php (APP_NAME, APP_KEY, APP_URL).
type App struct {
	Name string
	Key  string
	URL  string // APP_URL, base for signed verification links
	// LocalNames mirrors reference/config/app.php 'local_names' (code →
	// label + required flag); drives the localized-name fields.
	LocalNames []LocalName
}

// LocalName is one configured local-name field (reference app.local_names).
type LocalName struct {
	Code     string
	Label    string
	Required bool
}

// KeyBytes returns the decoded APP_KEY (Laravel "base64:" + base64 32 bytes),
// used as the AES-256-GCM key for 2FA secret/recovery-code encryption.
func (a App) KeyBytes() ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(a.Key, "base64:"))
	if err != nil {
		return nil, fmt.Errorf("config: decode APP_KEY: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("config: APP_KEY must decode to 32 bytes, got %d", len(raw))
	}
	return raw, nil
}

type Postgres struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// Session mirrors reference/config/session.php (database driver).
type Session struct {
	Cookie   string // SESSION_COOKIE
	Lifetime int    // SESSION_LIFETIME, minutes
	Secure   bool   // SESSION_SECURE_COOKIE
	HttpOnly bool   // SESSION_HTTP_ONLY
	SameSite string // SESSION_SAME_SITE
}

// Auth mirrors reference/config/auth.php sections used by the port.
type Auth struct {
	PasswordTimeout int // AUTH_PASSWORD_TIMEOUT, seconds (Fortify password.confirm TTL)
	// DefaultRoles mirrors reference/config/auth.php default_roles: index 0 is
	// the admin role the self-demotion guard protects.
	DefaultRoles []string
}

// Load reads .env if present (existing env vars win), then builds Config
// from environment variables matching the Laravel reference app.
func Load() (*Config, error) {
	loadDotEnv(".env")

	cfg := &Config{
		App: App{
			// reference/.env.example ships the stock Laravel default; real
			// deployments set APP_NAME/APP_KEY in .env.
			Name:       getenv("APP_NAME", "Laravel"),
			Key:        getenv("APP_KEY", ""),
			URL:        getenv("APP_URL", "http://localhost"),
			LocalNames: localNames(),
		},
		Postgres: Postgres{
			Host:     getenv("DB_HOST", "127.0.0.1"),
			Port:     getenv("DB_PORT", "5432"),
			User:     getenv("DB_USERNAME", "baaku"),
			Password: getenv("DB_PASSWORD", ""),
			DBName:   getenv("DB_DATABASE", "baaku_db"),
		},
		// Laravel derives the cookie name from APP_NAME; baaku keeps a
		// fixed default here.
		Session: Session{
			Cookie:   getenv("SESSION_COOKIE", "baaku-session"),
			Lifetime: getenvInt("SESSION_LIFETIME", 120),
			Secure:   getenvBool("SESSION_SECURE_COOKIE", false),
			HttpOnly: getenvBool("SESSION_HTTP_ONLY", true),
			SameSite: getenv("SESSION_SAME_SITE", "lax"),
		},
		// Defaults mirror reference/config/mail.php (host 127.0.0.1,
		// port 2525, from hello@example.com).
		Mail: Mail{
			Host:     getenv("MAIL_HOST", "127.0.0.1"),
			Port:     getenv("MAIL_PORT", "2525"),
			Username: getenv("MAIL_USERNAME", ""),
			Password: getenv("MAIL_PASSWORD", ""),
			From:     getenv("MAIL_FROM", "hello@example.com"),
		},
		Auth: Auth{
			PasswordTimeout: getenvInt("AUTH_PASSWORD_TIMEOUT", 10800),
			DefaultRoles:    []string{"admin", "moderator", "member"},
		},
		Features: Features{
			Posts:       false,
			Committee:   true,
			Memberships: true,
		},
		Membership: membershipConfig(),
		Career:     careerConfig(),
		Education:  educationConfig(),
	}
	return cfg, nil
}

// DSN returns the Postgres connection string.
func (p Postgres) DSN() string {
	// An empty password omits the segment entirely: pgx's connstring parser
	// treats the token after an empty "password=" as its value, silently
	// dropping dbname.
	dsn := fmt.Sprintf("host=%s port=%s user=%s", p.Host, p.Port, p.User)
	if p.Password != "" {
		dsn += " password=" + p.Password
	}
	return dsn + " dbname=" + p.DBName
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := getenv(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	if v := getenv(key, ""); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

// loadDotEnv parses a simple KEY=VALUE .env file into the process env without
// overwriting existing variables. No-op if the file does not exist.
// ponytail: no ${VAR} interpolation / export prefix support — add godotenv
// if the .env ever needs either.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}

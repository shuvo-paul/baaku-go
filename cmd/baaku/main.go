package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/logger"
	"github.com/shuvo-paul/baaku/internal/mailer"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/repository/confirm"
	passwordresetrepo "github.com/shuvo-paul/baaku/internal/repository/passwordreset"
	"github.com/shuvo-paul/baaku/internal/repository/profile"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	"github.com/shuvo-paul/baaku/internal/repository/twofactor"
	"github.com/shuvo-paul/baaku/internal/repository/user"
	"github.com/shuvo-paul/baaku/internal/service/completeprofile"
	"github.com/shuvo-paul/baaku/internal/service/emailverify"
	"github.com/shuvo-paul/baaku/internal/service/login"
	"github.com/shuvo-paul/baaku/internal/service/passwordchange"
	"github.com/shuvo-paul/baaku/internal/service/passwordconfirm"
	"github.com/shuvo-paul/baaku/internal/service/passwordreset"
	"github.com/shuvo-paul/baaku/internal/service/profileinfo"
	"github.com/shuvo-paul/baaku/internal/service/register"
	tfasvc "github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/twofactorchallenge"
)

// Built assets (make assets). all: keeps .gitkeep so go build works pre-build.
//
//go:embed all:static
var staticFS embed.FS

func main() {
	l := logger.New()

	cfg, err := config.Load()
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: load config")
	}
	key, err := cfg.App.KeyBytes()
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: APP_KEY (base64: 32 bytes) required — CSRF and 2FA encrypt with it")
	}

	ctx := context.Background()
	pool, err := database.OpenPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: open pool")
	}
	defer pool.Close()
	q := generated.New(pool)

	users := user.NewRepo(q, pool)
	sessStore := session.NewStore(q)
	twofaRepo := twofactor.NewRepo(users, q)
	confirmRepo := confirm.NewRepo(q, time.Duration(cfg.Auth.PasswordTimeout)*time.Second)
	twofa := tfasvc.NewService(twofaRepo, twofaRepo, confirmRepo, key, cfg.App.Name)
	profRepo := profile.NewRepo(q)

	authSvc := login.New(users, sessStore, users)
	challengeSvc := twofactorchallenge.New(sessStore, twofa)
	lh := handler.NewLogin(authSvc, challengeSvc, cfg.Session, cfg.App.Name)

	// Mailer-backed send func for verification + reset emails.
	sendMail := func(to, subject, body string) error {
		return mailer.Send(mailer.Config{
			Host: cfg.Mail.Host, Port: cfg.Mail.Port,
			Username: cfg.Mail.Username, Password: cfg.Mail.Password,
			From: cfg.Mail.From,
		}, to, subject, body)
	}
	rawKey := []byte(cfg.App.Key) // emailverify signs with the raw string, not decoded bytes
	resender := emailverify.NewResender(users, sendMail, cfg.App.URL, rawKey)

	resetTokens := passwordresetrepo.NewRepo(q)
	resetSvc := passwordreset.New(users, resetTokens)
	sendResetLink := func(email, rawToken string) error {
		subject, body, err := mailer.ResetPassword(mailer.ResetPasswordData{
			URL:            cfg.App.URL + "/reset-password/" + rawToken,
			ExpiresMinutes: int(passwordreset.ValidityWindow.Minutes()),
		})
		if err != nil {
			return err
		}
		return sendMail(email, subject, body)
	}

	regH := handler.NewRegister(register.NewRegisterService(users), sessStore, resender, cfg.Session, cfg.App.Name)
	prh := handler.NewPasswordReset(resetSvc, sendResetLink, cfg.App.Name)
	evh := handler.NewEmailVerify(users, resender, rawKey, cfg.App.Name)
	cph := handler.NewConfirmPassword(passwordconfirm.New(users, confirmRepo), cfg.App.Name)
	tfch := handler.NewTwoFactorChallenge(challengeSvc, cfg.Session, cfg.App.Name)
	dashH := handler.NewDashboard(cfg.App.Name)
	uph := handler.NewUpdatePassword(func(ctx context.Context, userID int64, current, newPw, confirm string) error {
		return passwordchange.ChangePassword(ctx, users, userID, current, newPw, confirm)
	})
	proh := handler.NewProfileUpdater(profileinfo.New(users).Update)
	compH := handler.NewCompleteProfile(completeprofile.New(profRepo), profRepo, cfg.App.Name)
	tfsH := handler.NewTwoFactorSettings(twofa)
	profpH := handler.NewProfilePage(twofa, cfg.App.Name)
	r := chi.NewRouter()
	r.Use(middleware.Session(cfg.Session.Cookie, sessStore))
	r.Use(middleware.CSRF(key))
	r.Use(middleware.MethodOverride)

	r.Get("/login", lh.ShowLogin)
	r.With(middleware.LoginThrottle(5, time.Minute)).Post("/login", lh.Login)
	r.Post("/logout", lh.Logout)

	r.Get("/register", regH.Show)
	r.Post("/register", regH.Store)

	r.Get("/forgot-password", prh.ShowForgot)
	r.Post("/forgot-password", prh.Forgot)
	r.Get("/reset-password/{token}", prh.ShowReset)
	r.Post("/reset-password", prh.Reset)

	r.Get("/two-factor-challenge", tfch.Show)
	r.Post("/two-factor-challenge", tfch.Submit)

	authMW := middleware.RequireAuth(users)
	r.With(authMW).Get("/email/verify", evh.Notice)
	r.With(authMW).Get("/email/verify/{id}/{hash}", evh.Verify)
	r.With(authMW).Post("/email/verification-notification", evh.Resend)

	r.With(authMW).Get("/user/confirm-password", cph.Show)
	r.With(authMW).Post("/user/confirm-password", cph.Confirm)
	r.With(authMW).Get("/user/confirmed-password-status", cph.Status)

	r.With(authMW).Put("/user/password", uph.Update)
	r.With(authMW).Put("/user/profile-information", proh.Update)

	// Reference routes/profile.php: profile.complete = auth + verified +
	// user.suspended (the gate middleware lands with the dashboard wiring).
	completeMW := r.With(authMW, middleware.RequireVerified, middleware.CheckUserSuspended)
	completeMW.Get("/profile/complete", compH.Show)
	completeMW.Post("/profile/complete", compH.Store)

	// Profile page (reference dashboard/profile: auth + verified +
	// complete-profile.check + user.suspended) — security tab only for now;
	// details/educations/careers tabs land in later waves.
	r.With(authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), middleware.CheckUserSuspended).Get("/dashboard/profile", profpH.Show)

	// Two-factor management (Fortify twoFactorAuthentication with
	// confirmPassword => true): every route behind auth + password.confirm.
	tfmw := r.With(authMW, middleware.RequirePasswordConfirmation(confirmRepo))
	tfmw.Post("/user/two-factor-authentication", tfsH.Enable)
	tfmw.Post("/user/confirmed-two-factor-authentication", tfsH.Confirm)
	tfmw.Delete("/user/two-factor-authentication", tfsH.Disable)
	tfmw.Get("/user/two-factor-qr-code", tfsH.QRCode)
	tfmw.Post("/user/two-factor-recovery-codes", tfsH.Regenerate)
	// Reference routes/dashboard.php protected group: auth + verified +
	// complete-profile.check. Suspended users keep /dashboard access; their
	// sub-routes add user.suspended, and posts/users add user.approved —
	// those routes land in later waves and get the guards there.
	r.With(authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo)).Get("/dashboard", dashH.Show)

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		l.Fatal().Err(err).Msg("baaku: static fs")
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	addr := ":" + port()
	l.Info().Str("addr", addr).Msg("baaku: listening")
	if err := http.ListenAndServe(addr, r); err != nil {
		l.Fatal().Err(err).Msg("baaku: serve")
	}
}

func port() string {
	if v := os.Getenv("APP_PORT"); v != "" {
		return v
	}
	return "8000"
}

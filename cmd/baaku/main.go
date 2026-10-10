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
	activitylogrepo "github.com/shuvo-paul/baaku/internal/repository/activitylog"
	careerrepo "github.com/shuvo-paul/baaku/internal/repository/career"
	committeerepo "github.com/shuvo-paul/baaku/internal/repository/committee"
	"github.com/shuvo-paul/baaku/internal/repository/confirm"
	educationrepo "github.com/shuvo-paul/baaku/internal/repository/education"
	memberdirectoryrepo "github.com/shuvo-paul/baaku/internal/repository/memberdirectory"
	membershiprepo "github.com/shuvo-paul/baaku/internal/repository/membershiprepo"
	methodrepo "github.com/shuvo-paul/baaku/internal/repository/methodrepo"
	passwordresetrepo "github.com/shuvo-paul/baaku/internal/repository/passwordreset"
	paymentrepo "github.com/shuvo-paul/baaku/internal/repository/paymentrepo"
	permissionrepo "github.com/shuvo-paul/baaku/internal/repository/permission"
	planrepo "github.com/shuvo-paul/baaku/internal/repository/planrepo"
	"github.com/shuvo-paul/baaku/internal/repository/profile"
	rolerepo "github.com/shuvo-paul/baaku/internal/repository/role"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	"github.com/shuvo-paul/baaku/internal/repository/twofactor"
	"github.com/shuvo-paul/baaku/internal/repository/user"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/career"
	committeesvc "github.com/shuvo-paul/baaku/internal/service/committee"
	"github.com/shuvo-paul/baaku/internal/service/completeprofile"
	"github.com/shuvo-paul/baaku/internal/service/education"
	"github.com/shuvo-paul/baaku/internal/service/emailverify"
	"github.com/shuvo-paul/baaku/internal/service/login"
	memberdirectorysvc "github.com/shuvo-paul/baaku/internal/service/memberdirectory"
	membershipsvc "github.com/shuvo-paul/baaku/internal/service/membership"
	membershipgate "github.com/shuvo-paul/baaku/internal/service/membershipgate"
	paysvc "github.com/shuvo-paul/baaku/internal/service/membershippayment"
	methodsvc "github.com/shuvo-paul/baaku/internal/service/membershippaymentmethod"
	plansvc "github.com/shuvo-paul/baaku/internal/service/membershipplan"
	"github.com/shuvo-paul/baaku/internal/service/passwordchange"
	"github.com/shuvo-paul/baaku/internal/service/passwordconfirm"
	"github.com/shuvo-paul/baaku/internal/service/passwordreset"
	permsvc "github.com/shuvo-paul/baaku/internal/service/permission"
	"github.com/shuvo-paul/baaku/internal/service/profiledetails"
	"github.com/shuvo-paul/baaku/internal/service/profileinfo"
	"github.com/shuvo-paul/baaku/internal/service/profilereview"
	"github.com/shuvo-paul/baaku/internal/service/register"
	rolesvc "github.com/shuvo-paul/baaku/internal/service/role"
	tfasvc "github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/twofactorchallenge"
	userrolesvc "github.com/shuvo-paul/baaku/internal/service/userrole"
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
	sessStore := session.NewStore(q, time.Duration(cfg.Session.Lifetime)*time.Minute)
	// Session GC: Laravel sweeps expired rows via the per-request session
	// lottery; a background hourly sweep keeps the table bounded here.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			if err := sessStore.GC(ctx, time.Now()); err != nil {
				l.Warn().Err(err).Msg("baaku: session GC")
			}
			<-ticker.C
		}
	}()
	twofaRepo := twofactor.NewRepo(users, q)
	confirmRepo := confirm.NewRepo(q, time.Duration(cfg.Auth.PasswordTimeout)*time.Second)
	twofa := tfasvc.NewService(twofaRepo, twofaRepo, confirmRepo, key, cfg.App.Name)
	profRepo := profile.NewRepo(q)
	// Wave 1: activity log (spatie port) + permission-gated dashboard nav.
	activityRepo := activitylogrepo.NewRepo(q)
	activityLog := activitylog.New(activityRepo)
	permRepo := permissionrepo.NewRepo(q)
	permSvc := permsvc.New(permRepo)
	// Wave 2: roles CRUD (reference RoleController) behind "manage roles".
	roleRepo := rolerepo.NewRepo(q)
	roleSvc := rolesvc.New(roleRepo, activityLog)
	// Wave 6: committee members + positions (reference CommitteeController +
	// PositionController) behind "manage committee".
	committeeRepo := committeerepo.NewRepo(q)
	committeeSvc := committeesvc.New(committeeRepo)

	authSvc := login.New(users, sessStore, users)
	challengeSvc := twofactorchallenge.New(sessStore, twofa, users)
	lh := handler.NewLogin(authSvc, challengeSvc, cfg.Session, cfg.App.Name, key)

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

	regH := handler.NewRegister(register.NewRegisterService(users), sessStore, resender, cfg.Session, cfg.Education, cfg.App.Name)
	prh := handler.NewPasswordReset(resetSvc, sendResetLink, cfg.App.Name)
	evh := handler.NewEmailVerify(users, resender, rawKey, cfg.App.Name)
	cph := handler.NewConfirmPassword(passwordconfirm.New(users, confirmRepo), cfg.App.Name)
	tfch := handler.NewTwoFactorChallenge(challengeSvc, cfg.Session, cfg.App.Name, key)
	dashH := handler.NewDashboard(cfg.App.Name)
	actH := handler.NewActivityLog(activityLog, cfg.App.Name)
	roleH := handler.NewRoleAdmin(roleSvc, cfg.App.Name)
	uph := handler.NewUpdatePassword(func(ctx context.Context, userID int64, current, newPw, confirm string) error {
		return passwordchange.ChangePassword(ctx, users, userID, current, newPw, confirm)
	})
	proh := handler.NewProfileUpdater(profileinfo.New(users).Update)
	compH := handler.NewCompleteProfile(completeprofile.New(profRepo, activityLog), profRepo, cfg.App.Name)
	tfsH := handler.NewTwoFactorSettings(twofa)
	// Wave 2: profile details + education/career CRUD.
	educationRepo := educationrepo.NewRepo(q)
	careerRepo := careerrepo.NewRepo(q)
	review := profilereview.New(users, activityLog)
	educationSvc := education.New(educationRepo, review)
	careerSvc := career.New(careerRepo, review)
	// Wave 3: member directory + membership-state management (reference
	// UserRoleController + UserStateController). The membership-feature gate
	// (membership:members middleware) lands with the memberships wave.
	memberRepo := memberdirectoryrepo.NewRepo(q)
	membersSvc := memberdirectorysvc.New(memberRepo, educationSvc, careerSvc, activityLog, sendMail, cfg.App.URL)
	// Wave 4: assign roles to a member (reference UserRoleController@edit/update).
	userRolesSvc := userrolesvc.New(users, roleRepo, roleRepo, activityLog, cfg.Auth.DefaultRoles)
	userRolesH := handler.NewUserRoles(userRolesSvc, cfg.App.Name)

	// Wave 5: memberships (reference MembershipController +
	// MyMembershipController). The membership-feature gate redirects locked
	// members to their membership page.
	membershipRepo := membershiprepo.NewRepo(q, pool)
	planRepo := planrepo.NewRepo(q)
	payRepo := paymentrepo.NewRepo(q)
	methodRepo := methodrepo.NewRepo(q)
	planSvc := plansvc.New(planRepo)
	methodSvc := methodsvc.New(methodRepo)
	membershipSvc := membershipsvc.New(membershipRepo, planSvc, activityLog)
	paySvc := paysvc.New(payRepo, membershipSvc, planSvc, methodSvc, activityLog)
	// The member show page needs the target's membership + payments for the
	// admin summary box (reference users/show.blade.php).
	membersH := handler.NewMembers(membersSvc, membershipSvc, paySvc, cfg.Features.Memberships, cfg.App.Name)
	membershipGate := membershipgate.New(cfg.Features.Memberships, membershipSvc)
	myMemH := handler.NewMyMembership(membershipSvc, paySvc, planSvc, methodSvc, cfg, cfg.App.Name)
	planH := handler.NewPlanAdmin(planSvc, cfg, cfg.App.Name)
	methodH := handler.NewMethodAdmin(methodSvc, cfg.App.Name)
	payH := handler.NewPaymentAdmin(paySvc, planSvc, methodSvc, cfg, cfg.App.Name)
	memH := handler.NewMembershipAdmin(membershipSvc, paySvc, cfg, cfg.App.Name)
	proofH := handler.NewMediaMembershipProof()
	profileDetailsSvc := profiledetails.New(users, profRepo, review)
	peh := handler.NewProfileEducation(educationSvc, cfg, cfg.App.Name)
	pch := handler.NewProfileCareer(careerSvc, cfg, cfg.App.Name)
	pdh := handler.NewProfileDetails(profileDetailsSvc, cfg, cfg.App.Name)
	profpH := handler.NewProfilePage(twofa, profRepo, educationRepo, careerRepo, cfg, cfg.App.Name)
	r := chi.NewRouter()
	r.Use(middleware.Session(cfg.Session.Cookie, sessStore))
	r.Use(middleware.Remember(cfg.Session, key, users, sessStore))
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
	// Fortify ships throttle:6,1 on both verification routes.
	verifyMW := r.With(authMW, middleware.Throttle(6, time.Minute))
	verifyMW.Get("/email/verify/{id}/{hash}", evh.Verify)
	verifyMW.Post("/email/verification-notification", evh.Resend)

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

	// LoadPermissions stashes the viewer's permission names so the sidebar
	// can gate nav items (Activity Log now; roles/users/… as they ship) the
	// same way the reference's layouts/dashboard.blade.php calls can().
	loadPerms := middleware.LoadPermissions(permRepo)
	// Profile page (reference dashboard/profile: auth + verified +
	// complete-profile.check + user.suspended) — security tab only for now;
	// details/educations/careers tabs land in later waves.
	profileMW := []func(http.Handler) http.Handler{authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), middleware.CheckUserSuspended, loadPerms}
	r.With(profileMW...).Get("/dashboard/profile", profpH.Show)

	// Wave 2 routes (reference routes/dashboard.php profile group): details
	// update + user-owned education/career CRUD, all behind the same guards.
	r.With(profileMW...).Put("/dashboard/profile/details", pdh.Update)
	r.With(profileMW...).Get("/dashboard/profile/educations/create", peh.Create)
	r.With(profileMW...).Post("/dashboard/profile/educations", peh.Store)
	r.With(profileMW...).Get("/dashboard/profile/educations/{id}/edit", peh.Edit)
	r.With(profileMW...).Put("/dashboard/profile/educations/{id}", peh.Update)
	r.With(profileMW...).Delete("/dashboard/profile/educations/{id}", peh.Destroy)
	r.With(profileMW...).Get("/dashboard/profile/careers/create", pch.Create)
	r.With(profileMW...).Post("/dashboard/profile/careers", pch.Store)
	r.With(profileMW...).Get("/dashboard/profile/careers/{id}/edit", pch.Edit)
	r.With(profileMW...).Put("/dashboard/profile/careers/{id}", pch.Update)
	r.With(profileMW...).Delete("/dashboard/profile/careers/{id}", pch.Destroy)

	// Profile photos streamed from the public disk (reference
	// MediaController@profilePhoto — no storage:link required).
	mediaH := handler.NewMediaProfilePhoto()
	r.Get("/media/profile-photos/{file}", mediaH.Show)
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
	r.With(authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), loadPerms).Get("/dashboard", dashH.Show)

	// Activity log (reference routes/dashboard.php: dashboard group →
	// user.suspended → permission:view activity log).
	r.With(authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), middleware.CheckUserSuspended, loadPerms, middleware.RequirePermission(permSvc, "view activity log")).Get("/dashboard/activity-log", actH.Show)

	// Roles CRUD (reference routes/dashboard.php: resource except show, inside
	// permission:manage roles → dashboard group → user.suspended).
	roleMW := []func(http.Handler) http.Handler{authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), middleware.CheckUserSuspended, loadPerms, middleware.RequirePermission(permSvc, "manage roles")}
	r.With(roleMW...).Get("/dashboard/roles", roleH.Index)
	r.With(roleMW...).Get("/dashboard/roles/create", roleH.Create)
	r.With(roleMW...).Post("/dashboard/roles", roleH.Store)
	r.With(roleMW...).Get("/dashboard/roles/{id}/edit", roleH.Edit)
	r.With(roleMW...).Put("/dashboard/roles/{id}", roleH.Update)
	r.With(roleMW...).Delete("/dashboard/roles/{id}", roleH.Destroy)

	// Committee + positions (reference routes/dashboard.php: committee group
	// behind permission:manage committee → user.suspended).
	committeeH := handler.NewCommitteeAdmin(committeeSvc, cfg.App.Name)
	positionsH := handler.NewPositionsAdmin(committeeSvc, cfg.App.Name)
	committeeMW := []func(http.Handler) http.Handler{authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), middleware.CheckUserSuspended, loadPerms, middleware.RequirePermission(permSvc, "manage committee")}
	r.With(committeeMW...).Get("/dashboard/committee", committeeH.Index)
	r.With(committeeMW...).Get("/dashboard/committee/create", committeeH.Create)
	r.With(committeeMW...).Post("/dashboard/committee", committeeH.Store)
	r.With(committeeMW...).Get("/dashboard/committee/users/search", committeeH.SearchUsers)
	r.With(committeeMW...).Get("/dashboard/committee/{id}/edit", committeeH.Edit)
	r.With(committeeMW...).Put("/dashboard/committee/{id}", committeeH.Update)
	r.With(committeeMW...).Delete("/dashboard/committee/{id}", committeeH.Destroy)
	r.With(committeeMW...).Post("/dashboard/committee/reorder", committeeH.Reorder)
	r.With(committeeMW...).Get("/dashboard/positions", positionsH.Index)
	r.With(committeeMW...).Get("/dashboard/positions/create", positionsH.Create)
	r.With(committeeMW...).Post("/dashboard/positions", positionsH.Store)
	r.With(committeeMW...).Get("/dashboard/positions/{id}/edit", positionsH.Edit)
	r.With(committeeMW...).Put("/dashboard/positions/{id}", positionsH.Update)
	r.With(committeeMW...).Delete("/dashboard/positions/{id}", positionsH.Destroy)

	// Committee photos streamed from the public disk (reference
	// MediaController@committeePhoto).
	committeePhotoH := handler.NewMediaCommitteePhoto()
	r.Get("/media/committee-photos/{file}", committeePhotoH.Show)

	// Public pages (reference routes/web.php): homepage + committee.
	homeH := handler.NewHomepage(committeeSvc)
	committeeFullH := handler.NewCommitteeFull(committeeSvc)
	r.Get("/", homeH.Show)
	r.Get("/committee", committeeFullH.Show)

	// Member directory + state management (reference routes/dashboard.php:
	// users.index/show behind user.approved inside the dashboard group;
	// users.state.update behind permission:manage members). The reference's
	// membership:members middleware needs the memberships feature (Wave 5).
	memberMW := []func(http.Handler) http.Handler{authMW, middleware.RequireVerified, middleware.CompleteProfileCheck(profRepo), middleware.CheckUserSuspended, middleware.CheckUserApproved, loadPerms}
	// users.index/show are behind the membership:members feature gate
	// (reference); staff who administer memberships bypass it.
	usersGateMW := append(memberMW, middleware.RequireMembershipFeature(membershipGate, "members"))
	r.With(usersGateMW...).Get("/dashboard/users", membersH.Index)
	r.With(usersGateMW...).Get("/dashboard/users/{id}", membersH.Show)
	r.With(append(memberMW, middleware.RequirePermission(permSvc, "manage members"))...).Put("/dashboard/users/{id}/state", membersH.UpdateState)
	// Assign roles to a member (reference users.roles.edit/update): both sit
	// behind permission:manage members in the same route group as
	// users.state.update.
	r.With(append(memberMW, middleware.RequirePermission(permSvc, "manage members"))...).Get("/dashboard/users/{id}/roles", userRolesH.Edit)
	r.With(append(memberMW, middleware.RequirePermission(permSvc, "manage members"))...).Put("/dashboard/users/{id}/roles", userRolesH.Update)

	// Wave 5: memberships. Member-facing routes sit inside the dashboard
	// group (auth + verified + complete-profile); the admin routes are behind
	// their membership permissions (reference routes/dashboard.php).
	// Media: payment proofs streamed from the public disk.
	r.Get("/media/membership-proofs/{file}", proofH.Show)

	// Member-facing (MyMembershipController).
	r.With(memberMW...).Get("/dashboard/membership", myMemH.Show)
	r.With(memberMW...).Get("/dashboard/membership/plans", myMemH.Plans)
	r.With(memberMW...).Get("/dashboard/membership/payments/create", myMemH.CreatePayment)
	r.With(memberMW...).Post("/dashboard/membership/payments", myMemH.StorePayment)
	r.With(memberMW...).Get("/dashboard/membership/payments/{id}", myMemH.ShowPayment)

	// Admin plans + payment methods (reference permission:manage membership
	// plans).
	planMW := append(memberMW, middleware.RequirePermission(permSvc, "manage membership plans"))
	r.With(planMW...).Get("/dashboard/plans", planH.Index)
	r.With(planMW...).Get("/dashboard/plans/create", planH.Create)
	r.With(planMW...).Post("/dashboard/plans", planH.Store)
	r.With(planMW...).Get("/dashboard/plans/{id}/edit", planH.Edit)
	r.With(planMW...).Put("/dashboard/plans/{id}", planH.Update)
	r.With(planMW...).Delete("/dashboard/plans/{id}", planH.Destroy)
	r.With(planMW...).Post("/dashboard/plans/reorder", planH.Reorder)
	r.With(planMW...).Get("/dashboard/payment-methods", methodH.Index)
	r.With(planMW...).Get("/dashboard/payment-methods/create", methodH.Create)
	r.With(planMW...).Post("/dashboard/payment-methods", methodH.Store)
	r.With(planMW...).Get("/dashboard/payment-methods/{id}/edit", methodH.Edit)
	r.With(planMW...).Put("/dashboard/payment-methods/{id}", methodH.Update)
	r.With(planMW...).Delete("/dashboard/payment-methods/{id}", methodH.Destroy)

	// Admin memberships + payments (reference permission:manage memberships).
	memberAdminMW := append(memberMW, middleware.RequirePermission(permSvc, "manage memberships"))
	r.With(memberAdminMW...).Get("/dashboard/memberships", memH.Index)
	r.With(memberAdminMW...).Get("/dashboard/memberships/{id}", memH.Show)
	r.With(memberAdminMW...).Put("/dashboard/memberships/{id}", memH.Update)
	r.With(memberAdminMW...).Post("/dashboard/memberships/{id}/cancel", memH.Cancel)
	r.With(memberAdminMW...).Get("/dashboard/payments", payH.Index)
	r.With(memberAdminMW...).Get("/dashboard/payments/create", payH.Create)
	r.With(memberAdminMW...).Post("/dashboard/payments", payH.Store)
	r.With(memberAdminMW...).Get("/dashboard/payments/{id}", payH.Show)
	r.With(memberAdminMW...).Post("/dashboard/payments/{id}/approve", payH.Approve)
	r.With(memberAdminMW...).Post("/dashboard/payments/{id}/reject", payH.Reject)

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

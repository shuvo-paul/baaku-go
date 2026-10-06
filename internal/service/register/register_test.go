package register_test

import (
	"strings"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/register"
)

func validInput() register.RegisterInput {
	return register.RegisterInput{
		Name:                 "New Member",
		Email:                "new@example.com",
		Phone:                "01700000000",
		Password:             "Password1!",
		PasswordConfirmation: "Password1!",
		Educations: []register.EducationInput{{
			Level:       "Honors",
			Institution: "Dhaka University",
			Subject:     "Bangla",
			StartYear:   2018,
			IsCurrent:   true,
		}},
	}
}

func strptr(s string) *string { return &s }
func i16ptr(v int16) *int16   { return &v }
func i32ptr(v int32) *int32   { return &v }

func TestRegisterInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*register.RegisterInput)
		wantKey string // expected error key; "" means valid
	}{
		{"valid input", func(*register.RegisterInput) {}, ""},
		{"name required", func(i *register.RegisterInput) { i.Name = "" }, "name"},
		{"name digits rejected", func(i *register.RegisterInput) { i.Name = "John123" }, "name"},
		{"name punctuation rejected", func(i *register.RegisterInput) { i.Name = "O'Brien" }, "name"},
		{"name over 255", func(i *register.RegisterInput) { i.Name = strings.Repeat("a", 256) }, "name"},
		{"name allows multiple spaces", func(i *register.RegisterInput) { i.Name = "Jean  Luc" }, ""},

		{"email required", func(i *register.RegisterInput) { i.Email = "" }, "email"},
		{"email invalid", func(i *register.RegisterInput) { i.Email = "not-an-email" }, "email"},
		{"email over 255", func(i *register.RegisterInput) { i.Email = strings.Repeat("a", 251) + "@x.co" }, "email"},

		{"phone required", func(i *register.RegisterInput) { i.Phone = "" }, "phone"},
		{"phone over 20", func(i *register.RegisterInput) { i.Phone = strings.Repeat("1", 21) }, "phone"},

		{"password required", func(i *register.RegisterInput) { i.Password = ""; i.PasswordConfirmation = "" }, "password"},
		{"password confirmation mismatch", func(i *register.RegisterInput) { i.PasswordConfirmation = "Other1!" }, "password"},
		{"password too short", func(i *register.RegisterInput) { i.Password = "Pw1!"; i.PasswordConfirmation = "Pw1!" }, "password"},
		{"password no uppercase", func(i *register.RegisterInput) { i.Password = "password1!"; i.PasswordConfirmation = "password1!" }, "password"},
		{"password no lowercase", func(i *register.RegisterInput) { i.Password = "PASSWORD1!"; i.PasswordConfirmation = "PASSWORD1!" }, "password"},
		{"password no digit", func(i *register.RegisterInput) { i.Password = "Password!!"; i.PasswordConfirmation = "Password!!" }, "password"},
		{"password no symbol", func(i *register.RegisterInput) { i.Password = "Password11"; i.PasswordConfirmation = "Password11" }, "password"},

		{"educations required", func(i *register.RegisterInput) { i.Educations = nil }, "educations"},
		{"education level required", func(i *register.RegisterInput) { i.Educations[0].Level = "" }, "educations.0.level"},
		{"education institution required", func(i *register.RegisterInput) { i.Educations[0].Institution = "" }, "educations.0.institution"},
		{"education subject required", func(i *register.RegisterInput) { i.Educations[0].Subject = "" }, "educations.0.subject"},
		{"education student_id optional", func(i *register.RegisterInput) { i.Educations[0].StudentID = strptr("S-1") }, ""},
		{"education student_id over 255", func(i *register.RegisterInput) { i.Educations[0].StudentID = strptr(strings.Repeat("s", 256)) }, "educations.0.student_id"},
		{"education start_year required", func(i *register.RegisterInput) { i.Educations[0].StartYear = 0 }, "educations.0.start_year"},
		{"education start_year not 4 digits", func(i *register.RegisterInput) { i.Educations[0].StartYear = 99 }, "educations.0.start_year"},
		{"education start_month in range ok", func(i *register.RegisterInput) { i.Educations[0].StartMonth = i16ptr(12) }, ""},
		{"education start_month out of range", func(i *register.RegisterInput) { i.Educations[0].StartMonth = i16ptr(13) }, "educations.0.start_month"},
		{"education start_month zero", func(i *register.RegisterInput) { i.Educations[0].StartMonth = i16ptr(0) }, "educations.0.start_month"},
		{"education end_month out of range", func(i *register.RegisterInput) { i.Educations[0].EndMonth = i16ptr(13) }, "educations.0.end_month"},

		// end_year-unless-current rule (CreateNewUser after-hook)
		{"end_year required when not current", func(i *register.RegisterInput) { i.Educations[0].IsCurrent = false }, "educations.0.end_year"},
		{"end_year ok when not current", func(i *register.RegisterInput) {
			i.Educations[0].IsCurrent = false
			i.Educations[0].EndYear = i32ptr(2022)
		}, ""},
		{"end_year ok when not current with month", func(i *register.RegisterInput) {
			i.Educations[0].IsCurrent = false
			i.Educations[0].EndYear = i32ptr(2022)
			i.Educations[0].EndMonth = i16ptr(6)
		}, ""},
		{"end_year nullable when current", func(i *register.RegisterInput) { i.Educations[0].IsCurrent = true }, ""},
		{"end_year not 4 digits", func(i *register.RegisterInput) { i.Educations[0].EndYear = i32ptr(99) }, "educations.0.end_year"},
		{"end_year before start_year", func(i *register.RegisterInput) { i.Educations[0].EndYear = i32ptr(2017) }, "educations.0.end_year"},
		{"end_year equal start_year ok", func(i *register.RegisterInput) { i.Educations[0].EndYear = i32ptr(2018) }, ""},
		{"second education index keyed", func(i *register.RegisterInput) {
			i.Educations = append(i.Educations, register.EducationInput{
				Level: "Honors", Institution: "DU", Subject: "Bangla", StartYear: 2018,
			})
		}, "educations.1.end_year"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			errs := in.Validate()
			if tt.wantKey == "" {
				if errs != nil {
					t.Fatalf("expected valid, got %v", errs)
				}
				return
			}
			if errs == nil {
				t.Fatalf("expected error for %q, got none", tt.wantKey)
			}
			if _, ok := errs[tt.wantKey]; !ok {
				t.Fatalf("expected error key %q, got %v", tt.wantKey, errs)
			}
		})
	}
}

func TestFieldErrorsError(t *testing.T) {
	e := register.FieldErrors{"phone": "The phone field is required.", "name": "The name field is required."}
	if got := e.Error(); got != "The name field is required.; The phone field is required." {
		t.Fatalf("unexpected joined message: %q", got)
	}
}

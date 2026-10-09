package views

import (
	"strings"
	"unicode"
)

// View models for the profile page and the education/career forms. These are
// dumb data holders — handlers map service/repo domain types into them.

// ProfileUser is the parent-user slice the details form needs.
type ProfileUser struct {
	Name  string
	Email string
	Phone string
}

// ProfileData is the profile row the details form needs. JSON columns arrive
// as plain maps; DateOfBirth is YYYY-MM-DD ("" when unset).
type ProfileData struct {
	PhotoURL         string
	DateOfBirth      string
	Gender           string
	BloodGroup       string
	PresentAddress   string
	PermanentAddress string
	Website          string
	SocialLinks      map[string]string
	EmergencyContact map[string]string
	LocalNames       map[string]string
}

// LocalNameField is one configured localized-name input.
type LocalNameField struct {
	Code     string
	Label    string
	Required bool
	Value    string
}

// EducationView is one education row for the list + edit form.
type EducationView struct {
	ID          int64
	Level       string
	Institution string
	StudentID   string
	Subject     string
	IsCurrent   bool
	StartYear   int32
	StartMonth  int16
	EndYear     int32
	EndMonth    int16
}

// CareerView is one career row for the list + edit form.
type CareerView struct {
	ID             int64
	EmploymentType string
	JobTitle       string
	Company        string
	Industry       string
	Location       string
	StartYear      int32
	StartMonth     int16
	IsCurrent      bool
	EndYear        int32
	EndMonth       int16
	Description    string
}

// SelectOption is a value/label pair for a <select> (alias of the component
// Option so handlers build slices directly).
type SelectOption = Option

// GenderOptions mirrors app/Enums/Gender::options().
func GenderOptions() []Option {
	return []Option{
		{"male", "Male"},
		{"female", "Female"},
		{"other", "Other"},
		{"prefer_not_to_say", "Prefer not to say"},
	}
}

// BloodGroupOptions mirrors app/Enums/BloodGroup::options().
func BloodGroupOptions() []Option {
	return []Option{
		{"A+", "A+"}, {"A-", "A-"}, {"B+", "B+"}, {"B-", "B-"},
		{"AB+", "AB+"}, {"AB-", "AB-"}, {"O+", "O+"}, {"O-", "O-"},
	}
}

// YearOptions renders the 4-digit year select options (1900–2099), newest
// first. The selected year is highlighted.
func YearOptions(selected int32) []YearOption {
	out := make([]YearOption, 0, 200)
	for y := int32(2099); y >= 1900; y-- {
		out = append(out, YearOption{Value: y, Selected: y == selected})
	}
	return out
}

// YearOption is one <option> in a year select.
type YearOption struct {
	Value    int32
	Selected bool
}

// MonthOptions renders the 1–12 month select options; selected highlights one
// (0 = none).
func MonthOptions(selected int16) []MonthOption {
	out := make([]MonthOption, 0, 12)
	for m := int16(1); m <= 12; m++ {
		out = append(out, MonthOption{Value: m, Selected: m == selected})
	}
	return out
}

// MonthOption is one <option> in a month select.
type MonthOption struct {
	Value    int16
	Selected bool
}

// monthName short display names for months (Jan–Dec).
func monthName(m int16) string {
	names := [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	if m >= 1 && m <= 12 {
		return names[m-1]
	}
	return ""
}

// initials mirrors Illuminate\Support\Str::initials — up to the first letters
// of the first two words, uppercased. Drives the empty photo-box monogram.
func initials(name string) string {
	words := strings.Fields(name)
	var b strings.Builder
	for _, w := range words {
		if b.Len() >= 2 {
			break
		}
		r := []rune(w)
		if len(r) > 0 {
			b.WriteRune(unicode.ToUpper(r[0]))
		}
	}
	return b.String()
}

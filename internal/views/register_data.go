package views

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EducationRow is one educations[i] array entry for the register wizard's
// Alpine state (reference auth/register.blade.php $oldEducations). Year/month
// stay strings so they bind directly to the form controls.
type EducationRow struct {
	Level       string `json:"level"`
	Institution string `json:"institution"`
	StudentID   string `json:"student_id"`
	Subject     string `json:"subject"`
	StartYear   string `json:"start_year"`
	StartMonth  string `json:"start_month"`
	IsCurrent   bool   `json:"is_current"`
	EndYear     string `json:"end_year"`
	EndMonth    string `json:"end_month"`
}

// RegisterData carries the register wizard's server state: old input and
// Laravel-style field errors (reference $errors + old()). Suggestion lists
// come from config.Education (reference config/education.php).
type RegisterData struct {
	Name         string
	Email        string
	Phone        string
	Errors       map[string]string
	Educations   []EducationRow
	Levels       []string
	Institutions []string
	Subjects     []string
}

// initialStep mirrors the reference blade: education errors keep the wizard
// on step 1; anything else jumps to the account-details step.
func (d RegisterData) initialStep() int {
	for k := range d.Errors {
		if strings.HasPrefix(k, "educations") {
			return 1
		}
	}
	if len(d.Errors) > 0 {
		return 2
	}
	return 1
}

// registerYears mirrors reference x-year-select's range: current year + 5
// down to 1991.
func registerYears() []string {
	out := []string{}
	for y := time.Now().Year() + 5; y >= 1991; y-- {
		out = append(out, fmt.Sprintf("%d", y))
	}
	return out
}

// registerMonths mirrors reference x-month-select: full month names 1–12.
func registerMonths() []string {
	return []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
}

// itoa renders a month number as its option value.
func itoa(n int) string { return strconv.Itoa(n) }

// registerXData builds the wizard's Alpine x-data (reference
// auth/register.blade.php x-data object). Server errors seed the same shape
// the client-side validator writes: field → [message].
func registerXData(d RegisterData) string {
	edus := d.Educations
	if len(edus) == 0 {
		edus = []EducationRow{{}}
	}
	errsArr := make(map[string][]string, len(d.Errors))
	for k, v := range d.Errors {
		errsArr[k] = []string{v}
	}
	edusJSON, _ := json.Marshal(edus)    // cannot fail: fixed struct fields
	errsJSON, _ := json.Marshal(errsArr) // cannot fail: string keys/values
	return fmt.Sprintf(registerXDataFormat, d.initialStep(), edusJSON, errsJSON)
}

// registerXDataFormat is the Alpine wizard object; %d = initial step,
// %s = educations array, %s = field-error map. Message strings mirror the
// register service's validation wording.
const registerXDataFormat = `{
	step: %d,
	educations: %s,
	errors: %s,
	eager: {},
	toKey(name) { return String(name || '').replace(/\[([^\]]+)\]/g, '.$1'); },
	fieldError(key) { return (this.errors[this.toKey(key)] || [])[0] || null; },
	validateStep(s) {
		this.clearStepErrors(s);
		let valid = true;
		if (s === 1) {
			this.educations.forEach((edu, i) => {
				const p = 'educations.' + i + '.';
				if (!edu.level) { this.errors[p + 'level'] = ['The level field is required.']; valid = false; }
				if (!edu.institution) { this.errors[p + 'institution'] = ['The institution field is required.']; valid = false; }
				if (!edu.subject) { this.errors[p + 'subject'] = ['The subject field is required.']; valid = false; }
				if (!edu.start_year) { this.errors[p + 'start_year'] = ['The start year field is required.']; valid = false; }
				if (!edu.is_current && !edu.end_year) { this.errors[p + 'end_year'] = ['The end year field is required.']; valid = false; }
			});
		} else if (s === 2) {
			const nameEl = this.$refs.form.querySelector('[name=name]');
			const emailEl = this.$refs.form.querySelector('[name=email]');
			const phoneEl = this.$refs.form.querySelector('[name=phone]');
			const pwEl = this.$refs.form.querySelector('[name=password]');
			const pwConfEl = this.$refs.form.querySelector('[name=password_confirmation]');
			if (nameEl && !nameEl.value.trim()) { this.errors['name'] = ['The name field is required.']; valid = false; } else if (nameEl && !/^[A-Za-z\s]+$/.test(nameEl.value)) { this.errors['name'] = ['Only letters (A–Z) and spaces are allowed.']; valid = false; } else { delete this.errors['name']; }
			if (emailEl && !emailEl.value.trim()) { this.errors['email'] = ['The email field is required.']; valid = false; } else if (emailEl && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailEl.value)) { this.errors['email'] = ['The email field must be a valid email address.']; valid = false; } else { delete this.errors['email']; }
			if (phoneEl && !phoneEl.value.trim()) { this.errors['phone'] = ['The phone field is required.']; valid = false; } else { delete this.errors['phone']; }
			if (pwEl && !pwEl.value) { this.errors['password'] = ['The password field is required.']; valid = false; } else if (pwEl && pwEl.value.length < 8) { this.errors['password'] = ['The password must be at least 8 characters.']; valid = false; } else { delete this.errors['password']; }
			if (pwConfEl && !pwConfEl.value) { this.errors['password_confirmation'] = ['The confirm password field is required.']; valid = false; } else if (pwConfEl && pwEl && pwConfEl.value !== pwEl.value) { this.errors['password_confirmation'] = ['The password field confirmation does not match.']; valid = false; } else { delete this.errors['password_confirmation']; }
		}
		return valid;
	},
	clearStepErrors(s) {
		const prefix = s === 1 ? 'educations.' : null;
		Object.keys(this.errors).forEach((k) => { if (prefix && k.startsWith(prefix)) delete this.errors[k]; });
	},
	attemptStep(s) {
		this.eager[s] = true;
		if (this.validateStep(s)) { this.step++; }
	},
	liveValidate() {
		if (this.eager[this.step]) { this.validateStep(this.step); }
	},
	addEducation() {
		this.educations.push({ level: '', institution: '', student_id: '', subject: '', start_year: '', start_month: '', is_current: false, end_year: '', end_month: '' });
	},
	removeEducation(i) { this.educations.splice(i, 1); }
}`

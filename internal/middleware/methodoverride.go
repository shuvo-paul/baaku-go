package middleware

import "net/http"

// MethodOverride mirrors Laravel's POST + _method form spoofing: HTML forms
// can only GET/POST, so PUT/DELETE forms carry a hidden _method. Register it
// after CSRF (the request is still a POST when the token is checked) and
// before routing.
func MethodOverride(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err == nil {
				switch m := r.FormValue("_method"); m {
				case http.MethodPut, http.MethodPatch, http.MethodDelete:
					r.Method = m
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

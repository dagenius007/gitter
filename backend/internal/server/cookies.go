package server

import (
	"fmt"
	"net/http"
	"time"
)

const (
	// CookieName is the name of the session cookie
	CookieName = "zana_session"
	// CookieMaxAge is the duration the cookie is valid (15 minutes)
	CookieMaxAge = 15 * time.Minute
)

// SetSessionCookie sets an HTTP-only session cookie with 15-minute expiration
func SetSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Detect if request is over HTTPS
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	fmt.Println("isSecure", isSecure)
	

	// Use SameSite=None for cross-origin when secure, Lax otherwise
	sameSite := http.SameSiteLaxMode
	if isSecure {
		sameSite = http.SameSiteNoneMode
	}

	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(CookieMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: sameSite,
		Secure:   isSecure,
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie removes the session cookie
func ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
	}
	http.SetCookie(w, cookie)
}

// GetSessionCookie reads the session ID from the cookie
func GetSessionCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

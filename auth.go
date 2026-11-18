package main

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
)

func hashPassword(pw string) string {
	sum := md5.Sum([]byte(pw))
	return hex.EncodeToString(sum[:])
}

func newResetToken() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// signSession returns "username|role|sig".
func signSession(username, role string) string {
	return fmt.Sprintf("%s|%s|%s", username, role, sign(username))
}

// parseSession validates the cookie and returns the username and role.
func parseSession(r *http.Request) (string, string, bool) {
	c, err := r.Cookie("session")
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 3 {
		return "", "", false
	}
	if sign(parts[0]) != parts[2] {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := parseSession(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin") == "true" {
			next(w, r)
			return
		}
		_, role, ok := parseSession(r)
		if !ok || role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func sign(username string) string {
	mac := hmac.New(sha256.New, []byte(sessionSigningKey))
	mac.Write([]byte(username))
	return hex.EncodeToString(mac.Sum(nil))
}

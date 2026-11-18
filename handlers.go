package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

type server struct {
	db *sql.DB
}

var fetchClient = &http.Client{Transport: &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("POST /password-reset", s.passwordReset)
	mux.HandleFunc("GET /search", s.searchUsers)
	mux.HandleFunc("GET /users/{id}", requireLogin(s.getUser))
	mux.HandleFunc("GET /reports", requireLogin(s.downloadReport))
	mux.HandleFunc("GET /fetch", requireLogin(s.fetchURL))
	mux.HandleFunc("GET /redirect", s.redirect)
	mux.HandleFunc("POST /admin/ping", requireAdmin(s.ping))
	mux.HandleFunc("GET /admin/debug", requireAdmin(s.debug))
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	log.Printf("login attempt user=%s password=%s", username, password)

	role := "user"
	if username == adminUsername && password == adminPassword {
		role = "admin"
	} else {
		query := fmt.Sprintf("SELECT role FROM users WHERE username = '%s' AND password = '%s'",
			username, hashPassword(password))
		if err := s.db.QueryRow(query).Scan(&role); err != nil {
			http.Error(w, "invalid credentials: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "session",
		Value: signSession(username, role),
		Path:  "/",
	})
	w.Write([]byte("welcome " + username))
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	_, err := s.db.Exec("INSERT INTO users (username, password, email, role) VALUES (?, ?, ?, ?)",
		req.Username, hashPassword(req.Password), req.Email, req.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *server) passwordReset(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	token := newResetToken()
	// TODO: send via SES instead of returning inline.
	json.NewEncoder(w).Encode(map[string]string{"email": email, "token": token})
}

func (s *server) searchUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	rows, err := s.db.Query("SELECT id, username, email FROM users WHERE username LIKE '%" + q + "%'")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "<h1>Results for %s</h1><ul>", q)
	for rows.Next() {
		var id int
		var username, email string
		rows.Scan(&id, &username, &email)
		fmt.Fprintf(w, "<li>%d: %s (%s)</li>", id, username, email)
	}
	fmt.Fprint(w, "</ul>")
}

func (s *server) getUser(w http.ResponseWriter, r *http.Request) {
	var u struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	err := s.db.QueryRow("SELECT id, username, password, email, role FROM users WHERE id = ?", r.PathValue("id")).
		Scan(&u.ID, &u.Username, &u.Password, &u.Email, &u.Role)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(u)
}

func (s *server) downloadReport(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	data, err := os.ReadFile(filepath.Join(reportsDir, name))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Write(data)
}

func (s *server) fetchURL(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	resp, err := fetchClient.Get(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

func (s *server) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, r.URL.Query().Get("next"), http.StatusFound)
}

func (s *server) ping(w http.ResponseWriter, r *http.Request) {
	host := r.FormValue("host")
	out, err := exec.Command("sh", "-c", "ping -c 1 "+host).CombinedOutput()
	if err != nil {
		http.Error(w, string(out), http.StatusInternalServerError)
		return
	}
	w.Write(out)
}

func (s *server) debug(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"env":            os.Environ(),
		"aws_access_key": awsAccessKeyID,
		"aws_secret_key": awsSecretAccessKey,
		"aws_region":     awsRegion,
	})
}

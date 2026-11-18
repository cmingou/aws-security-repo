package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	db, err := openDB(envOr("DB_PATH", "app.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	(&server{db: db}).routes(mux)

	addr := envOr("LISTEN_ADDR", "127.0.0.1:8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

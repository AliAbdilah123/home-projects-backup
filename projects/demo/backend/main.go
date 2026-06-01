package main

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	os.MkdirAll("backend", 0755)
	db, err := sql.Open("sqlite", filepath.Join("backend", "data.db"))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS visits (id INTEGER PRIMARY KEY, ts TEXT)`); err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/hello", func(w http.ResponseWriter, r *http.Request) {
		if _, err := db.Exec("INSERT INTO visits (ts) VALUES (?)", time.Now().Format(time.RFC3339)); err != nil {
			slog.Error("insert visit", "error", err)
		}

		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM visits").Scan(&count); err != nil {
			slog.Error("count visits", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello from Go backend","visits":` + string(rune('0'+count)) + `}`))
	})

	spaBase := "/"
	dist := http.Dir(filepath.Join("frontend", "dist"))
	fileServer := http.FileServer(dist)

	mux.HandleFunc(spaBase, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			r.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<p>API root</p>"))
	})

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	slog.Info("listening", "addr", ":"+port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		panic(err)
	}
}

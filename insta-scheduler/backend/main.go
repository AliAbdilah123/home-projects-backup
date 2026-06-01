package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type Post struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Caption   *string    `json:"caption"`
	ImageURL  *string    `json:"image_url"`
	Status    string     `json:"status"`
	Scheduled *time.Time `json:"scheduled"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

var db *sql.DB

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		next(w, r)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := env("ADDR", "127.0.0.1:8080")
	dbPath := env("DB_FILE", "scheduler.db")

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS posts(
		id TEXT PRIMARY KEY,
		title TEXT,
		caption TEXT,
		image_url TEXT,
		status TEXT DEFAULT 'draft',
		scheduled TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/posts", withCORS(getPosts))
	mux.HandleFunc("/api/v1/posts/create", withCORS(createPost))
	mux.HandleFunc("/api/v1/posts/update/", withCORS(updatePost))
	mux.HandleFunc("/api/v1/posts/delete/", withCORS(deletePost))
	mux.HandleFunc("/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))

	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func getPosts(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id,title,caption,image_url,status,scheduled,created_at,updated_at FROM posts ORDER BY COALESCE(scheduled, created_at) ASC")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := make([]Post, 0)
	for rows.Next() {
		var p Post
		var caption, imageURL, status, scheduled sql.NullString
		err := rows.Scan(&p.ID, &p.Title, &caption, &imageURL, &status, &scheduled, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if caption.Valid {
			s := caption.String
			p.Caption = &s
		}
		if imageURL.Valid {
			s := imageURL.String
			p.ImageURL = &s
		}
		if status.Valid {
			p.Status = status.String
		}
		if scheduled.Valid {
			t, err := time.Parse(time.RFC3339, scheduled.String)
			if err == nil {
				p.Scheduled = &t
			}
		}
		posts = append(posts, p)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

type CreatePostRequest struct {
	Title     string     `json:"title"`
	Caption   *string    `json:"caption"`
	ImageURL  *string    `json:"image_url"`
	Status    string     `json:"status"`
	Scheduled *time.Time `json:"scheduled"`
}

func createPost(w http.ResponseWriter, r *http.Request) {
	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	p := Post{
		ID:        now.Format("20060102150405") + "-" + randomToken(6),
		Title:     req.Title,
		Caption:   req.Caption,
		ImageURL:  req.ImageURL,
		Status:    req.Status,
		Scheduled: req.Scheduled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if p.Status == "" {
		p.Status = "draft"
	}

	_, err := db.Exec(
		"INSERT INTO posts (id, title, caption, image_url, status, scheduled, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)",
		p.ID, p.Title, p.Caption, p.ImageURL, p.Status, p.Scheduled, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

type UpdatePostRequest struct {
	Title     *string    `json:"title"`
	Caption   *string    `json:"caption"`
	ImageURL  *string    `json:"image_url"`
	Status    *string    `json:"status"`
	Scheduled *time.Time `json:"scheduled"`
}

func updatePost(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/posts/update/"):]
	var req UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var existing struct{ id string }
	if err := db.QueryRow("SELECT id FROM posts WHERE id=?", id).Scan(&existing.id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	set := []string{"updated_at=?"}
	args := []interface{}{time.Now().UTC()}
	if req.Title != nil {
		set = append(set, "title=?")
		args = append(args, *req.Title)
	}
	if req.Caption != nil {
		set = append(set, "caption=?")
		args = append(args, *req.Caption)
	}
	if req.ImageURL != nil {
		set = append(set, "image_url=?")
		args = append(args, *req.ImageURL)
	}
	if req.Status != nil {
		set = append(set, "status=?")
		args = append(args, *req.Status)
	}
	if req.Scheduled != nil {
		set = append(set, "scheduled=?")
		args = append(args, req.Scheduled)
	}
	args = append(args, id)
	_, _ = db.Exec("UPDATE posts SET "+join(set)+" WHERE id=?", args...)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"updated":true}`))
}

func deletePost(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/posts/delete/"):]
	_, err := db.Exec("DELETE FROM posts WHERE id=?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"deleted":true}`))
}

func join(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += "," + s
	}
	return out
}

func randomToken(n int) string {
	const set = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = set[time.Now().Nanosecond()%len(set)]
	}
	return string(b)
}

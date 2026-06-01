package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	_ "modernc.org/sqlite"
)

type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Complexity  string `json:"complexity"`
	Model       string `json:"model"`
	Assignee    string `json:"assignee"`
	Status      string `json:"status"`
}

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite", "file:kanban.db?mode=rwc")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		description TEXT,
		complexity TEXT,
		model TEXT,
		assignee TEXT,
		status TEXT DEFAULT 'todo'
	)`)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	http.HandleFunc("/api/v1/tasks", tasksHandler)

	addr := ":8080"
	if env := os.Getenv("ADDR"); env != "" {
		addr = env
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id, title, description, complexity, model, assignee, status FROM tasks ORDER BY id DESC LIMIT 100`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var out []Task
		for rows.Next() {
			var t Task
			if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Complexity, &t.Model, &t.Assignee, &t.Status); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out = append(out, t)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	case http.MethodPost:
		var in Task
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := db.Exec(`INSERT INTO tasks (title, description, complexity, model, assignee, status) VALUES (?, ?, ?, ?, ?, 'todo')`,
			in.Title, in.Description, in.Complexity, in.Model, in.Assignee)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		in.ID = int(id)
		json.NewEncoder(w).Encode(in)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

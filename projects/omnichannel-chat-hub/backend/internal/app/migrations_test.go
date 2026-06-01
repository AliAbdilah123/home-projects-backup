package app

import (
	"context"
	"testing"
)

func TestMigrationsApplyOnEmptyDatabase(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()

	tables := []string{
		"schema_migrations",
		"users",
		"channels",
		"contacts",
		"conversations",
		"messages",
		"webhook_events",
		"audit_logs",
	}

	for _, table := range tables {
		if !tableExists(t, repo.db, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	dsn := "file::memory:?cache=shared"

	s1, err := NewServer(Config{HTTPAddr: ":0", DatabaseURL: dsn})
	if err != nil {
		t.Fatalf("first NewServer() error = %v", err)
	}
	defer s1.Close()

	s2, err := NewServer(Config{HTTPAddr: ":0", DatabaseURL: dsn})
	if err != nil {
		t.Fatalf("second NewServer() error = %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one migration record")
	}
}

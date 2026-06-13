// Package database provides a small, dependency-free SQL migration runner and
// a seeder for initial data. Migrations are plain .up.sql / .down.sql files
// under migrations/, embedded into the binary and applied step by step inside
// transactions, with applied versions tracked in the schema_migrations table.
package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migration is a single versioned schema change.
type migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// loadMigrations reads every embedded *.sql file and groups up/down halves by
// version. It returns the set sorted ascending by version.
func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	byVersion := map[int]*migration{}
	for _, e := range entries {
		name := e.Name()
		// Expected form: <version>_<title>.<up|down>.sql  e.g. 001_create_users.up.sql
		var direction string
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			direction = "up"
		case strings.HasSuffix(name, ".down.sql"):
			direction = "down"
		default:
			continue
		}

		idx := strings.IndexByte(name, '_')
		if idx < 0 {
			return nil, fmt.Errorf("malformed migration filename: %s", name)
		}
		version, err := strconv.Atoi(name[:idx])
		if err != nil {
			return nil, fmt.Errorf("migration %s: invalid version prefix: %w", name, err)
		}

		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		m := byVersion[version]
		if m == nil {
			title := strings.TrimSuffix(strings.TrimSuffix(name[idx+1:], ".up.sql"), ".down.sql")
			m = &migration{Version: version, Name: title}
			byVersion[version] = m
		}
		if direction == "up" {
			m.Up = string(body)
		} else {
			m.Down = string(body)
		}
	}

	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.Up == "" {
			return nil, fmt.Errorf("migration %03d (%s) has no .up.sql", m.Version, m.Name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// splitStatements breaks a SQL file into individual statements. The pgx stdlib
// driver rejects multiple commands in one Exec, so we run them one at a time.
// Lines that are purely SQL comments are dropped. Our migration files contain
// no semicolons inside string literals, so a naive split on ';' is safe here.
func splitStatements(script string) []string {
	var clean strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		clean.WriteString(line)
		clean.WriteByte('\n')
	}
	var stmts []string
	for _, part := range strings.Split(clean.String(), ";") {
		if s := strings.TrimSpace(part); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}

// ensureMigrationsTable creates the bookkeeping table if it does not exist.
func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    BIGINT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

// appliedVersions returns the set of versions already applied.
func appliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// runInTx executes every statement of script in a single transaction.
func runInTx(db *sql.DB, statements []string, record func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %q: %w", firstLine(stmt), err)
		}
	}
	if record != nil {
		if err := record(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}

// Up applies every pending migration in ascending version order. It is safe to
// call on every startup: already-applied migrations are skipped.
func Up(db *sql.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	pending := 0
	for _, m := range migs {
		if applied[m.Version] {
			continue
		}
		stmts := splitStatements(m.Up)
		err := runInTx(db, stmts, func(tx *sql.Tx) error {
			_, e := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.Version, m.Name)
			return e
		})
		if err != nil {
			return fmt.Errorf("migration %03d_%s up: %w", m.Version, m.Name, err)
		}
		slog.Info("migration applied", "version", m.Version, "name", m.Name)
		pending++
	}
	if pending == 0 {
		slog.Info("database schema up to date — no pending migrations")
	}
	return nil
}

// Down rolls back the last `steps` applied migrations in descending order.
func Down(db *sql.DB, steps int) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	byVersion := map[int]migration{}
	for _, m := range migs {
		byVersion[m.Version] = m
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	versions := make([]int, 0, len(applied))
	for v := range applied {
		versions = append(versions, v)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	if steps <= 0 || steps > len(versions) {
		steps = len(versions)
	}

	for i := 0; i < steps; i++ {
		v := versions[i]
		m, ok := byVersion[v]
		if !ok || m.Down == "" {
			return fmt.Errorf("cannot roll back version %d: no .down.sql found", v)
		}
		stmts := splitStatements(m.Down)
		err := runInTx(db, stmts, func(tx *sql.Tx) error {
			_, e := tx.Exec("DELETE FROM schema_migrations WHERE version = $1", v)
			return e
		})
		if err != nil {
			return fmt.Errorf("migration %03d_%s down: %w", m.Version, m.Name, err)
		}
		slog.Info("migration rolled back", "version", m.Version, "name", m.Name)
	}
	return nil
}

// Status prints which migrations are applied and which are pending.
func Status(db *sql.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	fmt.Println("VERSION  STATUS    NAME")
	for _, m := range migs {
		status := "pending"
		if applied[m.Version] {
			status = "applied"
		}
		fmt.Printf("%03d      %-8s  %s\n", m.Version, status, m.Name)
	}
	return nil
}

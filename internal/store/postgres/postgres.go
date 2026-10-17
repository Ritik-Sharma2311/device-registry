// Package postgres is a PostgreSQL-backed device.Store.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ritik-Sharma2311/device-registry/internal/device"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const uniqueViolation = "23505"

type Store struct{ pool *pgxpool.Pool }

// Open connects to dsn and applies any pending migrations.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// migrate runs each embedded .sql file once, in filename order, recording it
// in schema_migrations. An advisory lock stops two starting instances from
// migrating at the same time.
func (s *Store) migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(727001)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(727001)`)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sqlText, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() { s.pool.Close() }

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

const cols = `id, name, kind, address, status, version, created_at, updated_at`

func scan(row pgx.Row) (device.Device, error) {
	var d device.Device
	err := row.Scan(&d.ID, &d.Name, &d.Kind, &d.Address, &d.Status, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	d.CreatedAt, d.UpdatedAt = d.CreatedAt.UTC(), d.UpdatedAt.UTC()
	return d, err
}

func (s *Store) Create(ctx context.Context, d device.Device) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO devices (`+cols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		d.ID, d.Name, d.Kind, d.Address, d.Status, d.Version, d.CreatedAt, d.UpdatedAt)
	if isUnique(err) {
		return device.ErrConflict
	}
	return err
}

func (s *Store) Get(ctx context.Context, id string) (device.Device, error) {
	d, err := scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM devices WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return device.Device{}, device.ErrNotFound
	}
	return d, err
}

func (s *Store) List(ctx context.Context, f device.Filter) ([]device.Device, error) {
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Kind != "" {
		add("kind = $%d", f.Kind)
	}
	if f.Status != "" {
		add("status = $%d", f.Status)
	}
	if f.AfterID != "" {
		add("id > $%d", f.AfterID)
	}
	q := `SELECT ` + cols + ` FROM devices`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY id`
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []device.Device
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Update is a compare-and-swap on version done in a single statement, so it is
// atomic without an explicit transaction.
func (s *Store) Update(ctx context.Context, in device.Device) (device.Device, error) {
	d, err := scan(s.pool.QueryRow(ctx,
		`UPDATE devices
		    SET name=$2, kind=$3, address=$4, status=$5, updated_at=$6, version=version+1
		  WHERE id=$1 AND version=$7
		RETURNING `+cols,
		in.ID, in.Name, in.Kind, in.Address, in.Status, in.UpdatedAt, in.Version))
	switch {
	case err == nil:
		return d, nil
	case isUnique(err):
		return device.Device{}, device.ErrConflict
	case errors.Is(err, pgx.ErrNoRows):
		// No row matched: either the id is gone or the version moved on.
		var exists bool
		if qerr := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices WHERE id=$1)`, in.ID).Scan(&exists); qerr != nil {
			return device.Device{}, qerr
		}
		if exists {
			return device.Device{}, device.ErrVersionConflict
		}
		return device.Device{}, device.ErrNotFound
	default:
		return device.Device{}, err
	}
}

func (s *Store) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return device.ErrNotFound
	}
	return nil
}

package maintenancelock

import (
	"context"
	"database/sql"
)

// SQL is implemented by sql.DB and guarded maintenance handles.
type SQL interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// DB checks ownership before SQL and cancels in-flight SQL on detected lock loss.
// It never closes the shared database.
type DB struct {
	Raw   *sql.DB
	Lease *Lease
}

func (d DB) Context() context.Context {
	if d.Lease == nil {
		return context.Background()
	}
	return d.Lease.Context()
}
func (d DB) Check() error {
	if d.Lease == nil {
		return nil
	}
	return d.Lease.Check()
}
func (d DB) Exec(q string, args ...any) (sql.Result, error) {
	if err := d.Check(); err != nil {
		return nil, err
	}
	return d.Raw.ExecContext(d.Context(), q, args...)
}
func (d DB) Query(q string, args ...any) (*sql.Rows, error) {
	if err := d.Check(); err != nil {
		return nil, err
	}
	return d.Raw.QueryContext(d.Context(), q, args...)
}
func (d DB) QueryRow(q string, args ...any) *sql.Row {
	_ = d.Check() // Check cancels the context on failure; Scan returns that error.
	return d.Raw.QueryRowContext(d.Context(), q, args...)
}

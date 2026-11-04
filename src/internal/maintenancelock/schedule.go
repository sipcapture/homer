package maintenancelock

import (
	"context"
	"errors"
	"time"
)

// Due checks a persistent, database-scoped schedule while holding the lease.
// PostgreSQL time avoids clock skew between replicas. Until Complete is called,
// the schedule stays unchanged so another owner can retry an interrupted cycle.
// Only Homer's namespaced key in DuckLake's existing metadata table is used;
// missing metadata is an error, never a reason to create a table.
func (l *Lease) Due(task string, interval time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ctx.Err(); err != nil {
		return false, err
	}
	if len(l.conns) != 1 {
		return false, errors.New("maintenance schedule requires one catalog")
	}
	ctx, cancel := context.WithTimeout(l.ctx, 5*time.Second)
	defer cancel()
	var due bool
	err := l.conns[0].QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM public.ducklake_metadata
		WHERE CASE WHEN key = $1 AND scope IS NULL AND scope_id IS NULL
		THEN value::timestamptz + $2::double precision * interval '1 second' > clock_timestamp()
		ELSE false END
	)`, scheduleKey(task), interval.Seconds()).Scan(&due)
	if err != nil {
		return false, errors.New("read PostgreSQL maintenance schedule failed")
	}
	return due, nil
}

// Complete records a finished cycle before releasing the same session lock.
func (l *Lease) Complete(task string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ctx.Err(); err != nil {
		return err
	}
	if len(l.conns) != 1 {
		return errors.New("maintenance schedule requires one catalog")
	}
	ctx, cancel := context.WithTimeout(l.ctx, 5*time.Second)
	defer cancel()
	// DuckLake metadata has no unique key constraint. Replace only our global
	// marker atomically; the session advisory lock serializes Homer writers.
	_, err := l.conns[0].Exec(ctx, `WITH removed AS (
		DELETE FROM public.ducklake_metadata
		WHERE key = $1 AND scope IS NULL AND scope_id IS NULL
	)
	INSERT INTO public.ducklake_metadata (key, value, scope, scope_id)
	VALUES ($1, clock_timestamp()::text, NULL, NULL)`, scheduleKey(task))
	if err != nil {
		return errors.New("update PostgreSQL maintenance schedule failed")
	}
	return nil
}

func scheduleKey(task string) string { return "homer.maintenance." + task + ".completed_at" }

package maintenancelock

import (
	"context"
	"errors"
	"time"
)

// Due checks a persistent, database-scoped schedule while holding the lease.
// PostgreSQL time avoids clock skew between replicas. Until Complete is called,
// the schedule stays unchanged so another owner can retry an interrupted cycle.
// Schedule markers live in a Homer-owned table, separate from DuckLake metadata.
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
	if err := l.ensureSchedule(ctx); err != nil {
		return false, err
	}
	var due bool
	err := l.conns[0].QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM public.homer_maintenance_schedule
		WHERE task = $1 AND completed_at + $2::double precision * interval '1 second' > clock_timestamp()
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
	if err := l.ensureSchedule(ctx); err != nil {
		return err
	}
	_, err := l.conns[0].Exec(ctx, `INSERT INTO public.homer_maintenance_schedule (task, completed_at)
		VALUES ($1, clock_timestamp())
		ON CONFLICT (task) DO UPDATE SET completed_at = EXCLUDED.completed_at`, scheduleKey(task))
	if err != nil {
		return errors.New("update PostgreSQL maintenance schedule failed")
	}
	return nil
}

func scheduleKey(task string) string { return "homer.maintenance." + task + ".completed_at" }

// ensureSchedule runs under the catalog's session advisory lock.
func (l *Lease) ensureSchedule(ctx context.Context) error {
	_, err := l.conns[0].Exec(ctx, `CREATE TABLE IF NOT EXISTS public.homer_maintenance_schedule (
		task TEXT PRIMARY KEY, completed_at TIMESTAMPTZ NOT NULL)`)
	if err != nil {
		return errors.New("initialize PostgreSQL maintenance schedule failed")
	}
	return nil
}

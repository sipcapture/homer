// Package maintenancelock coordinates maintenance across processes.
package maintenancelock

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const Key int64 = 724193509

var ErrLost = errors.New("PostgreSQL maintenance lock connection lost")

// Lease owns dedicated physical sessions. Context is canceled on lock loss.
// Operations must use Context and Check before starting subsequent steps.
type Lease struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	conns  []*pgx.Conn
	done   chan struct{}
	once   sync.Once
}

func (l *Lease) Context() context.Context { return l.ctx }

// Acquire locks catalogs in stable order without waiting. Partial acquisition
// is rolled back on contention/error. Use direct connections or session pooling,
// never transaction pooling. Database identity ignores local DuckDB aliases.
func Acquire(parent context.Context, dsns ...string) (*Lease, bool, error) {
	configs := make(map[string]*pgx.ConnConfig)
	for _, dsn := range dsns {
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, false, errors.New("invalid maintenance catalog connection")
		}
		configs[fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, cfg.Database)] = cfg
	}
	keys := make([]string, 0, len(configs))
	for key := range configs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ctx, cancel := context.WithCancel(parent)
	l := &Lease{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	failed := func() { close(l.done); l.Release() }
	for _, key := range keys {
		acquireCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		conn, err := pgx.ConnectConfig(acquireCtx, configs[key])
		if err != nil {
			stop()
			failed()
			return nil, false, errors.New("connect to PostgreSQL maintenance lock failed")
		}
		l.conns = append(l.conns, conn)
		var acquired bool
		err = conn.QueryRow(acquireCtx, "SELECT pg_try_advisory_lock($1)", Key).Scan(&acquired)
		stop()
		if err != nil {
			failed()
			return nil, false, errors.New("acquire PostgreSQL maintenance lock failed")
		}
		if !acquired {
			failed()
			return nil, false, nil
		}
	}
	go func() {
		defer close(l.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if l.Check() != nil {
					return
				}
			}
		}
	}()
	return l, true, nil
}

// Check verifies all owning sessions before another maintenance step.
func (l *Lease) Check() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(l.ctx, 3*time.Second)
	defer cancel()
	for _, conn := range l.conns {
		if err := conn.Ping(ctx); err != nil {
			l.cancel()
			return ErrLost
		}
	}
	return nil
}
func (l *Lease) Release() {
	l.once.Do(func() {
		l.cancel()
		<-l.done
		l.mu.Lock()
		defer l.mu.Unlock()
		for i := len(l.conns) - 1; i >= 0; i-- {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = l.conns[i].Close(ctx)
			cancel()
		}
	})
}

// Try retains the release-function API for callers without SQL work.
func Try(ctx context.Context, dsn string) (func(), bool, error) {
	l, acquired, err := Acquire(ctx, dsn)
	if err != nil || !acquired {
		return nil, acquired, err
	}
	return l.Release, true, nil
}

// Package maintenancelock coordinates maintenance across processes.
package maintenancelock

import (
	"context"
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"
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
	return acquireWithLookup(parent, net.DefaultResolver.LookupHost, dsns...)
}

type hostLookup func(context.Context, string) ([]string, error)

func acquireWithLookup(parent context.Context, lookup hostLookup, dsns ...string) (*Lease, bool, error) {
	configs := make([]*pgx.ConnConfig, 0, len(dsns))
	for _, dsn := range dsns {
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, false, errors.New("invalid maintenance catalog connection")
		}
		configs = append(configs, cfg)
	}
	configs = deduplicateCatalogConfigs(parent, configs, lookup)
	ctx, cancel := context.WithCancel(parent)
	l := &Lease{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	failed := func() { close(l.done); l.Release() }
	for _, cfg := range configs {
		acquireCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		conn, err := pgx.ConnectConfig(acquireCtx, cfg)
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

// deduplicateCatalogConfigs groups catalogs whose host/address sets overlap.
// Merging the entire sets also handles transitive aliases (A-B and B-C).
// Resolution is only a deduplication hint: failure must not prevent pgx from
// connecting with its own resolver/fallback behavior.
func deduplicateCatalogConfigs(ctx context.Context, configs []*pgx.ConnConfig, lookup hostLookup) []*pgx.ConnConfig {
	sort.Slice(configs, func(i, j int) bool { return configs[i].ConnString() < configs[j].ConnString() })
	parent := make([]int, len(configs))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	sets := make([]map[string]bool, len(configs))
	owners := make(map[string]int)
	for i, cfg := range configs {
		sets[i] = catalogAddressKeys(ctx, cfg, lookup)
		for key := range sets[i] {
			if previous, ok := owners[key]; ok {
				parent[root(i)] = root(previous)
			} else {
				owners[key] = i
			}
		}
	}
	type group struct {
		cfg  *pgx.ConnConfig
		keys map[string]bool
	}
	merged := make(map[int]*group)
	// configs are sorted, so the first member is a stable representative.
	for i, cfg := range configs {
		r := root(i)
		if merged[r] == nil {
			merged[r] = &group{cfg: cfg, keys: make(map[string]bool)}
		}
		for key := range sets[i] {
			merged[r].keys[key] = true
		}
	}
	groups := make([]*group, 0, len(merged))
	for _, g := range merged {
		groups = append(groups, g)
	}
	minKey := func(keys map[string]bool) string {
		var smallest string
		for key := range keys {
			if smallest == "" || key < smallest {
				smallest = key
			}
		}
		return smallest
	}
	sort.Slice(groups, func(i, j int) bool { return minKey(groups[i].keys) < minKey(groups[j].keys) })
	result := make([]*pgx.ConnConfig, 0, len(groups))
	for _, g := range groups {
		result = append(result, g.cfg)
	}
	return result
}

func catalogAddressKeys(ctx context.Context, cfg *pgx.ConnConfig, lookup hostLookup) map[string]bool {
	host := cfg.Host
	if !strings.HasPrefix(host, "/") {
		host = strings.TrimSuffix(strings.ToLower(host), ".")
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	keys := make(map[string]bool)
	add := func(address string) {
		if ip := net.ParseIP(address); ip != nil {
			if ip.IsLoopback() {
				address = "127.0.0.1"
			} else {
				address = ip.String()
			}
		}
		keys[net.JoinHostPort(address, strconv.Itoa(int(port)))+"/"+cfg.Database] = true
	}
	add(host)
	if !strings.HasPrefix(host, "/") && net.ParseIP(host) == nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		addresses, _ := lookup(ctx, host)
		for _, address := range addresses {
			add(address)
		}
	}
	return keys
}

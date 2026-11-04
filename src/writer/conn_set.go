// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"net"
	"sync"
)

// connSet tracks the open connections of a stream receiver (TCP, TLS) so that
// Stop can close them. Without that, a handler blocked reading an idle
// connection holds Stop until its read deadline (read_timeout_sec, default
// 300 s).
type connSet struct {
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
}

// add registers conn. Once closeAll has run it closes conn instead and
// reports false.
func (cs *connSet) add(conn net.Conn) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.closed {
		_ = conn.Close()
		return false
	}
	if cs.conns == nil {
		cs.conns = make(map[net.Conn]struct{})
	}
	cs.conns[conn] = struct{}{}
	return true
}

func (cs *connSet) remove(conn net.Conn) {
	cs.mu.Lock()
	delete(cs.conns, conn)
	cs.mu.Unlock()
}

// closeAll closes every registered connection, which makes a pending read on
// it return at once, and makes later add calls fail.
func (cs *connSet) closeAll() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.closed = true
	for conn := range cs.conns {
		_ = conn.Close()
	}
}

// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import (
	"sync"

	"github.com/sipcapture/homer-core/src/sipparser"
)

var hepPool = sync.Pool{
	New: func() any { return &HEP{} },
}

func acquireHEP(d *Decoder) *HEP {
	h := hepPool.Get().(*HEP)
	*h = HEP{decoder: d}
	return h
}

// ReleaseHEP returns a decoded HEP and its SipMsg to their pools after the
// consumer (ingest worker, writer) is done with the packet. Safe to call
// with nil. Do not use hep after ReleaseHEP.
func ReleaseHEP(h *HEP) {
	if h == nil {
		return
	}
	if h.SIP != nil {
		sipparser.ReleaseSipMsg(h.SIP)
		h.SIP = nil
	}
	h.decoder = nil
	hepPool.Put(h)
}

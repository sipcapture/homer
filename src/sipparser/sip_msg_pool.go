// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sipparser

import "sync"

var sipMsgPool = sync.Pool{
	New: func() any { return &SipMsg{} },
}

func acquireSipMsg(opts *ZeroCopyOpts) *SipMsg {
	s := sipMsgPool.Get().(*SipMsg)
	*s = SipMsg{zcHdrOpts: opts}
	return s
}

// ReleaseSipMsg returns a parsed message to the pool.
func ReleaseSipMsg(s *SipMsg) {
	if s == nil {
		return
	}
	if s.CustomHeader != nil {
		for k := range s.CustomHeader {
			delete(s.CustomHeader, k)
		}
	}
	s.zcHdrOpts = nil
	sipMsgPool.Put(s)
}

// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package hepstream

// Config controls an in-process broker. Wire it from the publisher-side
// config (ingest.hep_stream in YAML) — the coordinator gets its own,
// smaller config type for the WS fan-out side in src/config.
//
// Keep defaults conservative: this feature is opt-in, and the single
// biggest foot-gun is a huge BufferSize starving ingest of memory on a
// very high PPS box.
type Config struct {
	// Enable turns the broker on. When false, NewBroker returns a nil
	// *Broker and all Publish calls become free no-ops (see the nil
	// check in Broker.Publish).
	Enable bool

	// BufferSize is the number of recent events retained for
	// late-joining subscribers (initial burst). 0 disables history
	// replay entirely; late subscribers then only see live events.
	BufferSize int

	// MaxSubscribers caps the number of concurrent Subscribe calls.
	// A new Subscribe beyond this cap returns (nil, nil, err) without
	// registering anything.
	MaxSubscribers int

	// PerSubQueueLen is the buffered channel length for each
	// subscriber. Smaller = faster back-pressure detection; larger =
	// more tolerance for brief consumer stalls.
	PerSubQueueLen int

	// RatePerSubPPS throttles the broker per subscriber. Events
	// exceeding the rate are dropped on that subscriber only (the
	// drop is accounted in metrics). 0 disables throttling.
	RatePerSubPPS int
}

// DefaultConfig returns the recommended publisher-side defaults.
// Values chosen so a single-box install can turn the feature on without
// tuning and still see sane behaviour.
func DefaultConfig() Config {
	return Config{
		Enable:         false,
		BufferSize:     10_000,
		MaxSubscribers: 32,
		PerSubQueueLen: 256,
		RatePerSubPPS:  500,
	}
}

// applyFallbacks fills zero-valued fields in c with the defaults. Called
// by NewBroker so callers can pass a partially initialised Config (e.g.
// straight from YAML, where the operator only set Enable=true).
func (c Config) applyFallbacks() Config {
	d := DefaultConfig()
	if c.BufferSize == 0 {
		c.BufferSize = d.BufferSize
	}
	if c.MaxSubscribers == 0 {
		c.MaxSubscribers = d.MaxSubscribers
	}
	if c.PerSubQueueLen == 0 {
		c.PerSubQueueLen = d.PerSubQueueLen
	}
	// Rate of 0 intentionally means "unlimited", so we do not promote
	// it to the default here. The operator explicitly opted out of
	// throttling.
	return c
}

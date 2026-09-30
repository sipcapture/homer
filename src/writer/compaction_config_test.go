// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func TestCompactionConfigFromStorageMapsNativeKnobs(t *testing.T) {
	got := compactionConfigFromStorage(config.CompactionConfig{
		Engine:              EngineNativeGo,
		TargetFileSizeBytes: 512 << 20,
		MaxRowGroupBytes:    64 << 20,
	}, true)

	if got.MaxRowGroupBytes != 64<<20 {
		t.Errorf("MaxRowGroupBytes = %d, want %d", got.MaxRowGroupBytes, 64<<20)
	}
	if got.TargetFileSizeBytes != 512<<20 || got.Engine != EngineNativeGo || !got.Enable {
		t.Errorf("unexpected mapping: %+v", got)
	}
}

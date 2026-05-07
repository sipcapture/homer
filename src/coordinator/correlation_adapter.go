// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	"context"

	"github.com/sipcapture/homer-core/src/coordinator/handlers"
	"github.com/sipcapture/homer-core/src/scripting/correlation"
)

// correlatorAdapter bridges the concrete *correlation.CorrelationEngine to
// the handlers.Correlator interface. Keeping it here (rather than inside
// handlers/) avoids handlers importing the scripting package, which would
// pull golua into every build of the handler unit tests.
type correlatorAdapter struct {
	engine *correlation.CorrelationEngine
}

// Has implements handlers.Correlator.
func (a correlatorAdapter) Has(hepid int, profile string) bool {
	if a.engine == nil {
		return false
	}
	return a.engine.Has(hepid, profile)
}

// Correlate implements handlers.Correlator by translating the handler-level
// struct into the engine's input shape and back.
func (a correlatorAdapter) Correlate(ctx context.Context, in handlers.CorrelationInput) *handlers.CorrelationResult {
	if a.engine == nil {
		return nil
	}
	res := a.engine.Correlate(ctx, correlation.CorrelationInput{
		HepID:      in.HepID,
		Profile:    in.Profile,
		ProtoType:  in.ProtoType,
		EventType:  in.EventType,
		BaseRows:   in.BaseRows,
		SessionIDs: in.SessionIDs,
		Nodes:      in.Nodes,
		TimeFrom:   in.TimeFrom,
		TimeTo:     in.TimeTo,
	})
	if res == nil {
		return nil
	}
	return &handlers.CorrelationResult{
		ExtraSessionIDs: res.ExtraSessionIDs,
		Debug:           res.Debug,
	}
}

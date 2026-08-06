// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package metrics

import "testing"

func TestResolveAgentLabel(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		nodeID   string
		nodeName string
		want     string
	}{
		{name: "default uses node_id", mode: "node_id", nodeID: "2002", nodeName: "voice", want: "2002"},
		{name: "empty mode uses node_id", mode: "", nodeID: "2002", nodeName: "voice", want: "2002"},
		{name: "node_name mode", mode: "node_name", nodeID: "2002", nodeName: "voice", want: "voice"},
		{name: "node_name case insensitive", mode: "Node_Name", nodeID: "2002", nodeName: "voice", want: "voice"},
		{name: "node_name falls back to id", mode: "node_name", nodeID: "2002", nodeName: "", want: "2002"},
		{name: "node_name whitespace falls back", mode: "node_name", nodeID: "2002", nodeName: "  ", want: "2002"},
		{name: "empty id uses name", mode: "node_id", nodeID: "", nodeName: "voice", want: "voice"},
		{name: "both empty", mode: "node_name", nodeID: "", nodeName: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAgentLabel(tt.mode, tt.nodeID, tt.nodeName)
			if got != tt.want {
				t.Fatalf("ResolveAgentLabel(%q,%q,%q)=%q, want %q", tt.mode, tt.nodeID, tt.nodeName, got, tt.want)
			}
		})
	}
}

func TestNormalizeAgentLabel(t *testing.T) {
	if got := normalizeAgentLabel(""); got != AgentLabelNodeID {
		t.Fatalf("empty -> %q, want %q", got, AgentLabelNodeID)
	}
	if got := normalizeAgentLabel("bogus"); got != AgentLabelNodeID {
		t.Fatalf("bogus -> %q, want %q", got, AgentLabelNodeID)
	}
	if got := normalizeAgentLabel("NODE_NAME"); got != AgentLabelNodeName {
		t.Fatalf("NODE_NAME -> %q, want %q", got, AgentLabelNodeName)
	}
}

func TestNewSIPMetricsProcessorAgentLabel(t *testing.T) {
	p := NewSIPMetricsProcessor("", "", "node_name")
	if got := p.agentLabelFor("2002", "voice"); got != "voice" {
		t.Fatalf("agentLabelFor=%q, want voice", got)
	}

	p = NewSIPMetricsProcessor("", "", "")
	if got := p.agentLabelFor("2002", "voice"); got != "2002" {
		t.Fatalf("default agentLabelFor=%q, want 2002", got)
	}
}

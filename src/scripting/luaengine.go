// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package scripting

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sipcapture/golua/lua"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/remotelog"
	"github.com/sipcapture/homer-core/src/scripting/luar"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// LuaEngine implements the ScriptEngine interface for Lua
type LuaEngine struct {
	hepPkt    **decoder.HEP
	functions []string
	LuaState  *lua.State
	config    *ScriptConfig
	lokiAllow map[string]struct{}
}

// NewLuaEngine creates a new Lua script engine
func NewLuaEngine(cfg *ScriptConfig) (*LuaEngine, error) {
	logger.Debug("Scripting: Initializing Lua engine")

	d := &LuaEngine{
		config:    cfg,
		lokiAllow: remotelog.LokiCustomLabelAllowlist(cfg.LokiCustomLabels),
	}
	d.LuaState = lua.NewState()
	OpenSandbox(d.LuaState)

	// Register functions available in Lua scripts
	luar.Register(d.LuaState, "", luar.Map{
		"GetHEPStruct":       d.GetHEPStruct,
		"GetSIPStruct":       d.GetSIPStruct,
		"GetHEPProtoType":    d.GetHEPProtoType,
		"GetHEPSrcIP":        d.GetHEPSrcIP,
		"GetHEPSrcPort":      d.GetHEPSrcPort,
		"GetHEPDstIP":        d.GetHEPDstIP,
		"GetHEPDstPort":      d.GetHEPDstPort,
		"GetHEPTimeSeconds":  d.GetHEPTimeSeconds,
		"GetHEPTimeUseconds": d.GetHEPTimeUseconds,
		"GetHEPNodeID":       d.GetHEPNodeID,
		"GetRawMessage":      d.GetRawMessage,
		"SetRawMessage":      d.SetRawMessage,
		"SetCustomSIPHeader": d.SetCustomSIPHeader,
		"SetHEPField":        d.SetHEPField,
		"SetSIPProfile":      d.SetSIPProfile,
		"SetSIPHeader":       d.SetSIPHeader,
		"SetLokiLabel":       d.SetLokiLabel,
		"HashTable":          HashTable,
		"HashString":         HashString,
		"Logp":               d.Logp,
		"Print":              fmt.Println,
	})

	// Load and compile scripts
	code, err := ScanLuaCode(cfg.Folder)
	if err != nil {
		return nil, fmt.Errorf("failed to scan script folder: %w", err)
	}

	if code.Len() > 0 {
		err = d.LuaState.DoString(code.String())
		if err != nil {
			return nil, fmt.Errorf("failed to load Lua scripts: %w", err)
		}

		d.functions = ExtractFunctions(code)
		if len(d.functions) < 1 {
			logger.Warn("Scripting: No functions found in Lua scripts")
		} else {
			logger.Info("Scripting: Loaded Lua functions", "count", len(d.functions))
		}
	}

	return d, nil
}

// Run executes the Lua scripts on the HEP packet
func (d *LuaEngine) Run(hep *decoder.HEP) error {
	// Check HEP filter
	if !ShouldProcessHEP(hep.ProtoType, d.config.HEPFilter) {
		return nil
	}

	d.hepPkt = &hep

	for _, v := range d.functions {
		err := d.LuaState.DoString(v)
		if err != nil {
			return fmt.Errorf("Lua script error: %w", err)
		}
	}
	return nil
}

// Close closes the Lua engine
func (d *LuaEngine) Close() {
	if d.LuaState != nil {
		d.LuaState.Close()
	}
}

// Reload reloads the Lua scripts
func (d *LuaEngine) Reload() error {
	code, err := ScanLuaCode(d.config.Folder)
	if err != nil {
		return fmt.Errorf("failed to scan script folder: %w", err)
	}

	if code.Len() > 0 {
		err = d.LuaState.DoString(code.String())
		if err != nil {
			return fmt.Errorf("failed to reload Lua scripts: %w", err)
		}

		d.functions = ExtractFunctions(code)
		logger.Info("Scripting: Reloaded Lua functions", "count", len(d.functions))
	}

	return nil
}

// ---- Getters for HEP fields ----

func (d *LuaEngine) GetHEPStruct() interface{} {
	if (*d.hepPkt) == nil {
		return ""
	}
	return (*d.hepPkt)
}

func (d *LuaEngine) GetSIPStruct() interface{} {
	if (*d.hepPkt).SIP == nil {
		return ""
	}
	return (*d.hepPkt).SIP
}

func (d *LuaEngine) GetHEPProtoType() uint32 {
	return (*d.hepPkt).ProtoType
}

func (d *LuaEngine) GetHEPSrcIP() string {
	return (*d.hepPkt).SrcIP
}

func (d *LuaEngine) GetHEPSrcPort() uint32 {
	return (*d.hepPkt).SrcPort
}

func (d *LuaEngine) GetHEPDstIP() string {
	return (*d.hepPkt).DstIP
}

func (d *LuaEngine) GetHEPDstPort() uint32 {
	return (*d.hepPkt).DstPort
}

func (d *LuaEngine) GetHEPTimeSeconds() uint32 {
	return (*d.hepPkt).Tsec
}

func (d *LuaEngine) GetHEPTimeUseconds() uint32 {
	return (*d.hepPkt).Tmsec
}

func (d *LuaEngine) GetHEPNodeID() uint32 {
	return (*d.hepPkt).NodeID
}

func (d *LuaEngine) GetRawMessage() string {
	return (*d.hepPkt).Payload
}

// ---- Setters for HEP fields ----

func (d *LuaEngine) SetRawMessage(value string) {
	if (*d.hepPkt) == nil {
		logger.Error("Scripting: Cannot set raw message - HEP struct is nil")
		return
	}
	(*d.hepPkt).Payload = value
}

func (d *LuaEngine) SetCustomSIPHeader(m *map[string]string) {
	if (*d.hepPkt).SIP == nil {
		logger.Error("Scripting: Cannot set custom SIP header - SIP struct is nil")
		return
	}
	hepPkt := *d.hepPkt

	if hepPkt.SIP.CustomHeader == nil {
		hepPkt.SIP.CustomHeader = make(map[string]string)
	}

	for k, v := range *m {
		hepPkt.SIP.CustomHeader[k] = v
	}
}

// parseUint32HEPField parses a decimal string into uint32 (CodeQL: avoid Atoi→uint32 truncation).
func parseUint32HEPField(value string) (uint32, bool) {
	u, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(u), true
}

func (d *LuaEngine) SetHEPField(field string, value string) {
	if (*d.hepPkt) == nil {
		logger.Error("Scripting: Cannot set HEP field - HEP struct is nil")
		return
	}
	hepPkt := *d.hepPkt

	switch field {
	case "ProtoType":
		if u, ok := parseUint32HEPField(value); ok {
			hepPkt.ProtoType = u
		}
	case "SrcIP":
		hepPkt.SrcIP = value
	case "SrcPort":
		if u, ok := parseUint32HEPField(value); ok {
			hepPkt.SrcPort = u
		}
	case "DstIP":
		hepPkt.DstIP = value
	case "DstPort":
		if u, ok := parseUint32HEPField(value); ok {
			hepPkt.DstPort = u
		}
	case "NodeID":
		if u, ok := parseUint32HEPField(value); ok {
			hepPkt.NodeID = u
		}
	case "CID":
		hepPkt.CID = value
	case "SID":
		hepPkt.SID = value
	case "NodeName":
		hepPkt.NodeName = value
	case "TargetName":
		hepPkt.TargetName = value
	}
}

func (d *LuaEngine) SetSIPProfile(p string) {
	hepPkt := *d.hepPkt
	if hepPkt.SIP == nil {
		return
	}
	if strings.HasPrefix(p, "c") || strings.HasPrefix(p, "C") {
		hepPkt.SIP.Profile = "call"
	} else if strings.HasPrefix(p, "r") || strings.HasPrefix(p, "R") {
		hepPkt.SIP.Profile = "registration"
	} else {
		hepPkt.SIP.Profile = "default"
	}
}

// SetLokiLabel sets an allowlisted custom Loki stream label (max 5 distinct keys per HEP message).
// Same idea as heplify-server PR #595; invalid keys/values are skipped with a debug log.
func (d *LuaEngine) SetLokiLabel(key, value string) {
	if d.hepPkt == nil || *d.hepPkt == nil {
		logger.Error("Scripting: SetLokiLabel — HEP struct is nil")
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if err := remotelog.ValidateLokiCustomLabelKey(key, d.lokiAllow); err != nil {
		logger.Debug("Scripting: SetLokiLabel skipped", "key", key, "reason", err.Error())
		return
	}
	if err := remotelog.ValidateLokiCustomLabelValue(value); err != nil {
		logger.Debug("Scripting: SetLokiLabel skipped", "key", key, "reason", err.Error())
		return
	}
	pkt := *d.hepPkt
	if pkt.CustomLokiLabels == nil {
		pkt.CustomLokiLabels = make(map[string]string)
	}
	if len(pkt.CustomLokiLabels) >= remotelog.MaxCustomLokiLabelsPerMessage {
		if _, exists := pkt.CustomLokiLabels[key]; !exists {
			logger.Debug("Scripting: SetLokiLabel skipped — max custom labels per message",
				"max", remotelog.MaxCustomLokiLabelsPerMessage)
			return
		}
	}
	pkt.CustomLokiLabels[key] = value
	*d.hepPkt = pkt
}

func (d *LuaEngine) SetSIPHeader(header string, value string) {
	if (*d.hepPkt).SIP == nil {
		logger.Error("Scripting: Cannot set SIP header - SIP struct is nil")
		return
	}
	hepPkt := *d.hepPkt
	sip := hepPkt.SIP

	switch header {
	case "FromUser", "from_user":
		sip.FromUser = value
	case "FromHost", "from_domain":
		sip.FromHost = value
	case "FromTag", "from_tag":
		sip.FromTag = value
	case "ToUser", "to_user":
		sip.ToUser = value
	case "ToHost", "to_domain":
		sip.ToHost = value
	case "ToTag", "to_tag":
		sip.ToTag = value
	case "URIUser", "ruri_user":
		sip.URIUser = value
	case "URIHost", "ruri_domain":
		sip.URIHost = value
	case "CallID":
		sip.CallID = value
	case "Method":
		sip.FirstMethod = value
	case "ContactUser", "contact_user":
		sip.ContactUser = value
	case "ContactHost", "contact_domain":
		sip.ContactHost = value
	case "AuthUser", "auth_user":
		sip.AuthUser = value
	case "UserAgent", "user_agent":
		sip.UserAgent = value
	case "Server":
		sip.Server = value
	case "PaiUser", "pid_user":
		sip.PaiUser = value
	case "PaiHost", "pid_domain":
		sip.PaiHost = value
	case "ViaOne", "via":
		sip.ViaOne = value
	case "XCallID", "callid_aleg":
		sip.XCallID = value
	default:
		if sip.CustomHeader == nil {
			sip.CustomHeader = make(map[string]string)
		}
		sip.CustomHeader[header] = value
	}
}

func (d *LuaEngine) Logp(level string, message string, data interface{}) {
	if level == "ERROR" {
		logger.Error(fmt.Sprintf("[script] %s: %v", message, data))
	} else {
		logger.Debug(fmt.Sprintf("[script] %s: %v", message, data))
	}
}

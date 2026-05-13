// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

var (
	hepLPMTblOnce sync.Once
	hepLPMTbl     map[string]ducklake.TableKey
)

func initHepLPMeasurementIndex() {
	hepLPMTblOnce.Do(func() {
		m := make(map[string]ducklake.TableKey)
		for k, sc := range ducklake.GetTableSchemas() {
			if sc == nil {
				continue
			}
			m["hep_proto_"+sc.TableSuffix] = k
		}
		hepLPMTbl = m
	})
}

// hepLPTableKeyForMeasurement maps a Line Protocol measurement (e.g.
// hep_proto_1_call) to a DuckLake TableKey when it is a known HEP table.
func hepLPTableKeyForMeasurement(measurement string) (ducklake.TableKey, bool) {
	initHepLPMeasurementIndex()
	k, ok := hepLPMTbl[measurement]
	return k, ok
}

// lpPointFieldMap merges tags and fields with SanitizeIdent keys (Influx flat namespace).
func lpPointFieldMap(p *LineProtoPoint) map[string]interface{} {
	m := make(map[string]interface{}, len(p.Tags)+len(p.Fields))
	if p == nil {
		return m
	}
	for k, v := range p.Tags {
		m[SanitizeIdent(k)] = v
	}
	for k, v := range p.Fields {
		m[SanitizeIdent(k)] = v
	}
	return m
}

func lineProtoPointToHEPRow(p *LineProtoPoint, key ducklake.TableKey) ([]interface{}, error) {
	switch {
	case key.ProtoType == ducklake.ProtoTypeSIP && key.SubType == ducklake.SIPTypeCall:
		return lineProtoPointToSIPCallRow(p)
	case key.ProtoType == ducklake.ProtoTypeSIP && key.SubType == ducklake.SIPTypeRegistration:
		return lineProtoPointToSIPRegistrationRow(p)
	case key.ProtoType == ducklake.ProtoTypeSIP && key.SubType == ducklake.SIPTypeDefault:
		return lineProtoPointToSIPDefaultLPRow(p)
	case key.ProtoType == ducklake.ProtoTypeRTCPJSON || key.ProtoType == ducklake.ProtoTypeRTCP || key.ProtoType == ducklake.ProtoTypeRTP:
		return lineProtoPointToRTCPFamilyRow(p, key)
	case key.ProtoType == ducklake.ProtoTypeDNS:
		return lineProtoPointToDNSLPRow(p, key)
	case key.ProtoType == ducklake.ProtoTypeLOG:
		return lineProtoPointToLOGLPRow(p, key)
	default:
		return nil, fmt.Errorf("unsupported HEP Line Protocol table key %v", key)
	}
}

func lineProtoPointToSIPRegistrationRow(p *LineProtoPoint) ([]interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil point")
	}
	m := lpPointFieldMap(p)
	ts, err := resolveSIPCallTimestamp(p, m)
	if err != nil {
		return nil, err
	}
	dateStr, err := resolveSIPCallDate(m, ts)
	if err != nil {
		return nil, err
	}
	uuidStr := stringField(m, "uuid")
	if uuidStr == "" {
		uuidStr = uuid.NewString()
	}
	extra, err := resolveDataExtra(m)
	if err != nil {
		return nil, err
	}
	key := ducklake.TableKey{ProtoType: ducklake.ProtoTypeSIP, SubType: ducklake.SIPTypeRegistration}
	cols := ducklake.InsertColumnNamesForKey(key)
	if len(cols) == 0 {
		return nil, fmt.Errorf("registration column list unavailable")
	}
	vals := map[string]interface{}{
		"uuid":          uuidStr,
		"date":          dateStr,
		"timestamp":     ts,
		"session_id":    stringField(m, "session_id"),
		"aor":           stringField(m, "aor"),
		"contact":       stringField(m, "contact"),
		"expires":       stringField(m, "expires"),
		"user_agent":    stringField(m, "user_agent"),
		"src_ip":        stringField(m, "src_ip"),
		"dst_ip":        stringField(m, "dst_ip"),
		"src_port":      uint32Field(m, "src_port"),
		"dst_port":      uint32Field(m, "dst_port"),
		"method":        stringField(m, "method"),
		"response_code": stringField(m, "response_code"),
		"protocol":      uint32Field(m, "protocol"),
		"node_id":       stringField(m, "node_id"),
		"payload":       stringField(m, "payload"),
		"data_extra":    extra,
	}
	row := make([]interface{}, len(cols))
	for i, col := range cols {
		v, ok := vals[col]
		if !ok {
			return nil, fmt.Errorf("internal: missing binding for column %q", col)
		}
		row[i] = v
	}
	return row, nil
}

func lineProtoPointToSIPDefaultLPRow(p *LineProtoPoint) ([]interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil point")
	}
	m := lpPointFieldMap(p)
	ts, err := resolveSIPCallTimestamp(p, m)
	if err != nil {
		return nil, err
	}
	dateStr, err := resolveSIPCallDate(m, ts)
	if err != nil {
		return nil, err
	}
	uuidStr := stringField(m, "uuid")
	if uuidStr == "" {
		uuidStr = uuid.NewString()
	}
	extra, err := resolveDataExtra(m)
	if err != nil {
		return nil, err
	}
	key := ducklake.TableKey{ProtoType: ducklake.ProtoTypeSIP, SubType: ducklake.SIPTypeDefault}
	cols := ducklake.InsertColumnNamesForKey(key)
	if len(cols) == 0 {
		return nil, fmt.Errorf("sip default column list unavailable")
	}
	vals := map[string]interface{}{
		"uuid":          uuidStr,
		"date":          dateStr,
		"timestamp":     ts,
		"session_id":    stringField(m, "session_id"),
		"src_ip":        stringField(m, "src_ip"),
		"dst_ip":        stringField(m, "dst_ip"),
		"src_port":      uint32Field(m, "src_port"),
		"dst_port":      uint32Field(m, "dst_port"),
		"method":        stringField(m, "method"),
		"response_code": stringField(m, "response_code"),
		"protocol":      uint32Field(m, "protocol"),
		"node_id":       stringField(m, "node_id"),
		"cid":           stringField(m, "cid"),
		"payload":       stringField(m, "payload"),
		"data_extra":    extra,
	}
	row := make([]interface{}, len(cols))
	for i, col := range cols {
		v, ok := vals[col]
		if !ok {
			return nil, fmt.Errorf("internal: missing binding for column %q", col)
		}
		row[i] = v
	}
	return row, nil
}

func lineProtoPointToRTCPFamilyRow(p *LineProtoPoint, key ducklake.TableKey) ([]interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil point")
	}
	m := lpPointFieldMap(p)
	ts, err := resolveSIPCallTimestamp(p, m)
	if err != nil {
		return nil, err
	}
	dateStr, err := resolveSIPCallDate(m, ts)
	if err != nil {
		return nil, err
	}
	uuidStr := stringField(m, "uuid")
	if uuidStr == "" {
		uuidStr = uuid.NewString()
	}
	extra, err := resolveDataExtra(m)
	if err != nil {
		return nil, err
	}
	sessionID := stringField(m, "session_id")
	if sessionID == "" {
		sessionID = stringField(m, "cid")
	}
	cols := ducklake.InsertColumnNamesForKey(key)
	if len(cols) == 0 {
		return nil, fmt.Errorf("rtcp/rtp column list unavailable")
	}
	vals := map[string]interface{}{
		"uuid":       uuidStr,
		"date":       dateStr,
		"timestamp":  ts,
		"session_id": sessionID,
		"src_ip":     stringField(m, "src_ip"),
		"dst_ip":     stringField(m, "dst_ip"),
		"src_port":   uint32Field(m, "src_port"),
		"dst_port":   uint32Field(m, "dst_port"),
		"protocol":   uint32Field(m, "protocol"),
		"node_id":    stringField(m, "node_id"),
		"cid":        stringField(m, "cid"),
		"payload":    stringField(m, "payload"),
		"data_extra": extra,
	}
	row := make([]interface{}, len(cols))
	for i, col := range cols {
		v, ok := vals[col]
		if !ok {
			return nil, fmt.Errorf("internal: missing binding for column %q", col)
		}
		row[i] = v
	}
	return row, nil
}

func lineProtoPointToDNSLPRow(p *LineProtoPoint, key ducklake.TableKey) ([]interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil point")
	}
	m := lpPointFieldMap(p)
	ts, err := resolveSIPCallTimestamp(p, m)
	if err != nil {
		return nil, err
	}
	dateStr, err := resolveSIPCallDate(m, ts)
	if err != nil {
		return nil, err
	}
	uuidStr := stringField(m, "uuid")
	if uuidStr == "" {
		uuidStr = uuid.NewString()
	}
	extra, err := resolveDataExtra(m)
	if err != nil {
		return nil, err
	}
	cols := ducklake.InsertColumnNamesForKey(key)
	if len(cols) == 0 {
		return nil, fmt.Errorf("dns column list unavailable")
	}
	vals := map[string]interface{}{
		"uuid":       uuidStr,
		"date":       dateStr,
		"timestamp":  ts,
		"src_ip":     stringField(m, "src_ip"),
		"dst_ip":     stringField(m, "dst_ip"),
		"src_port":   uint32Field(m, "src_port"),
		"dst_port":   uint32Field(m, "dst_port"),
		"protocol":   uint32Field(m, "protocol"),
		"node_id":    stringField(m, "node_id"),
		"payload":    stringField(m, "payload"),
		"data_extra": extra,
	}
	row := make([]interface{}, len(cols))
	for i, col := range cols {
		v, ok := vals[col]
		if !ok {
			return nil, fmt.Errorf("internal: missing binding for column %q", col)
		}
		row[i] = v
	}
	return row, nil
}

func lineProtoPointToLOGLPRow(p *LineProtoPoint, key ducklake.TableKey) ([]interface{}, error) {
	if p == nil {
		return nil, fmt.Errorf("nil point")
	}
	m := lpPointFieldMap(p)
	ts, err := resolveSIPCallTimestamp(p, m)
	if err != nil {
		return nil, err
	}
	dateStr, err := resolveSIPCallDate(m, ts)
	if err != nil {
		return nil, err
	}
	uuidStr := stringField(m, "uuid")
	if uuidStr == "" {
		uuidStr = uuid.NewString()
	}
	extra, err := resolveDataExtra(m)
	if err != nil {
		return nil, err
	}
	sid := stringField(m, "session_id")
	if sid == "" {
		sid = stringField(m, "cid")
	}
	cols := ducklake.InsertColumnNamesForKey(key)
	if len(cols) == 0 {
		return nil, fmt.Errorf("log column list unavailable")
	}
	vals := map[string]interface{}{
		"uuid":       uuidStr,
		"date":       dateStr,
		"timestamp":  ts,
		"session_id": sid,
		"src_ip":     stringField(m, "src_ip"),
		"dst_ip":     stringField(m, "dst_ip"),
		"node_id":    stringField(m, "node_id"),
		"payload":    stringField(m, "payload"),
		"data_extra": extra,
	}
	row := make([]interface{}, len(cols))
	for i, col := range cols {
		v, ok := vals[col]
		if !ok {
			return nil, fmt.Errorf("internal: missing binding for column %q", col)
		}
		row[i] = v
	}
	return row, nil
}

// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
)

// AgentSubscription represents agent_location_session row
type AgentSubscription struct {
	UUID       string `json:"uuid"`
	GID        int    `json:"gid"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Path       string `json:"path"`
	Node       string `json:"node"`
	Type       string `json:"type"`
	TTL        int    `json:"ttl"`
	CreateDate string `json:"create_date"`
	ExpireDate string `json:"expire_date"`
	Active     int    `json:"active"`
}

type AgentSubscriptionFilters struct {
	Type  string
	Node  string
	Limit int
	Sort  string
}

// AgentSubscriptionService provides access to agent_location_session
type AgentSubscriptionService struct {
	db *sql.DB
}

func NewAgentSubscriptionService(db *sql.DB) *AgentSubscriptionService {
	return &AgentSubscriptionService{db: db}
}

func (s *AgentSubscriptionService) List(ctx context.Context, filters AgentSubscriptionFilters) ([]AgentSubscription, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := []string{"1=1"}
	if filters.Type != "" {
		value := "%" + sqlvalidator.SafeString(filters.Type) + "%"
		where = append(where, fmt.Sprintf("type ILIKE '%s'", value))
	}
	if filters.Node != "" {
		value := "%" + sqlvalidator.SafeString(filters.Node) + "%"
		where = append(where, fmt.Sprintf("node ILIKE '%s'", value))
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	sortExpr := filters.Sort
	if sortExpr == "" {
		sortExpr = "create_date DESC"
	}
	sql := fmt.Sprintf(
		`SELECT guid, gid, host, port, protocol, path, node, type, create_date, expire_date, active
		 FROM agent_location_session
		 WHERE %s
		 ORDER BY %s
		 LIMIT %d`,
		strings.Join(where, " AND "),
		sortExpr,
		limit,
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	items := make([]AgentSubscription, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToAgentSub(row))
	}
	return items, nil
}

func (s *AgentSubscriptionService) ListTypes(ctx context.Context, search string, limit int, sortExpr string) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := []string{"type IS NOT NULL"}
	if search != "" {
		where = append(where, fmt.Sprintf("lower(type) LIKE '%%%s%%'", escapeSQL(strings.ToLower(search))))
	}
	if limit <= 0 {
		limit = 100
	}
	if sortExpr == "" {
		sortExpr = "type ASC"
	}
	sql := fmt.Sprintf(
		`SELECT DISTINCT type
		 FROM agent_location_session
		 WHERE %s
		 ORDER BY %s
		 LIMIT %d`,
		strings.Join(where, " AND "),
		sortExpr,
		limit,
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	items := make([]string, 0, len(rows))
	for _, row := range rows {
		if v, ok := row["type"].(string); ok && v != "" {
			items = append(items, v)
		}
	}
	return items, nil
}

func (s *AgentSubscriptionService) GetByUUID(ctx context.Context, uuid string) (*AgentSubscription, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`SELECT guid, gid, host, port, protocol, path, node, type, create_date, expire_date, active
		 FROM agent_location_session
		 WHERE guid = '%s'`,
		escapeSQL(uuid),
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	item := mapRowToAgentSub(rows[0])
	return &item, nil
}

func (s *AgentSubscriptionService) Search(ctx context.Context, guid, subType string) ([]AgentSubscription, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := []string{"1=1"}
	if guid != "" {
		where = append(where, fmt.Sprintf("guid = '%s'", escapeSQL(guid)))
	}
	if subType != "" {
		where = append(where, fmt.Sprintf("type LIKE '%%%s%%'", escapeSQL(subType)))
	}
	sql := fmt.Sprintf(
		`SELECT guid, gid, host, port, protocol, path, node, type, create_date, expire_date, active
		 FROM agent_location_session
		 WHERE %s
		 ORDER BY create_date DESC`,
		strings.Join(where, " AND "),
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	items := make([]AgentSubscription, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToAgentSub(row))
	}
	return items, nil
}

func (s *AgentSubscriptionService) Create(ctx context.Context, item AgentSubscription) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if item.UUID == "" {
		item.UUID = newGUID()
	}
	if item.ExpireDate == "" {
		item.ExpireDate = time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	}
	if item.Active == 0 {
		item.Active = 1
	}
	sql := fmt.Sprintf(
		`INSERT INTO agent_location_session
		 (guid, gid, host, port, protocol, path, node, type, create_date, expire_date, active)
		 VALUES ('%s', %d, '%s', %d, '%s', '%s', '%s', '%s', current_timestamp, '%s', %d)`,
		escapeSQL(item.UUID),
		item.GID,
		escapeSQL(item.Host),
		item.Port,
		escapeSQL(item.Protocol),
		escapeSQL(item.Path),
		escapeSQL(item.Node),
		escapeSQL(item.Type),
		escapeSQL(item.ExpireDate),
		item.Active,
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return item.UUID, nil
}

func (s *AgentSubscriptionService) Update(ctx context.Context, uuid string, item AgentSubscription) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`UPDATE agent_location_session
		 SET gid = %d, host = '%s', port = %d, protocol = '%s', path = '%s',
		     node = '%s', type = '%s', expire_date = '%s', active = %d
		 WHERE guid = '%s'`,
		item.GID,
		escapeSQL(item.Host),
		item.Port,
		escapeSQL(item.Protocol),
		escapeSQL(item.Path),
		escapeSQL(item.Node),
		escapeSQL(item.Type),
		escapeSQL(item.ExpireDate),
		item.Active,
		escapeSQL(uuid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return uuid, nil
}

func (s *AgentSubscriptionService) Delete(ctx context.Context, uuid string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`DELETE FROM agent_location_session
		 WHERE guid = '%s'`,
		escapeSQL(uuid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return false, err
	}
	return true, nil
}

func mapRowToAgentSub(row map[string]interface{}) AgentSubscription {
	item := AgentSubscription{}
	if v, ok := row["guid"].(string); ok {
		item.UUID = v
	}
	if v, ok := row["gid"]; ok {
		item.GID = int(toInt64(v))
	}
	if v, ok := row["host"].(string); ok {
		item.Host = v
	}
	if v, ok := row["port"]; ok {
		item.Port = int(toInt64(v))
	}
	if v, ok := row["protocol"].(string); ok {
		item.Protocol = v
	}
	if v, ok := row["path"].(string); ok {
		item.Path = v
	}
	if v, ok := row["node"].(string); ok {
		item.Node = v
	}
	if v, ok := row["type"].(string); ok {
		item.Type = v
	}
	if v, ok := row["create_date"]; ok {
		item.CreateDate = toTimeString(v)
	}
	if v, ok := row["expire_date"]; ok {
		item.ExpireDate = toTimeString(v)
	}
	if v, ok := row["active"]; ok {
		item.Active = int(toInt64(v))
	}
	if item.ExpireDate != "" {
		item.TTL = ttlFromExpire(item.ExpireDate)
	}
	return item
}

func ttlFromExpire(expire string) int {
	expireTime, err := time.Parse(time.RFC3339, expire)
	if err != nil {
		return 0
	}
	ttl := int(time.Until(expireTime).Seconds())
	if ttl < 0 {
		return 0
	}
	return ttl
}

// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// FlightService manages connections to nodes via HTTP API
type FlightService struct {
	nodes        []config.NodeEndpoint
	queryClient  *http.Client // used for POST /query (timeout applied per request via context)
	healthClient *http.Client // used for GET /health (short fixed timeout)
	// queryTimeout bounds each individual POST /query. It is applied as a
	// context deadline (not http.Client.Timeout) so callers with their own,
	// shorter deadline are still honored.
	queryTimeout time.Duration
	connected    map[string]bool
	mu           sync.RWMutex
	stopCh       chan struct{}
	stopOnce     sync.Once
	lakeName     string
}

// NewFlightService creates a new FlightSQL service.
// queryTimeout controls the per-query timeout for data queries to nodes.
func NewFlightService(nodes []config.NodeEndpoint, queryTimeout time.Duration) *FlightService {
	if queryTimeout <= 0 {
		queryTimeout = 30 * time.Second
	}
	return &FlightService{
		nodes:        nodes,
		queryClient:  &http.Client{},
		queryTimeout: queryTimeout,
		healthClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		connected: make(map[string]bool),
		stopCh:    make(chan struct{}),
		lakeName:  "homer_lake",
	}
}

// SetLakeName configures the DuckLake catalog name used in SQL queries.
func (s *FlightService) SetLakeName(name string) {
	if name != "" {
		s.lakeName = name
	}
}

// LakeName returns the configured DuckLake catalog name.
func (s *FlightService) LakeName() string {
	if s.lakeName == "" {
		return "homer_lake"
	}
	return s.lakeName
}

// ConnectAll connects to all configured nodes.
// It retries a few times on startup to handle the case where the node module
// in the same process hasn't finished starting yet.
func (s *FlightService) ConnectAll() error {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.mu.Lock()
		allOk := true
		for _, node := range s.nodes {
			if err := s.checkNode(node); err != nil {
				s.connected[node.Name] = false
				allOk = false
				if attempt < maxRetries {
					logger.Info("Hub: Node not ready, retrying",
						"node", node.Name, "attempt", attempt, "max_retries", maxRetries, "retry_delay", retryDelay)
				} else {
					logger.Error(fmt.Sprintf("Hub: Failed to connect to node %s after %d attempts: %v",
						node.Name, maxRetries, err))
				}
			} else {
				s.connected[node.Name] = true
			}
		}
		s.mu.Unlock()

		if allOk {
			break
		}
		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}

	// Start background health check loop
	go s.healthCheckLoop()

	return nil
}

// healthCheckLoop periodically re-checks disconnected nodes and verifies
// connected ones are still alive.
func (s *FlightService) healthCheckLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			for _, node := range s.nodes {
				wasConnected := s.connected[node.Name]
				if err := s.checkNode(node); err != nil {
					if wasConnected {
						logger.Warn(fmt.Sprintf("Hub: Node %s went offline: %v", node.Name, err))
					}
					s.connected[node.Name] = false
				} else {
					if !wasConnected {
						logger.Info("Hub: Node is now online", "node", node.Name)
					}
					s.connected[node.Name] = true
				}
			}
			s.mu.Unlock()
		}
	}
}

// checkNode verifies a node is reachable via HTTP
func (s *FlightService) checkNode(node config.NodeEndpoint) error {
	// Node HTTP API runs on FlightServer.Port + 1
	httpPort := node.Port + 1
	url := fmt.Sprintf("http://%s:%d/health", node.Host, httpPort)

	resp, err := s.healthClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}

	logger.Info("🔗 Hub: Connected to node", "node", node.Name, "addr", fmt.Sprintf("%s:%d", node.Host, httpPort))
	return nil
}

// CloseAll closes all connections and stops the health check loop
func (s *FlightService) CloseAll() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = make(map[string]bool)
}

// QueryRequest for HTTP API
type queryRequest struct {
	SQL string `json:"sql"`
}

// QueryResponse from HTTP API
type queryResponse struct {
	Success bool                     `json:"success"`
	Data    []map[string]interface{} `json:"data"`
	Count   int                      `json:"count"`
	Error   string                   `json:"error"`
}

// QueryFirstConnected runs SQL on the first node that is currently connected.
// Intended for INSERT/DDL that must not be duplicated across a multi-node cluster.
func (s *FlightService) QueryFirstConnected(ctx context.Context, sql string) ([]map[string]interface{}, error) {
	s.mu.RLock()
	nodes := make([]config.NodeEndpoint, len(s.nodes))
	copy(nodes, s.nodes)
	connected := make(map[string]bool)
	for k, v := range s.connected {
		connected[k] = v
	}
	s.mu.RUnlock()

	for _, node := range nodes {
		if !connected[node.Name] {
			continue
		}
		return s.queryNode(ctx, node, sql)
	}
	return nil, fmt.Errorf("no connected storage nodes")
}

// Query executes a SQL query against all nodes and returns results
func (s *FlightService) Query(ctx context.Context, sql string) ([]map[string]interface{}, error) {
	s.mu.RLock()
	nodes := make([]config.NodeEndpoint, len(s.nodes))
	copy(nodes, s.nodes)
	connected := make(map[string]bool)
	for k, v := range s.connected {
		connected[k] = v
	}
	s.mu.RUnlock()

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no configured nodes")
	}

	connectedCount := 0
	for _, node := range nodes {
		if connected[node.Name] {
			connectedCount++
		}
	}
	logger.Info("Hub: Query to nodes", "connected", connectedCount, "total", len(nodes), "sql_chars", len(sql))
	logger.Debug("Hub: Query to nodes", "sql", sql)

	allResults := make([]map[string]interface{}, 0)
	var nodeErrs []error
	succeeded := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, node := range nodes {
		if !connected[node.Name] {
			logger.Info("Hub: Skipping disconnected node", "node", node.Name)
			continue
		}

		wg.Add(1)
		go func(n config.NodeEndpoint) {
			defer wg.Done()

			results, err := s.queryNode(ctx, n, sql)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logger.Error(fmt.Sprintf("Hub: Query failed on node %s: %v", n.Name, err))
				nodeErrs = append(nodeErrs, fmt.Errorf("node %s: %w", n.Name, err))
				return
			}
			succeeded++
			allResults = append(allResults, results...)
		}(node)
	}

	wg.Wait()

	// Surface the failure when no node produced a result; silently returning
	// an empty set hides real errors (query timeouts, SQL errors) from API
	// consumers. Partial results from a degraded cluster are still returned.
	if succeeded == 0 && len(nodeErrs) > 0 {
		return nil, errors.Join(nodeErrs...)
	}
	return allResults, nil
}

// QueryNode executes a SQL query against a specific node
func (s *FlightService) QueryNode(ctx context.Context, nodeName string, sql string) ([]map[string]interface{}, error) {
	s.mu.RLock()
	var node config.NodeEndpoint
	found := false
	for _, n := range s.nodes {
		if n.Name == nodeName {
			node = n
			found = true
			break
		}
	}
	s.mu.RUnlock()

	if !found {
		return nil, fmt.Errorf("node %s not found", nodeName)
	}

	return s.queryNode(ctx, node, sql)
}

// queryNode executes a query on a single node via HTTP
func (s *FlightService) queryNode(ctx context.Context, node config.NodeEndpoint, sql string) ([]map[string]interface{}, error) {
	// Per-query timeout; context.WithTimeout keeps the parent deadline when
	// the caller's is sooner.
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	// Node HTTP API runs on FlightServer.Port + 1
	httpPort := node.Port + 1
	url := fmt.Sprintf("http://%s:%d/query", node.Host, httpPort)

	reqBody, err := json.Marshal(queryRequest{SQL: sql})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := strings.TrimSpace(node.Token); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := s.queryClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("query HTTP error: status=%d body=%q", resp.StatusCode, string(body))
	}

	var result queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("query error: %s", result.Error)
	}

	return result.Data, nil
}

// GetStats returns connection statistics
func (s *FlightService) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["configured_nodes"] = len(s.nodes)

	connectedCount := 0
	nodeList := make([]map[string]interface{}, 0, len(s.nodes))
	for _, node := range s.nodes {
		isConnected := s.connected[node.Name]
		if isConnected {
			connectedCount++
		}
		nodeInfo := map[string]interface{}{
			"name":      node.Name,
			"host":      node.Host,
			"port":      node.Port,
			"connected": isConnected,
		}
		nodeList = append(nodeList, nodeInfo)
	}
	stats["connected_nodes"] = connectedCount
	stats["nodes"] = nodeList

	return stats
}

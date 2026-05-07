// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package remotelog provides remote logging clients for Loki, Elasticsearch, and InfluxDB Line Protocol.
package remotelog

import (
	"github.com/sipcapture/homer-core/src/decoder"
)

// RemoteLogger interface for sending HEP packets to remote logging systems
type RemoteLogger interface {
	// Send sends a HEP packet to the remote logging system
	Send(hep *decoder.HEP) error
	// Close closes the connection and flushes any buffered data
	Close() error
}

// LokiConfig configures the Loki client
type LokiConfig struct {
	Enable       bool   `json:"enable" mapstructure:"enable" default:"false"`
	URL          string `json:"url" mapstructure:"url" default:"http://localhost:3100/loki/api/v1/push"`
	Bulk         int    `json:"bulk" mapstructure:"bulk" default:"400"`                       // Batch size before sending
	Timer        int    `json:"timer" mapstructure:"timer" default:"4"`                       // Seconds before force flush
	Buffer       int    `json:"buffer" mapstructure:"buffer" default:"100000"`                // Channel buffer size
	HEPFilter    []int  `json:"hep_filter" mapstructure:"hep_filter"`                         // Which HEP types to log (empty = all)
	IPPortLabels bool `json:"ip_port_labels" mapstructure:"ip_port_labels" default:"false"` // Include IP/Port as labels
}

// ElasticsearchConfig configures the Elasticsearch client
type ElasticsearchConfig struct {
	Enable     bool   `json:"enable" mapstructure:"enable" default:"false"`
	Addr       string `json:"addr" mapstructure:"addr" default:"http://localhost:9200"`
	User       string `json:"user" mapstructure:"user"`
	Pass       string `json:"pass" mapstructure:"pass"`
	Discovery  bool   `json:"discovery" mapstructure:"discovery" default:"true"`
	IndexDaily bool   `json:"index_daily" mapstructure:"index_daily" default:"true"`
	IndexName  string `json:"index_name" mapstructure:"index_name" default:"hep"`
	HEPFilter  []int  `json:"hep_filter" mapstructure:"hep_filter"`
}

// LineProtoConfig configures the InfluxDB Line Protocol client
type LineProtoConfig struct {
	Enable    bool   `json:"enable" mapstructure:"enable" default:"false"`
	URL       string `json:"url" mapstructure:"url" default:"http://localhost:8086/write?db=hep"`
	Bulk      int    `json:"bulk" mapstructure:"bulk" default:"400"`
	Timer     int    `json:"timer" mapstructure:"timer" default:"4"`
	Buffer    int    `json:"buffer" mapstructure:"buffer" default:"100000"`
	HEPFilter []int  `json:"hep_filter" mapstructure:"hep_filter"`
}

// RemoteLogConfig combines all remote logging configurations
type RemoteLogConfig struct {
	Loki          LokiConfig          `json:"loki" mapstructure:"loki"`
	Elasticsearch ElasticsearchConfig `json:"elasticsearch" mapstructure:"elasticsearch"`
	LineProto     LineProtoConfig     `json:"lineproto" mapstructure:"lineproto"`
}

// ShouldProcess checks if the HEP packet should be processed based on filter
func ShouldProcess(protoType uint32, filter []int) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if uint32(f) == protoType {
			return true
		}
	}
	return false
}

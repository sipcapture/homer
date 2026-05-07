// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package flight

import (
	"context"
	"sync"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/hugr-lab/airport-go/catalog"
	"github.com/sipcapture/homer-core/src/decoder"
)

// HEPTable implements the catalog.Table interface for HEP data
type HEPTable struct {
	name       string
	comment    string
	schema     *arrow.Schema
	buffer     *RingBuffer
	bufferSize int
	mu         sync.RWMutex
}

// NewHEPTable creates a new HEP table with a ring buffer
func NewHEPTable(name, comment string, bufferSize int) *HEPTable {
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "version", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "protocol", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "src_ip", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "dst_ip", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "src_port", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "dst_port", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "tsec", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "tmsec", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "proto_type", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "node_id", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "node_pw", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "payload", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "cid", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "vlan", Type: arrow.PrimitiveTypes.Uint32, Nullable: true},
		{Name: "node_name", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "target_name", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sid", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "timestamp", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		// SIP fields
		{Name: "sip_call_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_method", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_from_user", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_from_host", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_to_user", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_to_host", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_cseq", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "sip_user_agent", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)

	return &HEPTable{
		name:       name,
		comment:    comment,
		schema:     schema,
		buffer:     NewRingBuffer(bufferSize),
		bufferSize: bufferSize,
	}
}

// Name returns the table name
func (t *HEPTable) Name() string {
	return t.name
}

// Comment returns the table description
func (t *HEPTable) Comment() string {
	return t.comment
}

// ArrowSchema returns the Arrow schema with optional column projection
func (t *HEPTable) ArrowSchema(columns []string) *arrow.Schema {
	if len(columns) == 0 {
		return t.schema
	}

	// Project columns
	fields := make([]arrow.Field, 0, len(columns))
	for _, col := range columns {
		for _, f := range t.schema.Fields() {
			if f.Name == col {
				fields = append(fields, f)
				break
			}
		}
	}
	return arrow.NewSchema(fields, nil)
}

// Scan returns a RecordReader for querying the table
func (t *HEPTable) Scan(ctx context.Context, opts *catalog.ScanOptions) (array.RecordReader, error) {
	t.mu.RLock()
	packets := t.buffer.GetAll()
	t.mu.RUnlock()

	if len(packets) == 0 {
		// Return empty reader
		return array.NewRecordReader(t.schema, nil)
	}

	// Build Arrow record from packets
	pool := memory.DefaultAllocator
	builder := array.NewRecordBuilder(pool, t.schema)
	defer builder.Release()

	// Get builders for each field
	versionBuilder := builder.Field(0).(*array.Uint32Builder)
	protocolBuilder := builder.Field(1).(*array.Uint32Builder)
	srcIPBuilder := builder.Field(2).(*array.StringBuilder)
	dstIPBuilder := builder.Field(3).(*array.StringBuilder)
	srcPortBuilder := builder.Field(4).(*array.Uint32Builder)
	dstPortBuilder := builder.Field(5).(*array.Uint32Builder)
	tsecBuilder := builder.Field(6).(*array.Uint32Builder)
	tmsecBuilder := builder.Field(7).(*array.Uint32Builder)
	protoTypeBuilder := builder.Field(8).(*array.Uint32Builder)
	nodeIDBuilder := builder.Field(9).(*array.Uint32Builder)
	nodePWBuilder := builder.Field(10).(*array.StringBuilder)
	payloadBuilder := builder.Field(11).(*array.StringBuilder)
	cidBuilder := builder.Field(12).(*array.StringBuilder)
	vlanBuilder := builder.Field(13).(*array.Uint32Builder)
	nodeNameBuilder := builder.Field(14).(*array.StringBuilder)
	targetNameBuilder := builder.Field(15).(*array.StringBuilder)
	sidBuilder := builder.Field(16).(*array.StringBuilder)
	timestampBuilder := builder.Field(17).(*array.Int64Builder)
	sipCallIDBuilder := builder.Field(18).(*array.StringBuilder)
	sipMethodBuilder := builder.Field(19).(*array.StringBuilder)
	sipFromUserBuilder := builder.Field(20).(*array.StringBuilder)
	sipFromHostBuilder := builder.Field(21).(*array.StringBuilder)
	sipToUserBuilder := builder.Field(22).(*array.StringBuilder)
	sipToHostBuilder := builder.Field(23).(*array.StringBuilder)
	sipCseqBuilder := builder.Field(24).(*array.StringBuilder)
	sipUserAgentBuilder := builder.Field(25).(*array.StringBuilder)

	// Fill data
	for _, hep := range packets {
		versionBuilder.Append(hep.Version)
		protocolBuilder.Append(hep.Protocol)
		srcIPBuilder.Append(hep.SrcIP)
		dstIPBuilder.Append(hep.DstIP)
		srcPortBuilder.Append(hep.SrcPort)
		dstPortBuilder.Append(hep.DstPort)
		tsecBuilder.Append(hep.Tsec)
		tmsecBuilder.Append(hep.Tmsec)
		protoTypeBuilder.Append(hep.ProtoType)
		nodeIDBuilder.Append(hep.NodeID)
		nodePWBuilder.Append(hep.NodePW)
		payloadBuilder.Append(hep.Payload)
		cidBuilder.Append(hep.CID)
		vlanBuilder.Append(hep.Vlan)
		nodeNameBuilder.Append(hep.NodeName)
		targetNameBuilder.Append(hep.TargetName)
		sidBuilder.Append(hep.SID)

		if hep.Timestamp.IsZero() {
			timestampBuilder.AppendNull()
		} else {
			timestampBuilder.Append(hep.Timestamp.UnixNano())
		}

		// SIP fields
		if hep.SIP != nil {
			sipCallIDBuilder.Append(hep.SIP.CallID)
			sipMethodBuilder.Append(hep.SIP.CseqMethod)
			sipFromUserBuilder.Append(hep.SIP.FromUser)
			sipFromHostBuilder.Append(hep.SIP.FromHost)
			sipToUserBuilder.Append(hep.SIP.ToUser)
			sipToHostBuilder.Append(hep.SIP.ToHost)
			sipCseqBuilder.Append(hep.SIP.CseqVal)
			sipUserAgentBuilder.Append(hep.SIP.UserAgent)
		} else {
			sipCallIDBuilder.AppendNull()
			sipMethodBuilder.AppendNull()
			sipFromUserBuilder.AppendNull()
			sipFromHostBuilder.AppendNull()
			sipToUserBuilder.AppendNull()
			sipToHostBuilder.AppendNull()
			sipCseqBuilder.AppendNull()
			sipUserAgentBuilder.AppendNull()
		}
	}

	// Create record
	record := builder.NewRecord()
	defer record.Release()

	// Create a copy for the reader
	record.Retain()
	return array.NewRecordReader(t.schema, []arrow.Record{record})
}

// AddHEP adds a HEP packet to the table's ring buffer
func (t *HEPTable) AddHEP(hep *decoder.HEP) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buffer.Add(hep)
}

// GetStats returns statistics about the table
func (t *HEPTable) GetStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return map[string]interface{}{
		"count":       t.buffer.Count(),
		"buffer_size": t.bufferSize,
	}
}

// RingBuffer is a thread-safe ring buffer for HEP packets
type RingBuffer struct {
	data  []*decoder.HEP
	head  int
	count int
	size  int
	mu    sync.RWMutex
}

// NewRingBuffer creates a new ring buffer
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]*decoder.HEP, size),
		size: size,
	}
}

// Add adds a packet to the buffer
func (rb *RingBuffer) Add(hep *decoder.HEP) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.head] = hep
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	}
}

// GetAll returns all packets in the buffer (oldest first)
func (rb *RingBuffer) GetAll() []*decoder.HEP {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]*decoder.HEP, rb.count)

	if rb.count < rb.size {
		// Buffer not full, data starts at 0
		copy(result, rb.data[:rb.count])
	} else {
		// Buffer full, oldest data starts at head
		start := rb.head
		for i := 0; i < rb.count; i++ {
			result[i] = rb.data[(start+i)%rb.size]
		}
	}

	return result
}

// Count returns the number of packets in the buffer
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Clear clears the buffer
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.head = 0
	rb.count = 0
	for i := range rb.data {
		rb.data[i] = nil
	}
}

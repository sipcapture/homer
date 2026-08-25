// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"fmt"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"github.com/apache/arrow-go/v18/arrow/flight/flightsql/schema_ref"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func flightInfoForCommand(desc *flight.FlightDescriptor, schema *arrow.Schema) *flight.FlightInfo {
	return &flight.FlightInfo{
		Endpoint:         []*flight.FlightEndpoint{{Ticket: &flight.Ticket{Ticket: desc.Cmd}}},
		FlightDescriptor: desc,
		TotalRecords:     -1,
		TotalBytes:       -1,
		Schema:           flight.SerializeSchema(schema, memory.DefaultAllocator),
	}
}

func (s *fsqlServer) GetFlightInfoCatalogs(_ context.Context, desc *flight.FlightDescriptor) (*flight.FlightInfo, error) {
	return flightInfoForCommand(desc, schema_ref.Catalogs), nil
}

func (s *fsqlServer) DoGetCatalogs(context.Context) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	name := s.lakeName
	if name == "" {
		name = "homer_lake"
	}
	return stringColumnStream(schema_ref.Catalogs, []string{name})
}

func (s *fsqlServer) GetFlightInfoSchemas(_ context.Context, _ flightsql.GetDBSchemas, desc *flight.FlightDescriptor) (*flight.FlightInfo, error) {
	return flightInfoForCommand(desc, schema_ref.DBSchemas), nil
}

func (s *fsqlServer) DoGetDBSchemas(_ context.Context, cmd flightsql.GetDBSchemas) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	if pat := cmd.GetDBSchemaFilterPattern(); pat != nil && *pat != "" && *pat != "%" && *pat != "main" {
		return emptyRecordStream(schema_ref.DBSchemas)
	}
	catalog := s.lakeName
	if catalog == "" {
		catalog = "homer_lake"
	}
	return twoStringColumnStream(schema_ref.DBSchemas, []string{catalog}, []string{"main"})
}

func (s *fsqlServer) GetFlightInfoTables(_ context.Context, _ flightsql.GetTables, desc *flight.FlightDescriptor) (*flight.FlightInfo, error) {
	return flightInfoForCommand(desc, schema_ref.Tables), nil
}

func (s *fsqlServer) DoGetTables(ctx context.Context, cmd flightsql.GetTables) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	schema := schema_ref.Tables
	db := s.node.queryDB()
	if db == nil {
		return emptyRecordStream(schema)
	}
	q := `SELECT table_catalog, table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_catalog NOT IN ('system', 'temp', 'memory')`
	if cat := cmd.GetCatalog(); cat != nil && *cat != "" {
		q += fmt.Sprintf(" AND table_catalog = '%s'", escapeSQLLiteral(*cat))
	}
	if pat := cmd.GetDBSchemaFilterPattern(); pat != nil && *pat != "" && *pat != "%" {
		q += fmt.Sprintf(" AND table_schema LIKE '%s'", escapeSQLLiteral(*pat))
	}
	if pat := cmd.GetTableNameFilterPattern(); pat != nil && *pat != "" && *pat != "%" {
		q += fmt.Sprintf(" AND table_name LIKE '%s'", escapeSQLLiteral(*pat))
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return emptyRecordStream(schema)
	}
	defer rows.Close()

	var catalogs, schemas, names, types []string
	for rows.Next() {
		var catalog, sch, name, typ string
		if err := rows.Scan(&catalog, &sch, &name, &typ); err != nil {
			return nil, nil, status.Errorf(codes.Internal, "tables scan: %v", err)
		}
		catalogs = append(catalogs, catalog)
		schemas = append(schemas, sch)
		names = append(names, name)
		types = append(types, typ)
	}
	if len(names) == 0 {
		return emptyRecordStream(schema)
	}
	return fourStringColumnStream(schema, catalogs, schemas, names, types)
}

func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func stringColumnStream(schema *arrow.Schema, values []string) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	bldr := array.NewStringBuilder(memory.DefaultAllocator)
	defer bldr.Release()
	bldr.AppendValues(values, nil)
	arr := bldr.NewArray()
	defer arr.Release()
	batch := array.NewRecord(schema, []arrow.Array{arr}, int64(len(values)))
	return recordStream(schema, batch)
}

func twoStringColumnStream(schema *arrow.Schema, a, b []string) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	ba := array.NewStringBuilder(memory.DefaultAllocator)
	defer ba.Release()
	bb := array.NewStringBuilder(memory.DefaultAllocator)
	defer bb.Release()
	ba.AppendValues(a, nil)
	bb.AppendValues(b, nil)
	aa, ab := ba.NewArray(), bb.NewArray()
	defer aa.Release()
	defer ab.Release()
	batch := array.NewRecord(schema, []arrow.Array{aa, ab}, int64(len(a)))
	return recordStream(schema, batch)
}

func fourStringColumnStream(schema *arrow.Schema, a, b, c, d []string) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	builders := make([]*array.StringBuilder, 4)
	arrs := make([]arrow.Array, 4)
	cols := [][]string{a, b, c, d}
	for i := range builders {
		builders[i] = array.NewStringBuilder(memory.DefaultAllocator)
		builders[i].AppendValues(cols[i], nil)
		arrs[i] = builders[i].NewArray()
		builders[i].Release()
	}
	defer func() {
		for _, arr := range arrs {
			arr.Release()
		}
	}()
	batch := array.NewRecord(schema, arrs, int64(len(a)))
	return recordStream(schema, batch)
}

func emptyRecordStream(schema *arrow.Schema) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	ch := make(chan flight.StreamChunk)
	close(ch)
	return schema, ch, nil
}

func recordStream(schema *arrow.Schema, batch arrow.Record) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	ch := make(chan flight.StreamChunk, 1)
	ch <- flight.StreamChunk{Data: batch}
	close(ch)
	return schema, ch, nil
}

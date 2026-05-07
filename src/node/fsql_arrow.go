// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func arrowSchemaFromRows(rows *sql.Rows) (*arrow.Schema, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	fields := make([]arrow.Field, len(colTypes))
	for i, ct := range colTypes {
		nullable, _ := ct.Nullable()
		fields[i] = arrow.Field{
			Name:     ct.Name(),
			Type:     duckDBTypeToArrow(ct.DatabaseTypeName()),
			Nullable: nullable,
		}
	}
	return arrow.NewSchema(fields, nil), nil
}

const fsqlBatchSize = 1024

func streamRows(ctx context.Context, rows *sql.Rows, schema *arrow.Schema, ch chan<- flight.StreamChunk) error {
	mem := memory.NewGoAllocator()
	numCols := schema.NumFields()
	for {
		builders := make([]array.Builder, numCols)
		for i, f := range schema.Fields() {
			builders[i] = array.NewBuilder(mem, f.Type)
		}
		scanDest := make([]interface{}, numCols)
		scanPtrs := make([]interface{}, numCols)
		for i := range scanPtrs {
			scanPtrs[i] = &scanDest[i]
		}
		n := 0
		for n < fsqlBatchSize && rows.Next() {
			select {
			case <-ctx.Done():
				for _, b := range builders {
					b.Release()
				}
				return status.FromContextError(ctx.Err()).Err()
			default:
			}
			if err := rows.Scan(scanPtrs...); err != nil {
				for _, b := range builders {
					b.Release()
				}
				return status.Errorf(codes.Internal, "scan error: %v", err)
			}
			for i, val := range scanDest {
				appendFsqlValue(builders[i], schema.Field(i).Type, val)
			}
			n++
		}
		if n == 0 {
			for _, b := range builders {
				b.Release()
			}
			break
		}
		cols := make([]arrow.Array, numCols)
		for i, b := range builders {
			cols[i] = b.NewArray()
			b.Release()
		}
		rec := array.NewRecord(schema, cols, int64(n))
		for _, c := range cols {
			c.Release()
		}
		ch <- flight.StreamChunk{Data: rec}
		if n < fsqlBatchSize {
			break
		}
	}
	return rows.Err()
}

func appendFsqlValue(b array.Builder, dt arrow.DataType, val interface{}) {
	if val == nil {
		b.AppendNull()
		return
	}
	switch bldr := b.(type) {
	case *array.BooleanBuilder:
		switch v := val.(type) {
		case bool:
			bldr.Append(v)
		default:
			bldr.AppendNull()
		}
	case *array.Int8Builder:
		bldr.Append(toFsqlInt8(val))
	case *array.Int16Builder:
		bldr.Append(toFsqlInt16(val))
	case *array.Int32Builder:
		bldr.Append(toFsqlInt32(val))
	case *array.Int64Builder:
		bldr.Append(toFsqlInt64(val))
	case *array.Uint8Builder:
		bldr.Append(uint8(toFsqlInt64(val)))
	case *array.Uint16Builder:
		bldr.Append(uint16(toFsqlInt64(val)))
	case *array.Uint32Builder:
		bldr.Append(uint32(toFsqlInt64(val)))
	case *array.Uint64Builder:
		bldr.Append(uint64(toFsqlInt64(val)))
	case *array.Float32Builder:
		bldr.Append(float32(toFsqlFloat64(val)))
	case *array.Float64Builder:
		bldr.Append(toFsqlFloat64(val))
	case *array.Date32Builder:
		switch v := val.(type) {
		case time.Time:
			bldr.Append(arrow.Date32(int32(v.Unix() / 86400)))
		case string:
			if t, err := time.Parse("2006-01-02", v); err == nil {
				bldr.Append(arrow.Date32(int32(t.Unix() / 86400)))
			} else {
				bldr.AppendNull()
			}
		default:
			bldr.AppendNull()
		}
	case *array.Time64Builder:
		switch v := val.(type) {
		case time.Time:
			micros := int64(v.Hour())*3600_000000 + int64(v.Minute())*60_000000 + int64(v.Second())*1_000000 + int64(v.Nanosecond()/1000)
			bldr.Append(arrow.Time64(micros))
		default:
			bldr.AppendNull()
		}
	case *array.TimestampBuilder:
		switch v := val.(type) {
		case time.Time:
			bldr.Append(arrow.Timestamp(v.UnixNano()))
		case string:
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
				if t, err := time.Parse(layout, v); err == nil {
					bldr.Append(arrow.Timestamp(t.UnixNano()))
					return
				}
			}
			bldr.AppendNull()
		default:
			bldr.AppendNull()
		}
	case *array.DurationBuilder:
		bldr.Append(0)
	case *array.BinaryBuilder:
		switch v := val.(type) {
		case []byte:
			bldr.Append(v)
		case string:
			bldr.Append([]byte(v))
		default:
			bldr.Append([]byte(fmt.Sprintf("%v", v)))
		}
	case *array.StringBuilder:
		switch v := val.(type) {
		case string:
			bldr.Append(v)
		case []byte:
			bldr.Append(string(v))
		case *big.Int:
			bldr.Append(v.String())
		case time.Time:
			bldr.Append(v.UTC().Format(time.RFC3339Nano))
		default:
			bldr.Append(fmt.Sprintf("%v", v))
		}
	default:
		b.AppendNull()
	}
}

func toFsqlInt8(v interface{}) int8 {
	switch x := v.(type) {
	case int8:
		return x
	case int16:
		return int8(x)
	case int32:
		return int8(x)
	case int64:
		return int8(x)
	case int:
		return int8(x)
	default:
		return 0
	}
}

func toFsqlInt16(v interface{}) int16 {
	switch x := v.(type) {
	case int8:
		return int16(x)
	case int16:
		return x
	case int32:
		return int16(x)
	case int64:
		return int16(x)
	case int:
		return int16(x)
	default:
		return 0
	}
}

func toFsqlInt32(v interface{}) int32 {
	switch x := v.(type) {
	case int8:
		return int32(x)
	case int16:
		return int32(x)
	case int32:
		return x
	case int64:
		return int32(x)
	case int:
		return int32(x)
	default:
		return 0
	}
}

func toFsqlInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case uint64:
		return int64(x)
	default:
		return 0
	}
}

func toFsqlFloat64(v interface{}) float64 {
	switch x := v.(type) {
	case float32:
		return float64(x)
	case float64:
		return x
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}

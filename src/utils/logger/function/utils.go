// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package function

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Jeffail/gabs/v2"
)

func KeyExits(value interface{}, keyMap []interface{}) bool {
	for _, v := range keyMap {
		if value == v {
			return true
		}
	}
	return false
}

func ArrayKeyExits(key string, dataKeys *gabs.Container) bool {
	for _, v := range dataKeys.Children() {
		if key == v.Data().(string) {
			return true
		}
	}
	return false
}

// DBFields reflects on a struct and returns the values of fields with `db` tags,
// or a map[string]interface{} and returns the keys.
func DBFields(values interface{}) []string {
	v := reflect.ValueOf(values)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	fields := []string{}
	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i).Tag.Get("db")
			clickhouse := v.Type().Field(i).Tag.Get("clickhouse")

			if field != "" && field != "-" && !strings.Contains(clickhouse, "materialized") {
				fields = append(fields, field)
			}
		}
		return fields
	}

	panic(fmt.Errorf("DBFields requires a struct or a map, found: %s", v.Kind().String()))
}

// DBFields reflects on a struct and returns the values of fields with `db` tags,
// or a map[string]interface{} and returns the keys.
func GenerateArg(values interface{}) []interface{} {

	v := reflect.ValueOf(values)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var fields []interface{}

	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i).Tag.Get("db")
			clickhouse := v.Type().Field(i).Tag.Get("clickhouse")

			if field != "" && field != "-" && !strings.Contains(clickhouse, "materialized") {
				fields = append(fields, v.Field(i).Interface())
			}
		}
		return fields
	}

	panic(fmt.Errorf("DBFields requires a struct or a map, found: %s", v.Kind().String()))
}

func FieldName(fields []string) string {
	return strings.Join(fields[:], ",")
}

func FieldValue(fields []string) string {
	return ":" + strings.Join(fields[:], ",:")
}

func FieldPrepare(fields []string) string {
	return strings.Repeat("?,", len(fields)-1) + "?"
}

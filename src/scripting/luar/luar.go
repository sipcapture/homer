// Copyright (c) 2010-2016 Steve Donovan
// Adapted for Homer-Core

package luar

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/sipcapture/golua/lua"
)

// ConvError records a conversion error from value 'From' to value 'To'.
type ConvError struct {
	From interface{}
	To   interface{}
}

// ErrTableConv arises when some table entries could not be converted.
var ErrTableConv = errors.New("some table elements could not be converted")

func (l ConvError) Error() string {
	return fmt.Sprintf("cannot convert %v to %v", l.From, l.To)
}

// Lua 5.1 'lua_tostring' function only supports string and numbers.
func luaToString(L *lua.State, idx int) string {
	switch L.Type(idx) {
	case lua.LUA_TNUMBER:
		L.PushValue(idx)
		defer L.Pop(1)
		return L.ToString(-1)
	case lua.LUA_TSTRING:
		return L.ToString(-1)
	case lua.LUA_TBOOLEAN:
		b := L.ToBoolean(idx)
		if b {
			return "true"
		}
		return "false"
	case lua.LUA_TNIL:
		return "nil"
	}
	return fmt.Sprintf("%s: %v", L.LTypename(idx), L.ToPointer(idx))
}

func luaDesc(L *lua.State, idx int) string {
	return fmt.Sprintf("Lua value '%v' (%v)", luaToString(L, idx), L.LTypename(idx))
}

// NullT is the type of Null.
type NullT int

// Map is an alias for map of strings.
type Map map[string]interface{}

var (
	// Null is the definition of 'luar.null' which is used in place of 'nil'
	Null = NullT(0)
)

var (
	tslice = typeof((*[]interface{})(nil))
	tmap   = typeof((*map[string]interface{})(nil))
	nullv  = reflect.ValueOf(Null)
)

// visitor holds the index to the table in LUA_REGISTRYINDEX
type visitor struct {
	L     *lua.State
	index int
}

func newVisitor(L *lua.State) visitor {
	var v visitor
	v.L = L
	v.L.NewTable()
	v.index = v.L.Ref(lua.LUA_REGISTRYINDEX)
	return v
}

func (v *visitor) close() {
	v.L.Unref(lua.LUA_REGISTRYINDEX, v.index)
}

func (v *visitor) mark(val reflect.Value) {
	ptr := val.Pointer()
	if ptr == 0 {
		return
	}

	v.L.RawGeti(lua.LUA_REGISTRYINDEX, v.index)
	v.L.PushValue(-2)
	v.L.RawSeti(-2, int(ptr))
	v.L.Pop(1)
}

func (v *visitor) push(val reflect.Value) bool {
	ptr := val.Pointer()
	v.L.RawGeti(lua.LUA_REGISTRYINDEX, v.index)
	v.L.RawGeti(-1, int(ptr))
	if v.L.IsNil(-1) {
		v.L.Pop(2)
		return false
	}
	v.L.Replace(-2)
	return true
}

func isNil(v reflect.Value) bool {
	nullables := [...]bool{
		reflect.Chan:      true,
		reflect.Func:      true,
		reflect.Interface: true,
		reflect.Map:       true,
		reflect.Ptr:       true,
		reflect.Slice:     true,
	}

	kind := v.Type().Kind()
	if int(kind) >= len(nullables) {
		return false
	}
	return nullables[kind] && v.IsNil()
}

func copyMapToTable(L *lua.State, v reflect.Value, visited visitor) {
	n := v.Len()
	L.CreateTable(0, n)
	visited.mark(v)
	for _, key := range v.MapKeys() {
		val := v.MapIndex(key)
		goToLua(L, key, true, visited)
		if isNil(val) {
			val = nullv
		}
		goToLua(L, val, false, visited)
		L.SetTable(-3)
	}
}

func copySliceToTable(L *lua.State, v reflect.Value, visited visitor) {
	vp := v
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	n := v.Len()
	L.CreateTable(n, 0)
	if v.Kind() == reflect.Slice {
		visited.mark(v)
	} else if vp.Kind() == reflect.Ptr {
		visited.mark(vp)
	}

	for i := 0; i < n; i++ {
		L.PushInteger(int64(i + 1))
		val := v.Index(i)
		if isNil(val) {
			val = nullv
		}
		goToLua(L, val, false, visited)
		L.SetTable(-3)
	}
}

func copyStructToTable(L *lua.State, v reflect.Value, visited visitor) {
	vp := v
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	n := v.NumField()
	L.CreateTable(n, 0)
	if vp.Kind() == reflect.Ptr {
		visited.mark(vp)
	}

	for i := 0; i < n; i++ {
		st := v.Type()
		field := st.Field(i)
		key := field.Name
		tag := field.Tag.Get("lua")
		if tag != "" {
			key = tag
		}
		goToLua(L, key, false, visited)
		val := v.Field(i)
		goToLua(L, val, false, visited)
		L.SetTable(-3)
	}
}

func callGoFunction(L *lua.State, v reflect.Value, args []reflect.Value) []reflect.Value {
	defer func() {
		if x := recover(); x != nil {
			L.RaiseError(fmt.Sprintf("error %s", x))
		}
	}()
	results := v.Call(args)
	return results
}

func goToLuaFunction(L *lua.State, v reflect.Value) lua.LuaGoFunction {
	switch f := v.Interface().(type) {
	case func(*lua.State) int:
		return f
	}

	t := v.Type()
	argsT := make([]reflect.Type, t.NumIn())
	for i := range argsT {
		argsT[i] = t.In(i)
	}

	return func(L *lua.State) int {
		var lastT reflect.Type
		isVariadic := t.IsVariadic()

		if isVariadic {
			n := len(argsT)
			lastT = argsT[n-1].Elem()
			argsT = argsT[:n-1]
		}

		args := make([]reflect.Value, len(argsT))
		for i, t := range argsT {
			val := reflect.New(t)
			err := LuaToGo(L, i+1, val.Interface())
			if err != nil {
				L.RaiseError(fmt.Sprintf("cannot convert Go function argument #%v: %v", i, err))
			}
			args[i] = val.Elem()
		}

		if isVariadic {
			n := L.GetTop()
			for i := len(argsT) + 1; i <= n; i++ {
				val := reflect.New(lastT)
				err := LuaToGo(L, i, val.Interface())
				if err != nil {
					L.RaiseError(fmt.Sprintf("cannot convert Go function argument #%v: %v", i, err))
				}
				args = append(args, val.Elem())
			}
			argsT = argsT[:len(argsT)+1]
		}
		results := callGoFunction(L, v, args)
		for _, val := range results {
			GoToLuaProxy(L, val)
		}
		return len(results)
	}
}

// GoToLua pushes a Go value 'val' on the Lua stack.
func GoToLua(L *lua.State, a interface{}) {
	visited := newVisitor(L)
	goToLua(L, a, false, visited)
	visited.close()
}

// GoToLuaProxy is like GoToLua but pushes a proxy on the Lua stack when it makes sense.
func GoToLuaProxy(L *lua.State, a interface{}) {
	visited := newVisitor(L)
	goToLua(L, a, true, visited)
	visited.close()
}

func goToLua(L *lua.State, a interface{}, proxify bool, visited visitor) {
	var v reflect.Value
	v, ok := a.(reflect.Value)
	if !ok {
		v = reflect.ValueOf(a)
	}
	if !v.IsValid() {
		L.PushNil()
		return
	}

	if v.Kind() == reflect.Interface && !v.IsNil() {
		v = reflect.ValueOf(v.Interface())
	}

	vp := v
	for v.Kind() == reflect.Ptr {
		vp = v
		v = v.Elem()
	}

	if !v.IsValid() {
		L.PushNil()
		return
	}

	if v.CanInterface() && v.Interface() == Null {
		makeValueProxy(L, v, cInterfaceMeta)
		return
	}

	switch v.Kind() {
	case reflect.Float64, reflect.Float32:
		if proxify && isNewType(v.Type()) {
			makeValueProxy(L, vp, cNumberMeta)
		} else {
			L.PushNumber(v.Float())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if proxify && isNewType(v.Type()) {
			makeValueProxy(L, vp, cNumberMeta)
		} else {
			L.PushNumber(float64(v.Int()))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if proxify && isNewType(v.Type()) {
			makeValueProxy(L, vp, cNumberMeta)
		} else {
			L.PushNumber(float64(v.Uint()))
		}
	case reflect.String:
		if proxify && isNewType(v.Type()) {
			makeValueProxy(L, vp, cStringMeta)
		} else {
			L.PushString(v.String())
		}
	case reflect.Bool:
		if proxify && isNewType(v.Type()) {
			makeValueProxy(L, vp, cInterfaceMeta)
		} else {
			L.PushBoolean(v.Bool())
		}
	case reflect.Complex128, reflect.Complex64:
		makeValueProxy(L, vp, cComplexMeta)
	case reflect.Array:
		if proxify {
			vRawType := reflect.ArrayOf(v.Type().Len(), v.Type().Elem())
			if vRawType != v.Type() || v.CanSet() {
				if !v.CanSet() {
					vp = reflect.New(v.Type())
					reflect.Copy(vp.Elem(), v)
					vp = vp.Elem()
				}
				makeValueProxy(L, vp, cSliceMeta)
				return
			}
		}
		if vp.Kind() == reflect.Ptr && visited.push(vp) {
			return
		}
		copySliceToTable(L, vp, visited)
	case reflect.Slice:
		if proxify {
			makeValueProxy(L, vp, cSliceMeta)
		} else {
			if visited.push(v) {
				return
			}
			copySliceToTable(L, v, visited)
		}
	case reflect.Map:
		if proxify {
			makeValueProxy(L, vp, cMapMeta)
		} else {
			if visited.push(v) {
				return
			}
			copyMapToTable(L, v, visited)
		}
	case reflect.Struct:
		if proxify {
			if vp.CanInterface() {
				switch val := vp.Interface().(type) {
				case error:
					L.PushString(val.Error())
					return
				case *LuaObject:
					if val.l == L {
						val.Push()
					} else {
						L.PushNil()
					}
					return
				default:
				}
			}

			if !v.CanSet() {
				vp = reflect.New(v.Type())
				vp.Elem().Set(v)
			}
			makeValueProxy(L, vp, cStructMeta)
		} else {
			if vp.Kind() == reflect.Ptr && visited.push(vp) {
				return
			}
			copyStructToTable(L, vp, visited)
		}
	case reflect.Chan:
		makeValueProxy(L, vp, cChannelMeta)
	case reflect.Func:
		L.PushGoFunction(goToLuaFunction(L, v))
	default:
		if val, ok := v.Interface().(error); ok {
			L.PushString(val.Error())
		} else if v.IsNil() {
			L.PushNil()
		} else {
			makeValueProxy(L, vp, cInterfaceMeta)
		}
	}
}

func luaIsEmpty(L *lua.State, idx int) bool {
	L.PushNil()
	if idx < 0 {
		idx--
	}
	if L.Next(idx) != 0 {
		L.Pop(2)
		return false
	}
	return true
}

func luaMapLen(L *lua.State, idx int) int {
	L.PushNil()
	if idx < 0 {
		idx--
	}
	len := 0
	for L.Next(idx) != 0 {
		len++
		L.Pop(1)
	}
	return len
}

func copyTableToMap(L *lua.State, idx int, v reflect.Value, visited map[uintptr]reflect.Value) (status error) {
	t := v.Type()
	if v.IsNil() {
		v.Set(reflect.MakeMap(t))
	}
	te, tk := t.Elem(), t.Key()

	ptr := L.ToPointer(idx)
	if !luaIsEmpty(L, idx) {
		visited[ptr] = v
	}

	L.PushNil()
	if idx < 0 {
		idx--
	}
	for L.Next(idx) != 0 {
		key := reflect.New(tk).Elem()
		err := luaToGo(L, -2, key, visited)
		if err != nil {
			status = ErrTableConv
			L.Pop(1)
			continue
		}
		val := reflect.New(te).Elem()
		err = luaToGo(L, -1, val, visited)
		if err != nil {
			status = ErrTableConv
			L.Pop(1)
			continue
		}
		v.SetMapIndex(key, val)
		L.Pop(1)
	}

	return
}

func copyTableToSlice(L *lua.State, idx int, v reflect.Value, visited map[uintptr]reflect.Value) (status error) {
	t := v.Type()
	n := int(L.ObjLen(idx))

	if n > v.Len() {
		if t.Kind() == reflect.Array {
			n = v.Len()
		} else {
			v.Set(reflect.MakeSlice(t, n, n))
		}
	} else if n < v.Len() {
		if t.Kind() == reflect.Array {
			for i := n; i < v.Len(); i++ {
				v.Index(i).Set(reflect.Zero(t.Elem()))
			}
		} else {
			v.SetLen(n)
		}
	}

	if n > 0 && t.Kind() != reflect.Array {
		ptr := L.ToPointer(idx)
		visited[ptr] = v
	}

	te := t.Elem()
	for i := 1; i <= n; i++ {
		L.RawGeti(idx, i)
		val := reflect.New(te).Elem()
		err := luaToGo(L, -1, val, visited)
		if err != nil {
			status = ErrTableConv
			L.Pop(1)
			continue
		}
		v.Index(i - 1).Set(val)
		L.Pop(1)
	}

	return
}

func copyTableToStruct(L *lua.State, idx int, v reflect.Value, visited map[uintptr]reflect.Value) (status error) {
	t := v.Type()

	ptr := L.ToPointer(idx)
	if !luaIsEmpty(L, idx) {
		visited[ptr] = v.Addr()
	}

	fields := map[string]string{}
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("lua")
		if tag != "" {
			fields[tag] = field.Name
			continue
		}
		fields[field.Name] = field.Name
	}

	L.PushNil()
	if idx < 0 {
		idx--
	}
	for L.Next(idx) != 0 {
		L.PushValue(-2)
		key := L.ToString(-1)
		L.Pop(1)
		f := v.FieldByName(fields[key])
		if f.CanSet() {
			val := reflect.New(f.Type()).Elem()
			err := luaToGo(L, -1, val, visited)
			if err != nil {
				status = ErrTableConv
				L.Pop(1)
				continue
			}
			f.Set(val)
		}
		L.Pop(1)
	}

	return
}

// LuaToGo converts the Lua value at index 'idx' to the Go value.
func LuaToGo(L *lua.State, idx int, a interface{}) error {
	v := reflect.ValueOf(a)
	if v.Kind() != reflect.Ptr {
		return errors.New("not a pointer")
	}
	if v.IsNil() {
		return errors.New("nil pointer")
	}

	v = v.Elem()
	if v.Kind() == reflect.Ptr && L.IsNil(idx) {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	return luaToGo(L, idx, v, map[uintptr]reflect.Value{})
}

func luaToGo(L *lua.State, idx int, v reflect.Value, visited map[uintptr]reflect.Value) error {
	vp := v
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		vp = v
		v = v.Elem()
	}
	kind := v.Kind()

	switch L.Type(idx) {
	case lua.LUA_TNIL:
		v.Set(reflect.Zero(v.Type()))
	case lua.LUA_TBOOLEAN:
		if kind != reflect.Bool && kind != reflect.Interface {
			return ConvError{From: luaDesc(L, idx), To: v.Type()}
		}
		v.Set(reflect.ValueOf(L.ToBoolean(idx)))
	case lua.LUA_TNUMBER:
		switch k := unsizedKind(v); k {
		case reflect.Int64, reflect.Uint64, reflect.Float64, reflect.Interface:
			f := reflect.ValueOf(L.ToNumber(idx))
			v.Set(f.Convert(v.Type()))
		case reflect.Complex128:
			v.SetComplex(complex(L.ToNumber(idx), 0))
		default:
			return ConvError{From: luaDesc(L, idx), To: v.Type()}
		}
	case lua.LUA_TSTRING:
		if kind != reflect.String && kind != reflect.Interface {
			return ConvError{From: luaDesc(L, idx), To: v.Type()}
		}
		v.Set(reflect.ValueOf(L.ToString(idx)))
	case lua.LUA_TUSERDATA:
		if isValueProxy(L, idx) {
			val, typ := valueOfProxy(L, idx)
			if val.Interface() == Null {
				v.Set(reflect.Zero(v.Type()))
				return nil
			}

			if typ.ConvertibleTo(vp.Type()) {
				vp.Set(val.Convert(vp.Type()))
				return nil
			}

			for !typ.ConvertibleTo(v.Type()) && val.Kind() == reflect.Ptr {
				val = val.Elem()
				typ = typ.Elem()
			}
			if !typ.ConvertibleTo(v.Type()) {
				return ConvError{From: fmt.Sprintf("proxy (%v)", typ), To: v.Type()}
			}
			v.Set(val.Convert(v.Type()))
			return nil
		} else if kind != reflect.Interface || v.Type() != reflect.TypeOf(LuaObject{}) {
			return ConvError{From: luaDesc(L, idx), To: v.Type()}
		}
		v.Set(reflect.ValueOf(NewLuaObject(L, idx)))
	case lua.LUA_TTABLE:
		ptr := L.ToPointer(idx)
		if val, ok := visited[ptr]; ok {
			if v.Kind() == reflect.Struct && val.Type().ConvertibleTo(vp.Type()) {
				vp.Set(val)
				return nil
			} else if val.Type().ConvertibleTo(v.Type()) {
				v.Set(val)
				return nil
			}
		}

		switch kind {
		case reflect.Array:
			fallthrough
		case reflect.Slice:
			return copyTableToSlice(L, idx, v, visited)
		case reflect.Map:
			return copyTableToMap(L, idx, v, visited)
		case reflect.Struct:
			return copyTableToStruct(L, idx, v, visited)
		case reflect.Interface:
			n := int(L.ObjLen(idx))

			switch v.Elem().Kind() {
			case reflect.Map:
				return copyTableToMap(L, idx, v.Elem(), visited)
			case reflect.Slice:
				v.Set(reflect.MakeSlice(v.Elem().Type(), n, n))
				return copyTableToSlice(L, idx, v.Elem(), visited)
			}

			if luaMapLen(L, idx) != n {
				v.Set(reflect.MakeMap(tmap))
				return copyTableToMap(L, idx, v.Elem(), visited)
			}
			v.Set(reflect.MakeSlice(tslice, n, n))
			return copyTableToSlice(L, idx, v.Elem(), visited)
		default:
			return ConvError{From: luaDesc(L, idx), To: v.Type()}
		}
	case lua.LUA_TFUNCTION:
		if kind == reflect.Interface {
			v.Set(reflect.ValueOf(NewLuaObject(L, idx)))
		} else if vp.Type() == reflect.TypeOf(&LuaObject{}) {
			vp.Set(reflect.ValueOf(NewLuaObject(L, idx)))
		} else {
			return ConvError{From: luaDesc(L, idx), To: v.Type()}
		}
	default:
		return ConvError{From: luaDesc(L, idx), To: v.Type()}
	}

	return nil
}

func isNewType(t reflect.Type) bool {
	types := [...]reflect.Type{
		reflect.Invalid:    nil,
		reflect.Bool:       typeof((*bool)(nil)),
		reflect.Int:        typeof((*int)(nil)),
		reflect.Int8:       typeof((*int8)(nil)),
		reflect.Int16:      typeof((*int16)(nil)),
		reflect.Int32:      typeof((*int32)(nil)),
		reflect.Int64:      typeof((*int64)(nil)),
		reflect.Uint:       typeof((*uint)(nil)),
		reflect.Uint8:      typeof((*uint8)(nil)),
		reflect.Uint16:     typeof((*uint16)(nil)),
		reflect.Uint32:     typeof((*uint32)(nil)),
		reflect.Uint64:     typeof((*uint64)(nil)),
		reflect.Uintptr:    typeof((*uintptr)(nil)),
		reflect.Float32:    typeof((*float32)(nil)),
		reflect.Float64:    typeof((*float64)(nil)),
		reflect.Complex64:  typeof((*complex64)(nil)),
		reflect.Complex128: typeof((*complex128)(nil)),
		reflect.String:     typeof((*string)(nil)),
	}

	pt := types[int(t.Kind())]
	return pt != t
}

// Register makes a number of Go values available in Lua code as proxies.
func Register(L *lua.State, table string, values Map) {
	pop := true
	if table == "*" {
		pop = false
	} else if len(table) > 0 {
		L.GetGlobal(table)
		if L.IsNil(-1) {
			L.Pop(1)
			L.NewTable()
			L.SetGlobal(table)
			L.GetGlobal(table)
		}
	} else {
		L.GetGlobal("_G")
	}
	for name, val := range values {
		GoToLuaProxy(L, val)
		L.SetField(-2, name)
	}
	if pop {
		L.Pop(1)
	}
}

func typeof(a interface{}) reflect.Type {
	return reflect.TypeOf(a).Elem()
}

// Copyright (c) 2010-2016 Steve Donovan
// Adapted for Homer-Core

package luar

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/sipcapture/golua/lua"
)

// Lua proxy objects for Go slices, maps and structs
type valueProxy struct {
	v reflect.Value
	t reflect.Type
}

const (
	cNumberMeta    = "numberMT"
	cComplexMeta   = "complexMT"
	cStringMeta    = "stringMT"
	cSliceMeta     = "sliceMT"
	cMapMeta       = "mapMT"
	cStructMeta    = "structMT"
	cInterfaceMeta = "interfaceMT"
	cChannelMeta   = "channelMT"
)

var (
	proxyIdCounter uintptr
	proxyMap       = map[uintptr]*valueProxy{}
	proxymu        sync.RWMutex
)

func commonKind(v1, v2 reflect.Value) reflect.Kind {
	k1 := unsizedKind(v1)
	k2 := unsizedKind(v2)
	if k1 == k2 && (k1 == reflect.Uint64 || k1 == reflect.Int64) {
		return k1
	}
	if k1 == reflect.Complex128 || k2 == reflect.Complex128 {
		return reflect.Complex128
	}
	return reflect.Float64
}

func isPointerToPrimitive(v reflect.Value) bool {
	return v.Kind() == reflect.Ptr && v.Elem().IsValid() && v.Elem().Type() != nil
}

func isPredeclaredType(t reflect.Type) bool {
	return t == reflect.TypeOf(0.0) || t == reflect.TypeOf("")
}

func isValueProxy(L *lua.State, idx int) bool {
	res := false
	if L.IsUserdata(idx) {
		L.GetMetaTable(idx)
		if !L.IsNil(-1) {
			L.GetField(-1, "luago.value")
			res = !L.IsNil(-1)
			L.Pop(1)
		}
		L.Pop(1)
	}
	return res
}

func luaToGoValue(L *lua.State, idx int) (reflect.Value, reflect.Type) {
	var a interface{}
	err := LuaToGo(L, idx, &a)
	if err != nil {
		L.RaiseError(err.Error())
	}
	return reflect.ValueOf(a), reflect.TypeOf(a)
}

func makeValueProxy(L *lua.State, v reflect.Value, proxyMT string) {
	L.LGetMetaTable(proxyMT)
	if L.IsNil(-1) {
		flagValue := func() {
			L.SetMetaMethod("__tostring", proxy__tostring)
			L.SetMetaMethod("__gc", proxy__gc)
			L.SetMetaMethod("__eq", proxy__eq)
			L.PushBoolean(true)
			L.SetField(-2, "luago.value")
			L.Pop(1)
		}
		switch proxyMT {
		case cNumberMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", interface__index)
			L.SetMetaMethod("__lt", number__lt)
			L.SetMetaMethod("__add", number__add)
			L.SetMetaMethod("__sub", number__sub)
			L.SetMetaMethod("__mul", number__mul)
			L.SetMetaMethod("__div", number__div)
			L.SetMetaMethod("__mod", number__mod)
			L.SetMetaMethod("__pow", number__pow)
			L.SetMetaMethod("__unm", number__unm)
			flagValue()
		case cComplexMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", complex__index)
			L.SetMetaMethod("__add", number__add)
			L.SetMetaMethod("__sub", number__sub)
			L.SetMetaMethod("__mul", number__mul)
			L.SetMetaMethod("__div", number__div)
			L.SetMetaMethod("__pow", number__pow)
			L.SetMetaMethod("__unm", number__unm)
			flagValue()
		case cStringMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", string__index)
			L.SetMetaMethod("__len", string__len)
			L.SetMetaMethod("__lt", string__lt)
			L.SetMetaMethod("__concat", string__concat)
			L.SetMetaMethod("__ipairs", string__ipairs)
			L.SetMetaMethod("__pairs", string__ipairs)
			flagValue()
		case cSliceMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", slice__index)
			L.SetMetaMethod("__newindex", slice__newindex)
			L.SetMetaMethod("__len", slicemap__len)
			L.SetMetaMethod("__ipairs", slice__ipairs)
			L.SetMetaMethod("__pairs", slice__ipairs)
			flagValue()
		case cMapMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", map__index)
			L.SetMetaMethod("__newindex", map__newindex)
			L.SetMetaMethod("__len", slicemap__len)
			L.SetMetaMethod("__ipairs", map__ipairs)
			L.SetMetaMethod("__pairs", map__pairs)
			flagValue()
		case cStructMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", struct__index)
			L.SetMetaMethod("__newindex", struct__newindex)
			flagValue()
		case cInterfaceMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", interface__index)
			flagValue()
		case cChannelMeta:
			L.NewMetaTable(proxyMT)
			L.SetMetaMethod("__index", channel__index)
			flagValue()
		}
	}

	proxymu.Lock()
	id := proxyIdCounter
	proxyIdCounter++
	proxyMap[id] = &valueProxy{v: v, t: v.Type()}
	proxymu.Unlock()

	L.Pop(1)
	rawptr := L.NewUserdata(reflect.TypeOf(id).Size())
	*(*uintptr)(rawptr) = id
	L.LGetMetaTable(proxyMT)
	L.SetMetaTable(-2)
}

func pushGoMethod(L *lua.State, name string, v reflect.Value) {
	method := v.MethodByName(name)
	if !method.IsValid() {
		t := v.Type()
		if t.Kind() != reflect.Ptr {
			if v.CanAddr() {
				v = v.Addr()
			} else {
				vp := reflect.New(t)
				vp.Elem().Set(v)
				v = vp
			}
		}
		method = v.MethodByName(name)
		if !method.IsValid() {
			L.PushNil()
			return
		}
	}
	GoToLua(L, method)
}

func pushNumberValue(L *lua.State, a interface{}, t1, t2 reflect.Type) {
	v := reflect.ValueOf(a)
	isComplex := unsizedKind(v) == reflect.Complex128
	mt := cNumberMeta
	if isComplex {
		mt = cComplexMeta
	}
	if t1 == t2 || isPredeclaredType(t2) {
		makeValueProxy(L, v.Convert(t1), mt)
	} else if isPredeclaredType(t1) {
		makeValueProxy(L, v.Convert(t2), mt)
	} else if isComplex {
		complexType := reflect.TypeOf(0i)
		makeValueProxy(L, v.Convert(complexType), cComplexMeta)
	} else {
		L.PushNumber(valueToNumber(L, v))
	}
}

func slicer(L *lua.State, v reflect.Value, metatable string) lua.LuaGoFunction {
	return func(L *lua.State) int {
		L.CheckInteger(1)
		L.CheckInteger(2)
		i := L.ToInteger(1) - 1
		j := L.ToInteger(2) - 1
		if i < 0 || i >= v.Len() || i > j || j > v.Len() {
			L.RaiseError("slice bounds out of range")
		}
		vn := v.Slice(i, j)
		makeValueProxy(L, vn, metatable)
		return 1
	}
}

func unsizedKind(v reflect.Value) reflect.Kind {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.Int64
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflect.Uint64
	case reflect.Float64, reflect.Float32:
		return reflect.Float64
	case reflect.Complex128, reflect.Complex64:
		return reflect.Complex128
	}
	return v.Kind()
}

func valueOfProxy(L *lua.State, idx int) (reflect.Value, reflect.Type) {
	proxyId := *(*uintptr)(L.ToUserdata(idx))

	proxymu.RLock()
	val, ok := proxyMap[proxyId]
	proxymu.RUnlock()

	if !ok {
		L.RaiseError(fmt.Sprintf("No value proxy in arg #%d", idx))
	}

	return val.v, val.t
}

func valueToComplex(L *lua.State, v reflect.Value) complex128 {
	if unsizedKind(v) == reflect.Complex128 {
		return v.Complex()
	}
	return complex(valueToNumber(L, v), 0)
}

func valueToNumber(L *lua.State, v reflect.Value) float64 {
	switch unsizedKind(v) {
	case reflect.Int64:
		return float64(v.Int())
	case reflect.Uint64:
		return float64(v.Uint())
	case reflect.Float64:
		return v.Float()
	case reflect.String:
		if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
			return f
		}
	}
	L.RaiseError(fmt.Sprintf("cannot convert %#v to number", v))
	return 0
}

func valueToString(L *lua.State, v reflect.Value) string {
	switch unsizedKind(v) {
	case reflect.Uint64, reflect.Int64, reflect.Float64:
		return fmt.Sprintf("%v", valueToNumber(L, v))
	case reflect.String:
		return v.String()
	}
	L.RaiseError("cannot convert to string")
	return ""
}

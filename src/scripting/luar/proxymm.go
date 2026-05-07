// Copyright (c) 2010-2016 Steve Donovan
// Adapted for Homer-Core

package luar

import (
	"fmt"
	"math"
	"math/cmplx"
	"reflect"

	"github.com/sipcapture/golua/lua"
)

func channel__index(L *lua.State) int {
	v, t := valueOfProxy(L, 1)
	name := L.ToString(2)
	switch name {
	case "recv":
		f := func(L *lua.State) int {
			val, ok := v.Recv()
			if ok {
				GoToLuaProxy(L, val)
				return 1
			}
			return 0
		}
		L.PushGoFunction(f)
	case "send":
		f := func(L *lua.State) int {
			val := reflect.New(t.Elem())
			err := LuaToGo(L, 1, val.Interface())
			if err != nil {
				L.RaiseError(fmt.Sprintf("channel requires %v value type", t.Elem()))
			}
			v.Send(val.Elem())
			return 0
		}
		L.PushGoFunction(f)
	case "close":
		f := func(L *lua.State) int {
			v.Close()
			return 0
		}
		L.PushGoFunction(f)
	default:
		pushGoMethod(L, name, v)
	}
	return 1
}

func complex__index(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	name := L.ToString(2)
	switch name {
	case "real":
		L.PushNumber(real(v.Complex()))
	case "imag":
		L.PushNumber(imag(v.Complex()))
	default:
		pushGoMethod(L, name, v)
	}
	return 1
}

func interface__index(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	name := L.ToString(2)
	pushGoMethod(L, name, v)
	return 1
}

func map__index(L *lua.State) int {
	v, t := valueOfProxy(L, 1)
	key := reflect.New(t.Key())
	err := LuaToGo(L, 2, key.Interface())
	if err == nil {
		key = key.Elem()
		val := v.MapIndex(key)
		if val.IsValid() {
			GoToLuaProxy(L, val)
			return 1
		}
	}
	if !L.IsNumber(2) && L.IsString(2) {
		name := L.ToString(2)
		pushGoMethod(L, name, v)
		return 1
	}
	if err != nil {
		L.RaiseError(fmt.Sprintf("map requires %v key", t.Key()))
	}
	return 0
}

func map__ipairs(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	keys := v.MapKeys()
	intKeys := map[uint64]reflect.Value{}

	for _, k := range keys {
		if k.Kind() == reflect.Interface {
			k = k.Elem()
		}
		switch unsizedKind(k) {
		case reflect.Int64:
			i := k.Int()
			if i > 0 {
				intKeys[uint64(i)] = k
			}
		case reflect.Uint64:
			intKeys[k.Uint()] = k
		}
	}

	idx := uint64(0)
	iter := func(L *lua.State) int {
		idx++
		if _, ok := intKeys[idx]; !ok {
			return 0
		}
		GoToLuaProxy(L, idx)
		val := v.MapIndex(intKeys[idx])
		GoToLuaProxy(L, val)
		return 2
	}
	L.PushGoFunction(iter)
	return 1
}

func map__newindex(L *lua.State) int {
	v, t := valueOfProxy(L, 1)
	key := reflect.New(t.Key())
	err := LuaToGo(L, 2, key.Interface())
	if err != nil {
		L.RaiseError(fmt.Sprintf("map requires %v key", t.Key()))
	}
	key = key.Elem()
	val := reflect.New(t.Elem())
	err = LuaToGo(L, 3, val.Interface())
	if err != nil {
		L.RaiseError(fmt.Sprintf("map requires %v value type", t.Elem()))
	}
	val = val.Elem()
	v.SetMapIndex(key, val)
	return 0
}

func map__pairs(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	keys := v.MapKeys()
	idx := -1
	n := v.Len()
	iter := func(L *lua.State) int {
		idx++
		if idx == n {
			return 0
		}
		GoToLuaProxy(L, keys[idx])
		val := v.MapIndex(keys[idx])
		GoToLuaProxy(L, val)
		return 2
	}
	L.PushGoFunction(iter)
	return 1
}

func number__add(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	var result interface{}
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		result = v1.Uint() + v2.Uint()
	case reflect.Int64:
		result = v1.Int() + v2.Int()
	case reflect.Float64:
		result = valueToNumber(L, v1) + valueToNumber(L, v2)
	case reflect.Complex128:
		result = valueToComplex(L, v1) + valueToComplex(L, v2)
	}
	pushNumberValue(L, result, t1, t2)
	return 1
}

func number__div(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	var result interface{}
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		result = v1.Uint() / v2.Uint()
	case reflect.Int64:
		result = v1.Int() / v2.Int()
	case reflect.Float64:
		result = valueToNumber(L, v1) / valueToNumber(L, v2)
	case reflect.Complex128:
		result = valueToComplex(L, v1) / valueToComplex(L, v2)
	}
	pushNumberValue(L, result, t1, t2)
	return 1
}

func number__lt(L *lua.State) int {
	v1, _ := luaToGoValue(L, 1)
	v2, _ := luaToGoValue(L, 2)
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		L.PushBoolean(v1.Uint() < v2.Uint())
	case reflect.Int64:
		L.PushBoolean(v1.Int() < v2.Int())
	case reflect.Float64:
		L.PushBoolean(valueToNumber(L, v1) < valueToNumber(L, v2))
	}
	return 1
}

func number__mod(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	var result interface{}
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		result = v1.Uint() % v2.Uint()
	case reflect.Int64:
		result = v1.Int() % v2.Int()
	case reflect.Float64:
		result = math.Mod(valueToNumber(L, v1), valueToNumber(L, v2))
	}
	pushNumberValue(L, result, t1, t2)
	return 1
}

func number__mul(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	var result interface{}
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		result = v1.Uint() * v2.Uint()
	case reflect.Int64:
		result = v1.Int() * v2.Int()
	case reflect.Float64:
		result = valueToNumber(L, v1) * valueToNumber(L, v2)
	case reflect.Complex128:
		result = valueToComplex(L, v1) * valueToComplex(L, v2)
	}
	pushNumberValue(L, result, t1, t2)
	return 1
}

func number__pow(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	var result interface{}
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		result = math.Pow(float64(v1.Uint()), float64(v2.Uint()))
	case reflect.Int64:
		result = math.Pow(float64(v1.Int()), float64(v2.Int()))
	case reflect.Float64:
		result = math.Pow(valueToNumber(L, v1), valueToNumber(L, v2))
	case reflect.Complex128:
		result = cmplx.Pow(valueToComplex(L, v1), valueToComplex(L, v2))
	}
	pushNumberValue(L, result, t1, t2)
	return 1
}

func number__sub(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	var result interface{}
	switch commonKind(v1, v2) {
	case reflect.Uint64:
		result = v1.Uint() - v2.Uint()
	case reflect.Int64:
		result = v1.Int() - v2.Int()
	case reflect.Float64:
		result = valueToNumber(L, v1) - valueToNumber(L, v2)
	case reflect.Complex128:
		result = valueToComplex(L, v1) - valueToComplex(L, v2)
	}
	pushNumberValue(L, result, t1, t2)
	return 1
}

func number__unm(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	var result interface{}
	switch unsizedKind(v1) {
	case reflect.Uint64:
		result = -v1.Uint()
	case reflect.Int64:
		result = -v1.Int()
	case reflect.Float64, reflect.String:
		result = -valueToNumber(L, v1)
	case reflect.Complex128:
		result = -v1.Complex()
	}
	v := reflect.ValueOf(result)
	if unsizedKind(v1) == reflect.Complex128 {
		makeValueProxy(L, v.Convert(t1), cComplexMeta)
	} else if isNewType(t1) {
		makeValueProxy(L, v.Convert(t1), cNumberMeta)
	} else {
		L.PushNumber(v.Float())
	}
	return 1
}

func proxy__eq(L *lua.State) int {
	var a1 interface{}
	_ = LuaToGo(L, 1, &a1)
	var a2 interface{}
	_ = LuaToGo(L, 2, &a2)
	L.PushBoolean(a1 == a2)
	return 1
}

func proxy__gc(L *lua.State) int {
	proxyId := *(*uintptr)(L.ToUserdata(1))
	proxymu.Lock()
	delete(proxyMap, proxyId)
	proxymu.Unlock()
	return 0
}

func proxy__tostring(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	L.PushString(fmt.Sprintf("%v", v))
	return 1
}

func slice__index(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if L.IsNumber(2) {
		idx := L.ToInteger(2)
		if idx < 1 || idx > v.Len() {
			L.RaiseError("slice/array get: index out of range")
		}
		v := v.Index(idx - 1)
		GoToLuaProxy(L, v)

	} else if L.IsString(2) {
		name := L.ToString(2)
		if v.Kind() == reflect.Array {
			pushGoMethod(L, name, v)
			return 1
		}
		switch name {
		case "append":
			f := func(L *lua.State) int {
				narg := L.GetTop()
				args := []reflect.Value{}
				for i := 1; i <= narg; i++ {
					elem := reflect.New(v.Type().Elem())
					err := LuaToGo(L, i, elem.Interface())
					if err != nil {
						L.RaiseError(fmt.Sprintf("slice requires %v value type", v.Type().Elem()))
					}
					args = append(args, elem.Elem())
				}
				newslice := reflect.Append(v, args...)
				makeValueProxy(L, newslice, cSliceMeta)
				return 1
			}
			L.PushGoFunction(f)
		case "cap":
			L.PushInteger(int64(v.Cap()))
		case "slice":
			L.PushGoFunction(slicer(L, v, cSliceMeta))
		default:
			pushGoMethod(L, name, v)
		}
	} else {
		L.RaiseError("non-integer slice/array index")
	}
	return 1
}

func slice__ipairs(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	n := v.Len()
	idx := -1
	iter := func(L *lua.State) int {
		idx++
		if idx == n {
			return 0
		}
		GoToLuaProxy(L, idx+1)
		val := v.Index(idx)
		GoToLuaProxy(L, val)
		return 2
	}
	L.PushGoFunction(iter)
	return 1
}

func slice__newindex(L *lua.State) int {
	v, t := valueOfProxy(L, 1)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}
	idx := L.ToInteger(2)
	val := reflect.New(t.Elem())
	err := LuaToGo(L, 3, val.Interface())
	if err != nil {
		L.RaiseError(fmt.Sprintf("slice requires %v value type", t.Elem()))
	}
	val = val.Elem()
	if idx < 1 || idx > v.Len() {
		L.RaiseError("slice/array set: index out of range")
	}
	v.Index(idx - 1).Set(val)
	return 0
}

func slicemap__len(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	L.PushInteger(int64(v.Len()))
	return 1
}

func string__concat(L *lua.State) int {
	v1, t1 := luaToGoValue(L, 1)
	v2, t2 := luaToGoValue(L, 2)
	s1 := valueToString(L, v1)
	s2 := valueToString(L, v2)
	result := s1 + s2

	if t1 == t2 || isPredeclaredType(t2) {
		v := reflect.ValueOf(result)
		makeValueProxy(L, v.Convert(t1), cStringMeta)
	} else if isPredeclaredType(t1) {
		v := reflect.ValueOf(result)
		makeValueProxy(L, v.Convert(t2), cStringMeta)
	} else {
		L.PushString(result)
	}

	return 1
}

func string__index(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	if L.IsNumber(2) {
		idx := L.ToInteger(2)
		if idx < 1 || idx > v.Len() {
			L.RaiseError("index out of range")
		}
		v := v.Index(idx - 1).Convert(reflect.TypeOf(""))
		GoToLuaProxy(L, v)
	} else if L.IsString(2) {
		name := L.ToString(2)
		if name == "slice" {
			L.PushGoFunction(slicer(L, v, cStringMeta))
		} else {
			pushGoMethod(L, name, v)
		}
	} else {
		L.RaiseError("non-integer string index")
	}
	return 1
}

func string__ipairs(L *lua.State) int {
	v, _ := valueOfProxy(L, 1)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	r := []rune(v.String())
	n := len(r)
	idx := -1
	iter := func(L *lua.State) int {
		idx++
		if idx == n {
			return 0
		}
		GoToLuaProxy(L, idx+1)
		GoToLuaProxy(L, string(r[idx]))
		return 2
	}
	L.PushGoFunction(iter)
	return 1
}

func string__len(L *lua.State) int {
	v1, _ := luaToGoValue(L, 1)
	L.PushInteger(int64(v1.Len()))
	return 1
}

func string__lt(L *lua.State) int {
	v1, _ := luaToGoValue(L, 1)
	v2, _ := luaToGoValue(L, 2)
	L.PushBoolean(v1.String() < v2.String())
	return 1
}

func struct__index(L *lua.State) int {
	v, t := valueOfProxy(L, 1)
	name := L.ToString(2)
	vp := v
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	field := v.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		pushGoMethod(L, name, vp)
	} else {
		GoToLuaProxy(L, field)
	}
	return 1
}

func struct__newindex(L *lua.State) int {
	v, t := valueOfProxy(L, 1)
	name := L.ToString(2)
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		L.RaiseError(fmt.Sprintf("no field named `%s` for type %s", name, v.Type()))
	}
	val := reflect.New(field.Type())
	err := LuaToGo(L, 3, val.Interface())
	if err != nil {
		L.RaiseError(fmt.Sprintf("struct field %v requires %v value type, error with target: %v", name, field.Type(), err))
	}
	field.Set(val.Elem())
	return 0
}

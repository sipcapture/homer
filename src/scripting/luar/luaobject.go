// Copyright (c) 2010-2016 Steve Donovan
// Adapted for Homer-Core

package luar

import (
	"errors"
	"reflect"

	"github.com/sipcapture/golua/lua"
)

// LuaObject encapsulates a Lua object like a table or a function.
type LuaObject struct {
	l   *lua.State
	ref int
}

var (
	ErrLuaObjectCallResults   = errors.New("results must be a pointer to pointer/slice/struct")
	ErrLuaObjectCallable      = errors.New("LuaObject must be callable")
	ErrLuaObjectIndexable     = errors.New("not indexable")
	ErrLuaObjectUnsharedState = errors.New("LuaObjects must share the same state")
)

// NewLuaObject creates a new LuaObject from stack index.
func NewLuaObject(L *lua.State, idx int) *LuaObject {
	L.PushValue(idx)
	ref := L.Ref(lua.LUA_REGISTRYINDEX)
	return &LuaObject{l: L, ref: ref}
}

// NewLuaObjectFromName creates a new LuaObject from the object designated by the sequence of 'subfields'.
func NewLuaObjectFromName(L *lua.State, subfields ...interface{}) *LuaObject {
	L.GetGlobal("_G")
	defer L.Pop(1)
	err := get(L, subfields...)
	if err != nil {
		return nil
	}
	val := NewLuaObject(L, -1)
	L.Pop(1)
	return val
}

// NewLuaObjectFromValue creates a new LuaObject from a Go value.
func NewLuaObjectFromValue(L *lua.State, val interface{}) *LuaObject {
	GoToLua(L, val)
	return NewLuaObject(L, -1)
}

// Call calls a Lua function, given the desired results and the arguments.
func (lo *LuaObject) Call(results interface{}, args ...interface{}) error {
	L := lo.l
	lo.Push()
	if !L.IsFunction(-1) {
		if !L.GetMetaField(-1, "__call") {
			L.Pop(1)
			return ErrLuaObjectCallable
		}
		L.Remove(-2)
	}

	for _, arg := range args {
		GoToLuaProxy(L, arg)
	}

	if results == nil {
		err := L.Call(len(args), 0)
		if err != nil {
			L.Pop(1)
			return err
		}
		return nil
	}

	resptr := reflect.ValueOf(results)
	if resptr.Kind() != reflect.Ptr {
		return ErrLuaObjectCallResults
	}
	res := resptr.Elem()

	switch res.Kind() {
	case reflect.Ptr:
		err := L.Call(len(args), 1)
		defer L.Pop(1)
		if err != nil {
			return err
		}
		return LuaToGo(L, -1, res.Interface())

	case reflect.Slice:
		residx := L.GetTop() - len(args)
		err := L.Call(len(args), lua.LUA_MULTRET)
		if err != nil {
			L.Pop(1)
			return err
		}

		nresults := L.GetTop() - residx + 1
		defer L.Pop(nresults)
		t := res.Type()

		if res.IsNil() || nresults > res.Len() {
			v := reflect.MakeSlice(t, nresults, nresults)
			res.Set(v)
		} else if nresults < res.Len() {
			res.SetLen(nresults)
		}

		for i := 0; i < nresults; i++ {
			err = LuaToGo(L, residx+i, res.Index(i).Addr().Interface())
			if err != nil {
				return err
			}
		}

	case reflect.Struct:
		exportedFields := []reflect.Value{}
		for i := 0; i < res.NumField(); i++ {
			if res.Field(i).CanInterface() {
				exportedFields = append(exportedFields, res.Field(i).Addr())
			}
		}
		nresults := len(exportedFields)
		err := L.Call(len(args), nresults)
		if err != nil {
			L.Pop(1)
			return err
		}
		defer L.Pop(nresults)
		residx := L.GetTop() - nresults + 1

		for i := 0; i < nresults; i++ {
			err = LuaToGo(L, residx+i, exportedFields[i].Interface())
			if err != nil {
				return err
			}
		}

	default:
		return ErrLuaObjectCallResults
	}

	return nil
}

// Close frees the Lua reference of this object.
func (lo *LuaObject) Close() {
	lo.l.Unref(lua.LUA_REGISTRYINDEX, lo.ref)
}

func get(L *lua.State, subfields ...interface{}) error {
	L.PushValue(-1)

	for _, field := range subfields {
		if L.IsTable(-1) {
			GoToLua(L, field)
			L.GetTable(-2)
		} else if L.GetMetaField(-1, "__index") {
			L.PushValue(-2)
			GoToLua(L, field)
			err := L.Call(2, 1)
			if err != nil {
				L.Pop(1)
				return err
			}
		} else {
			return ErrLuaObjectIndexable
		}
		L.Remove(-2)
	}
	return nil
}

// Get stores in 'a' the Lua value indexed at the sequence of 'subfields'.
func (lo *LuaObject) Get(a interface{}, subfields ...interface{}) error {
	lo.Push()
	defer lo.l.Pop(1)
	err := get(lo.l, subfields...)
	if err != nil {
		return err
	}
	defer lo.l.Pop(1)
	return LuaToGo(lo.l, -1, a)
}

// GetObject returns the LuaObject indexed at the sequence of 'subfields'.
func (lo *LuaObject) GetObject(subfields ...interface{}) (*LuaObject, error) {
	lo.Push()
	defer lo.l.Pop(1)
	err := get(lo.l, subfields...)
	if err != nil {
		return nil, err
	}
	val := NewLuaObject(lo.l, -1)
	lo.l.Pop(1)
	return val, nil
}

// Push pushes this LuaObject on the stack.
func (lo *LuaObject) Push() {
	lo.l.RawGeti(lua.LUA_REGISTRYINDEX, lo.ref)
}

// Set sets the value at the sequence of 'subfields' with the value 'a'.
func (lo *LuaObject) Set(a interface{}, subfields ...interface{}) error {
	parentKeys := subfields[:len(subfields)-1]
	parent, err := lo.GetObject(parentKeys...)
	if err != nil {
		return err
	}

	L := parent.l
	parent.Push()
	defer L.Pop(1)

	lastField := subfields[len(subfields)-1]
	if L.IsTable(-1) {
		GoToLuaProxy(L, lastField)
		GoToLuaProxy(L, a)
		L.SetTable(-3)
	} else if L.GetMetaField(-1, "__newindex") {
		L.PushValue(-2)
		GoToLuaProxy(L, lastField)
		GoToLuaProxy(L, a)
		err := L.Call(3, 0)
		if err != nil {
			L.Pop(1)
			return err
		}
	} else {
		return ErrLuaObjectIndexable
	}
	return nil
}

package lrucache

import (
	. "reflect"
	"time"
)

// Memory cache item.
type MemoryItem struct {
	key        string
	val        interface{}
	LastAccess int64
	expired    time.Duration
}

func (this *MemoryItem) Size() int {
	size := (len(this.key) + 16) * 2 //Include mapindex space
	size += Sizeof(this.val)         // value size , use reflect realize
	size += 16                       // expired, sec and nsec in time.Time
	size += 32                       // Oneself pointer and three pointers in List
	return size
}

/*
函数功能：返回任意类型的占用空间大小
函数参数：i interface{} 任意类型的变量/常量值
函数返回：int 参数所占用的空间byte大小
支持类型：Bool, Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64,
		Uintptr, Float32, Float64, Complex64, Complex128, Func, UnsafePointer, String,
		Interface, Ptr,Array, Slice, Map, Chan, Struct
*/
func Sizeof(i interface{}) int {
	return sizeof(ValueOf(i), nil)
}

func sizeof(v Value, mp map[uintptr]bool) int {
	switch v.Kind() {
	case Bool, Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64,
		Uintptr, Float32, Float64, Complex64, Complex128, Func, UnsafePointer:
		return int(v.Type().Size())
	case String:
		return v.Len() + int(v.Type().Size())
	case Interface:
		return sizeof(v.Elem(), mp) + int(v.Type().Size())
	case Ptr:
		if v.IsNil() {
			return int(v.Type().Size())
		}
		if mp == nil {
			mp = make(map[uintptr]bool)
		}
		if _, ok := mp[v.Pointer()]; ok {
			return int(v.Type().Size())
		}
		mp[v.Pointer()] = true
		return sizeof(v.Elem(), mp) + int(v.Type().Size())
	case Array, Slice:
		Len := int(v.Type().Size())
		for i := 0; i < v.Len(); i++ {
			Len += sizeof(v.Index(i), mp)
		}
		if v.Cap() > v.Len() {
			Len += (v.Cap() - v.Len()) * int(v.Type().Elem().Size())
		}
		return Len
	case Map:
		Len := int(v.Type().Size())
		for _, key := range v.MapKeys() {
			Len += sizeof(key, mp) + sizeof(v.MapIndex(key), mp)
		}
		return Len
	case Chan:
		return v.Cap()*int(v.Type().Elem().Size()) + int(v.Type().Size())
	case Struct:
		Len := 0
		for i := 0; i < v.NumField(); i++ {
			Len += sizeof(v.Field(i), mp)
		}
		return Len
	case Invalid:
		return 0
	}
	panic("Unknow value's type")
}

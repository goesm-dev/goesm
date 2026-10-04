package unsupported

import "unsafe"

func Halves(x *int64) *[2]int32 {
	return (*[2]int32)(unsafe.Pointer(x))
}

func Bits(f *float64) *uint64 {
	return (*uint64)(unsafe.Pointer(f))
}

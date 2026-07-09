// Copyright 2026 The Libc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libc // import "modernc.org/libc"

import (
	"testing"
	"unsafe"
)

// The float atomic helpers round-trip a value through the out-parameter ABI
// (__atomic_store writes *val into *ptr, __atomic_load writes *ptr into *ret),
// atomically copying the bit pattern. wasm2c emits them for f32/f64 access to
// shared linear memory; they delegate to the same-width integer helpers.
func TestAtomicLoadStoreFloat(t *testing.T) {
	tls := NewTLS()

	defer tls.Close()

	t.Run("Float32", func(t *testing.T) {
		var cell, val, ret float32
		val = 3.14159

		X__atomic_storeFloat32(tls, uintptr(unsafe.Pointer(&cell)), uintptr(unsafe.Pointer(&val)), __ATOMIC_SEQ_CST)
		if cell != val {
			t.Fatalf("store: cell = %v, want %v", cell, val)
		}

		X__atomic_loadFloat32(tls, uintptr(unsafe.Pointer(&cell)), uintptr(unsafe.Pointer(&ret)), __ATOMIC_SEQ_CST)
		if ret != val {
			t.Fatalf("load: ret = %v, want %v", ret, val)
		}
	})

	t.Run("Float64", func(t *testing.T) {
		var cell, val, ret float64
		val = 2.718281828459045

		X__atomic_storeFloat64(tls, uintptr(unsafe.Pointer(&cell)), uintptr(unsafe.Pointer(&val)), __ATOMIC_SEQ_CST)
		if cell != val {
			t.Fatalf("store: cell = %v, want %v", cell, val)
		}

		X__atomic_loadFloat64(tls, uintptr(unsafe.Pointer(&cell)), uintptr(unsafe.Pointer(&ret)), __ATOMIC_SEQ_CST)
		if ret != val {
			t.Fatalf("load: ret = %v, want %v", ret, val)
		}
	})
}

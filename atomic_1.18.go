//go:build !go1.19

package daemon

import "sync/atomic"

type atomicNoCopy struct{}

func atomicB32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

type atomicBool struct {
	_ atomicNoCopy
	v uint32
}

// Load atomically loads and returns the value stored in x.
func (x *atomicBool) Load() bool { return atomic.LoadUint32(&x.v) != 0 }

// Store atomically stores val into x.
func (x *atomicBool) Store(val bool) { atomic.StoreUint32(&x.v, atomicB32(val)) }

// Swap atomically stores new into x and returns the previous value.
func (x *atomicBool) Swap(new bool) (old bool) { return atomic.SwapUint32(&x.v, atomicB32(new)) != 0 }

// CompareAndSwap executes the compare-and-swap operation for the boolean value x.
func (x *atomicBool) CompareAndSwap(old, new bool) (swapped bool) {
	return atomic.CompareAndSwapUint32(&x.v, atomicB32(old), atomicB32(new))
}

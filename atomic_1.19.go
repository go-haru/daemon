//go:build go1.19

package daemon

import "sync/atomic"

type atomicBool = atomic.Bool

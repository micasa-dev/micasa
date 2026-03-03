// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package safeconv

import (
	"fmt"
	"math"
)

// Int narrows an int64 to int, returning an error if the value overflows.
func Int(v int64) (int, error) {
	if v < math.MinInt || v > math.MaxInt {
		return 0, fmt.Errorf("int64 value %d overflows int", v)
	}
	return int(v), nil
}

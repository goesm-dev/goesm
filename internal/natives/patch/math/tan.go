//go:build goesm

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goesm's patch of math's tan.go: tan.
// The octant j is an int instead of a uint64. j is below 2^29 (the
// argument is below reduceThreshold, or trigReduce returns it mod 8), so
// the results are the same, but a uint64 is a BigInt under goesm and an int
// a JS number: the BigInt arithmetic made these functions several times
// slower than the rest of their bodies.
package math

func tan(x float64) float64 {
	const (
		PI4A = 7.85398125648498535156e-1  // 0x3fe921fb40000000, Pi/4 split into three parts
		PI4B = 3.77489470793079817668e-8  // 0x3e64442d00000000,
		PI4C = 2.69515142907905952645e-15 // 0x3ce8469898cc5170,
	)
	// special cases
	switch {
	case x == 0 || IsNaN(x):
		return x // return ±0 || NaN()
	case IsInf(x, 0):
		return NaN()
	}

	// make argument positive but save the sign
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	var j int
	var y, z float64
	if x >= reduceThreshold {
		var ju uint64
		ju, z = trigReduce(x)
		j = int(ju)
	} else {
		j = int(x * (4 / Pi)) // integer part of x/(Pi/4), as integer for tests on the phase angle
		y = float64(j)        // integer part of x/(Pi/4), as float

		/* map zeros and singularities to origin */
		if j&1 == 1 {
			j++
			y++
		}

		z = ((x - y*PI4A) - y*PI4B) - y*PI4C
	}
	zz := z * z

	if zz > 1e-14 {
		y = z + z*(zz*(((_tanP[0]*zz)+_tanP[1])*zz+_tanP[2])/((((zz+_tanQ[1])*zz+_tanQ[2])*zz+_tanQ[3])*zz+_tanQ[4]))
	} else {
		y = z
	}
	if j&2 == 2 {
		y = -1 / y
	}
	if sign {
		y = -y
	}
	return y
}

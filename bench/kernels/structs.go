package kernels

import "math"

// body is a planet of the n-body simulation.
type body struct {
	x, y, z, vx, vy, vz, mass float64
}

const (
	solarMass   = 4 * math.Pi * math.Pi
	daysPerYear = 365.24
)

func newSystem() []body {
	bodies := []body{
		{0, 0, 0, 0, 0, 0, solarMass},
		{4.84143144246472090e+00, -1.16032004402742839e+00, -1.03622044471123109e-01,
			1.66007664274403694e-03 * daysPerYear, 7.69901118419740425e-03 * daysPerYear,
			-6.90460016972063023e-05 * daysPerYear, 9.54791938424326609e-04 * solarMass},
		{8.34336671824457987e+00, 4.12479856412430479e+00, -4.03523417114321381e-01,
			-2.76742510726862411e-03 * daysPerYear, 4.99852801234917238e-03 * daysPerYear,
			2.30417297573763929e-05 * daysPerYear, 2.85885980666130812e-04 * solarMass},
		{1.28943695621391310e+01, -1.51111514016986312e+01, -2.23307578892655734e-01,
			2.96460137564761618e-03 * daysPerYear, 2.37847173959480950e-03 * daysPerYear,
			-2.96589568540237556e-05 * daysPerYear, 4.36624404335156298e-05 * solarMass},
		{1.53796971148509165e+01, -2.59193146099879641e+01, 1.79258772950371181e-01,
			2.68067772490389322e-03 * daysPerYear, 1.62824170038242295e-03 * daysPerYear,
			-9.51592254519715870e-05 * daysPerYear, 5.15138902046611451e-05 * solarMass},
	}
	var px, py, pz float64
	for _, b := range bodies {
		px += b.vx * b.mass
		py += b.vy * b.mass
		pz += b.vz * b.mass
	}
	bodies[0].vx = -px / solarMass
	bodies[0].vy = -py / solarMass
	bodies[0].vz = -pz / solarMass
	return bodies
}

func advance(bodies []body, dt float64) {
	for i := range bodies {
		bi := &bodies[i]
		for j := i + 1; j < len(bodies); j++ {
			bj := &bodies[j]
			dx, dy, dz := bi.x-bj.x, bi.y-bj.y, bi.z-bj.z
			d2 := dx*dx + dy*dy + dz*dz
			mag := dt / (d2 * math.Sqrt(d2))
			bi.vx -= dx * bj.mass * mag
			bi.vy -= dy * bj.mass * mag
			bi.vz -= dz * bj.mass * mag
			bj.vx += dx * bi.mass * mag
			bj.vy += dy * bi.mass * mag
			bj.vz += dz * bi.mass * mag
		}
	}
	for i := range bodies {
		b := &bodies[i]
		b.x += dt * b.vx
		b.y += dt * b.vy
		b.z += dt * b.vz
	}
}

func energy(bodies []body) float64 {
	e := 0.0
	for i, b := range bodies {
		e += 0.5 * b.mass * (b.vx*b.vx + b.vy*b.vy + b.vz*b.vz)
		for _, b2 := range bodies[i+1:] {
			dx, dy, dz := b.x-b2.x, b.y-b2.y, b.z-b2.z
			e -= b.mass * b2.mass / math.Sqrt(dx*dx+dy*dy+dz*dz)
		}
	}
	return e
}

// NBody simulates the Jovian planets for n steps (the Benchmarks Game
// program): float64 fields of structs reached through pointers. It returns
// the energy in units of 1e-9, rounded.
func NBody(n int) int {
	bodies := newSystem()
	for i := 0; i < n; i++ {
		advance(bodies, 0.01)
	}
	return int(math.Round(-energy(bodies) * 1e9))
}

// node is a node of a binary tree.
type node struct {
	left, right *node
}

func bottomUp(depth int) *node {
	if depth == 0 {
		return &node{}
	}
	return &node{bottomUp(depth - 1), bottomUp(depth - 1)}
}

func (n *node) check() int {
	if n.left == nil {
		return 1
	}
	return 1 + n.left.check() + n.right.check()
}

// BinaryTrees allocates and walks complete binary trees (the Benchmarks Game
// program, without the long-lived tree): allocation and garbage collection.
func BinaryTrees(maxDepth int) int {
	total := 0
	for depth := 4; depth <= maxDepth; depth += 2 {
		iterations := 1 << (maxDepth - depth + 4)
		for i := 0; i < iterations; i++ {
			total += bottomUp(depth).check()
		}
	}
	return total
}

// Shape is implemented by the types Interfaces dispatches over.
type Shape interface {
	Area() float64
	Scale(f float64) Shape
}

type rect struct{ w, h float64 }
type circle struct{ r float64 }
type triangle struct{ b, h float64 }

func (s rect) Area() float64             { return s.w * s.h }
func (s rect) Scale(f float64) Shape     { return rect{s.w * f, s.h * f} }
func (s circle) Area() float64           { return math.Pi * s.r * s.r }
func (s circle) Scale(f float64) Shape   { return circle{s.r * f} }
func (s triangle) Area() float64         { return s.b * s.h / 2 }
func (s triangle) Scale(f float64) Shape { return triangle{s.b * f, s.h * f} }

// Interfaces calls methods through an interface on n shapes of three
// types: dynamic dispatch and small values boxed in interfaces.
func Interfaces(n int) int {
	shapes := make([]Shape, 0, 1000)
	for i := 0; i < 1000; i++ {
		f := float64(i%10 + 1)
		switch i % 3 {
		case 0:
			shapes = append(shapes, rect{f, f + 1})
		case 1:
			shapes = append(shapes, circle{f})
		default:
			shapes = append(shapes, triangle{f, f * 2})
		}
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		s := shapes[i%len(shapes)]
		sum += s.Scale(1.5).Area()
	}
	return int(sum) % 1000000007
}

package basics

// ---- struct arguments passed without a copy ----

type sharedPoint struct{ X, Y int }

var sharedAt *sharedPoint

// readAfterWrite writes the point sharedAt names before it reads p.
func readAfterWrite(p sharedPoint) int {
	sharedAt.X = 100
	return p.X
}

// readAfterWriteVia writes *q before it reads p.
func readAfterWriteVia(p sharedPoint, q *sharedPoint) int {
	q.X = 100
	return p.X
}

var globalPoint sharedPoint

// readAfterGlobalWrite writes globalPoint before it reads p.
func readAfterGlobalWrite(p sharedPoint) int {
	globalPoint.X = 100
	return p.X
}

// A callee that writes memory gets a copy of an argument that the write can
// reach.
func ArgCopiedForWritingCallee() []int {
	p := sharedPoint{1, 2}
	sharedAt = &p
	r := []int{readAfterWrite(p), p.X}
	q := sharedPoint{3, 4}
	r = append(r, readAfterWriteVia(q, &q), q.X)
	globalPoint = sharedPoint{5, 6}
	return append(r, readAfterGlobalWrite(globalPoint), globalPoint.X)
}

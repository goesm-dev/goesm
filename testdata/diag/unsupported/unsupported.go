package unsupported

func Loop() int {
	i := 0
again:
	i++
	if i < 3 {
		goto again
	}
	return i
}

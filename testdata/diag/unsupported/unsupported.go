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

func seq(yield func(int) bool) {
	for i := 0; i < 3; i++ {
		if !yield(i) {
			return
		}
	}
}

func recvOne(ch chan int) int { return <-ch }

func BlockingInRangeFunc(ch chan int) int {
	n := 0
	for i := range seq {
		n += i + recvOne(ch)
	}
	return n
}

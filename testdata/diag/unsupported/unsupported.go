package unsupported

func seq(yield func(int) bool) {
	for i := 0; i < 3; i++ {
		if !yield(i) {
			return
		}
	}
}

func DeferInRangeFunc() (n int) {
	for i := range seq {
		defer func() { n += i }()
	}
	return n
}

func Local[T any]() any {
	type pair struct{ a, b T }
	return pair{}
}

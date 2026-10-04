// Package stdlibuse calls standard library packages that goesm compiles
// from their ordinary Go source (no hand-written TypeScript ports).
package stdlibuse

import (
	"errors"
	"maps"
	"math/bits"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

func RuneCount() int { return utf8.RuneCountInString("héllo, 世界") }

func DecodeLast() []int {
	r, size := utf8.DecodeLastRuneInString("世界")
	return []int{int(r), size}
}

func Valid() []bool {
	return []bool{utf8.ValidString("ok"), utf8.ValidString("\xff"), utf8.Valid([]byte("世"))}
}

func AppendRune() string {
	return string(utf8.AppendRune([]byte("x"), '界'))
}

func Upper() string {
	var out []rune
	for _, r := range "héllo, ωorld" {
		out = append(out, unicode.ToUpper(r))
	}
	return string(out)
}

func Classes() []bool {
	return []bool{unicode.IsLetter('é'), unicode.IsDigit('٣'), unicode.IsSpace('　'), unicode.Is(unicode.Han, '世')}
}

func Bits32() []int {
	return []int{bits.OnesCount32(0xF0F0), bits.LeadingZeros32(1), bits.TrailingZeros32(8), bits.Len32(1023), int(bits.Reverse8(1)), int(bits.RotateLeft32(1, -1))}
}

func StringsBasics() []string {
	return []string{
		strings.ToUpper("héllo"),
		strings.Repeat("ab", 3),
		strings.TrimSpace("  x y  "),
		strings.Join(strings.Fields(" a  b c "), ","),
		strings.ReplaceAll("a-b-c", "-", "+"),
		strings.Title("x"),
	}
}

func StringsSearch() []int {
	return []int{strings.Index("chicken", "ken"), strings.LastIndex("go gopher", "go"), strings.IndexByte("golang", 'l'), strings.Count("cheese", "e"), strings.IndexRune("chicken", 'k'), strings.Compare("a", "b")}
}

func StringsSplit() []string {
	parts := strings.Split("a,b,,c", ",")
	before, after, found := strings.Cut("key=value", "=")
	return append(parts, before, after, strconv.FormatBool(found))
}

func Builder() string {
	var b strings.Builder
	for i := 0; i < 3; i++ {
		b.WriteString("x")
		b.WriteByte('-')
		b.WriteRune('界')
	}
	fmt := b.String()
	return fmt + strconv.Itoa(b.Len())
}

func Replacer() string {
	r := strings.NewReplacer("<", "&lt;", ">", "&gt;")
	return r.Replace("<a>")
}

func StrconvInts() []string {
	n, err := strconv.Atoi("42")
	u := uint64(0xff)
	_, err2 := strconv.Atoi("x1")
	return []string{strconv.Itoa(n), fmt(err), strconv.FormatUint(u, 2), err2.Error(), strconv.Quote("hi\n\"世\""), strconv.FormatInt(255, 16)}
}

func StrconvRange() string {
	_, err := strconv.ParseInt("300", 10, 8)
	return err.Error()
}

func fmt(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

type myErr struct{ code int }

func (e *myErr) Error() string { return "my error " + strconv.Itoa(e.code) }

var errBase = errors.New("base")

func ErrorsWrap() []bool {
	wrapped := &wrapErr{msg: "outer", err: errBase}
	var me *myErr
	joined := errors.Join(errBase, &myErr{7})
	return []bool{errors.Is(wrapped, errBase), errors.Is(wrapped, errors.New("base")), errors.As(joined, &me), me != nil && me.code == 7, errors.Unwrap(wrapped) == errBase}
}

func ErrorsJoin() string {
	return errors.Join(errors.New("a"), nil, errors.New("b")).Error()
}

type wrapErr struct {
	msg string
	err error
}

func (w *wrapErr) Error() string { return w.msg + ": " + w.err.Error() }
func (w *wrapErr) Unwrap() error { return w.err }

func StringsUnicode() []any {
	return []any{strings.EqualFold("Gö", "GÖ"), strings.IndexRune("chicken", 'k'), strings.ToLower("ÀB"), strings.Map(func(r rune) rune { return r + 1 }, "abc")}
}

func Sorting() [][]string {
	words := []string{"pear", "fig", "apple"}
	sort.Strings(words)
	byLen := []string{"ccc", "a", "bb"}
	sort.Slice(byLen, func(i, j int) bool { return len(byLen[i]) < len(byLen[j]) })
	nums := []int{3, 1, 2}
	slices.Sort(nums)
	return [][]string{words, byLen, {strconv.Itoa(nums[0]), strconv.Itoa(nums[2])}}
}

type point struct{ x, y int }

// sort.Slice swaps struct elements by value, so pointers keep their slots.
func SortStructs() []int {
	ps := []point{{3, 0}, {1, 0}, {2, 0}}
	first := &ps[0]
	sort.Slice(ps, func(i, j int) bool { return ps[i].x < ps[j].x })
	return []int{ps[0].x, ps[1].x, ps[2].x, first.x}
}

func ErrorsAsInterface() []bool {
	var e interface{ Unwrap() error }
	wrapped := &wrapErr{msg: "outer", err: errBase}
	return []bool{errors.As(wrapped, &e), e == any(wrapped), errors.As(errBase, &e)}
}

func SyncPrimitives() []int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	total := 0
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			total += i
			mu.Unlock()
		}()
	}
	wg.Wait()
	var once sync.Once
	calls := 0
	for range 3 {
		once.Do(func() { calls++ })
	}
	var m sync.Map
	m.Store("a", 1)
	v, _ := m.Load("a")
	return []int{total, calls, v.(int)}
}

// A goroutine holding the mutex across a blocking receive makes the other
// one wait.
func MutexContention() []string {
	var mu sync.Mutex
	var log []string
	ch := make(chan int)
	done := make(chan bool)
	mu.Lock()
	go func() {
		mu.Lock()
		log = append(log, "second")
		mu.Unlock()
		done <- true
	}()
	go func() { ch <- 1 }()
	<-ch
	log = append(log, "first")
	mu.Unlock()
	<-done
	return log
}

func SlicesAndMaps() []any {
	s := []int{1, 2, 3, 4, 5}
	s = slices.Insert(s, 1, 9, 8)
	s = slices.Delete(s, 3, 4)
	idx, found := slices.BinarySearch([]int{1, 3, 5}, 3)
	m := map[string]int{"a": 1, "b": 2}
	c := maps.Clone(m)
	c["a"] = 10
	keys := slices.Sorted(maps.Keys(c))
	return []any{s, slices.Contains(s, 9), slices.Index(s, 4), idx, found, m["a"], c["a"], keys, slices.Max(s)}
}

// SortKinds sorts slices of each ordered kind (integers and strings take the
// engine's sort, floats Go's pdqsort), also a slice into the middle of an
// array.
func SortKinds() []any {
	ints := []int{5, -3, 9007199254740991, 0, -9007199254740991, 5, 2}
	slices.Sort(ints)
	strs := []string{"b", "a\xff", "a", "", "é", "日本", "aa", "A", "a\x00"}
	sort.Strings(strs)
	bs := []byte("hello, world")
	slices.Sort(bs)
	i64 := []int64{1 << 62, -1 << 63, 0, -1, 1<<63 - 1}
	slices.Sort(i64)
	u64 := []uint64{1<<64 - 1, 0, 1 << 63, 42}
	slices.Sort(u64)
	i8 := []int8{127, -128, 0, -1}
	slices.Sort(i8)
	arr := [6]uint16{9, 8, 7, 6, 5, 4}
	slices.Sort(arr[1:5])
	type myInt int
	mine := []myInt{3, 1, 2}
	slices.Sort(mine)
	fs := []float64{3, -1, 0.5, -2.5}
	sort.Float64s(fs)
	var empty []string
	slices.Sort(empty)
	return []any{ints, strs, string(bs), i64, u64, i8, arr, mine, fs, empty == nil, slices.IsSorted(strs)}
}

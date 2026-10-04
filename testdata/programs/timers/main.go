// Command timers uses time.Sleep, timers, tickers, AfterFunc and timeouts
// in select; the output does not depend on exact timing.
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	start := time.Now()
	time.Sleep(20 * time.Millisecond)
	el := time.Since(start)
	fmt.Println("slept at least 20ms:", el >= 20*time.Millisecond, el < 2*time.Second)

	t := time.NewTimer(10 * time.Millisecond)
	v := <-t.C
	fmt.Println("timer fired:", !v.Before(start), t.Stop())

	t = time.NewTimer(time.Hour)
	fmt.Println("stop pending:", t.Stop(), t.Stop())
	fmt.Println("reset stopped:", t.Reset(5*time.Millisecond))
	<-t.C
	fmt.Println("reset fired")

	tk := time.NewTicker(5 * time.Millisecond)
	n := 0
	for range tk.C {
		n++
		if n == 3 {
			break
		}
	}
	tk.Stop()
	fmt.Println("ticks:", n)
	tk.Reset(2 * time.Millisecond)
	<-tk.C
	tk.Stop()
	fmt.Println("ticker reset")

	var wg sync.WaitGroup
	wg.Add(1)
	done := false
	time.AfterFunc(5*time.Millisecond, func() {
		done = true
		wg.Done()
	})
	wg.Wait()
	fmt.Println("AfterFunc ran:", done)
	af := time.AfterFunc(time.Hour, func() { fmt.Println("never") })
	fmt.Println("AfterFunc stopped:", af.Stop())

	ch := make(chan int)
	go func() {
		time.Sleep(30 * time.Millisecond)
		ch <- 1
	}()
	select {
	case <-ch:
		fmt.Println("unexpected")
	case <-time.After(5 * time.Millisecond):
		fmt.Println("timeout")
	}
	fmt.Println("received:", <-ch)

	results := make(chan string, 3)
	for i, d := range []int{60, 10, 35} {
		go func() {
			time.Sleep(time.Duration(d) * time.Millisecond)
			results <- fmt.Sprint("worker ", i)
		}()
	}
	for range 3 {
		fmt.Println(<-results)
	}

	deadline := time.Now().Add(15 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	fmt.Println("deadline passed:", time.Until(deadline) <= 0)
	d, _ := time.ParseDuration("1h2m3.5s")
	fmt.Println(d, d.Seconds(), time.Duration(1500)*time.Microsecond)
	tm := time.Date(2026, time.October, 4, 12, 30, 0, 0, time.UTC)
	fmt.Println(tm, tm.Add(36*time.Hour).Weekday(), tm.Format(time.RFC3339), tm.Unix(), tm.YearDay())
	p, err := time.Parse("2006-01-02 15:04", "2026-02-28 23:59")
	fmt.Println(p, err, p.Add(time.Minute).Month())
}

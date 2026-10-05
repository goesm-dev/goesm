package kernels

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
	"sync"
)

// The kernels of this file are ways of writing Go that a compiler to JS can
// turn into performance cliffs: work native Go spreads over cores, 64-bit
// arithmetic outside locals, calls that may block, and math/big.

// Parallel splits CPU work over four goroutines and waits for them. Native
// Go runs them on separate cores; JS runs them one after another.
func Parallel(n int) int {
	const workers = 4
	var wg sync.WaitGroup
	sums := make([]int, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s := 0
			for i := w; i < n; i += workers {
				s += collatz(i + 1)
			}
			sums[w] = s
		}(w)
	}
	wg.Wait()
	total := 0
	for _, s := range sums {
		total += s
	}
	return total
}

// collatz returns the number of steps of the Collatz sequence from x to 1.
func collatz(x int) int {
	steps := 0
	for x != 1 {
		if x%2 == 0 {
			x /= 2
		} else {
			x = 3*x + 1
		}
		steps++
	}
	return steps
}

// xorshift is a xorshift64 generator whose state lives in a struct field.
type xorshift struct{ s uint64 }

func (x *xorshift) next() uint64 {
	x.s ^= x.s << 13
	x.s ^= x.s >> 7
	x.s ^= x.s << 17
	return x.s
}

// Rand64 draws n numbers from a xorshift64 generator: uint64 arithmetic on
// a struct field, through a method.
func Rand64(n int) int {
	r := &xorshift{88172645463325252}
	var acc uint64
	for i := 0; i < n; i++ {
		acc += r.next() >> 40
	}
	return int(acc % 1000000007)
}

// Sink receives values. chanSink's Put blocks on a channel, as an io.Writer
// that is a pipe does.
type Sink interface{ Put(v int) }

type sumSink struct{ sum int }

func (s *sumSink) Put(v int) { s.sum += v }

type chanSink struct{ ch chan int }

func (s chanSink) Put(v int) { s.ch <- v }

func fill(s Sink, n int) {
	for i := 0; i < n; i++ {
		s.Put(i & 0xff)
	}
}

// MaybeBlocking calls a method through an interface n times. The
// implementation called never blocks, but another one of the interface
// does.
func MaybeBlocking(n int) int {
	s := &sumSink{}
	fill(s, n)
	if n < 0 { // never: chanSink is only there to be one of the Sinks
		fill(chanSink{make(chan int)}, 1)
	}
	return s.sum
}

var rsaKey = func() *rsa.PrivateKey {
	p, _ := new(big.Int).SetString("e274fb05232d7d9093cf0fcd4e8802c8cfcecf87ae77219588c0e67744e927faf4bb03cf8ad2a04cb37a2f32148351b974494443c4bc1e3134d6706e38c84468809c6966d86a695fc010b0da370403c3e68115f013a5767ffe20744d0809c2f6d0c2c0c4b361a27e0af6906bfc02fb8399e444015f357124610a9bc556dcca7f", 16)
	q, _ := new(big.Int).SetString("f6d4b33255580015e1854bed878d2d93e05c85ae68feeaccbd84b0c5a91753ee4b43f0e13be47efb578df91ff2c86273169b66c01fdbc967fdf9ef15c70ec00b50df5bc1f8569a0ec248cc4c42ce4c9c56292329e0748489b02a919e0f48c5c16724c984b9425dae039fbe757e75b222c1bac332797ae63d33e32523d513274f", 16)
	k := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: new(big.Int).Mul(p, q), E: 65537}, Primes: []*big.Int{p, q}}
	phi := new(big.Int).Mul(new(big.Int).Sub(p, big.NewInt(1)), new(big.Int).Sub(q, big.NewInt(1)))
	k.D = new(big.Int).ModInverse(big.NewInt(65537), phi)
	k.Precompute()
	return k
}()

// RSASign signs n SHA-256 digests with a 2048-bit RSA key (PKCS #1 v1.5):
// modular exponentiation of 1024-bit numbers.
func RSASign(n int) int {
	acc := 0
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte{byte(i), byte(i >> 8)})
		sig, err := rsa.SignPKCS1v15(nil, rsaKey, crypto.SHA256, h[:])
		if err != nil {
			panic(err)
		}
		acc = (acc*31 + int(sig[0]) + int(sig[len(sig)-1])) % 1000000007
	}
	return acc
}

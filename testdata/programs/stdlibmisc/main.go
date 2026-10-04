// Standard library packages whose gc implementations reach into the
// runtime or use unsafe: crypto hashes and ciphers, crypto/rand, math/rand,
// sync/atomic.Value and slice to array pointer conversions.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash/maphash"
	mrand "math/rand"
	mrand2 "math/rand/v2"
	"slices"
	"sync/atomic"
)

func hashes() {
	msg := []byte("The quick brown fox jumps over the lazy dog")
	fmt.Printf("md5    %x\n", md5.Sum(msg))
	fmt.Printf("sha1   %x\n", sha1.Sum(msg))
	fmt.Printf("sha256 %x\n", sha256.Sum256(msg))
	fmt.Printf("sha512 %x\n", sha512.Sum512(msg))
	fmt.Printf("sha3   %x\n", sha3.Sum256(msg))
	fmt.Printf("shake  %x\n", sha3.SumSHAKE128(msg, 16))
	m := hmac.New(sha256.New, []byte("key"))
	m.Write(msg)
	fmt.Printf("hmac   %x\n", m.Sum(nil))
}

func ciphers() {
	key, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	sealed := gcm.Seal(nil, nonce, []byte("attack at dawn"), []byte("ad"))
	fmt.Printf("gcm    %x\n", sealed)
	opened, err := gcm.Open(nil, nonce, sealed, []byte("ad"))
	fmt.Printf("open   %q %v\n", opened, err)
	sealed[0] ^= 1
	_, err = gcm.Open(nil, nonce, sealed, []byte("ad"))
	fmt.Println("tampered:", err)

	ctr := cipher.NewCTR(block, make([]byte, 16))
	out := make([]byte, 20)
	ctr.XORKeyStream(out, []byte("counter mode stream!"))
	fmt.Printf("ctr    %x\n", out)
}

func random() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	fmt.Println("crypto/rand:", len(b), !slices.Equal(b, make([]byte, 32)))
	fmt.Println("rand.Text:", len(rand.Text()))
	n := mrand.Intn(10)
	fmt.Println("math/rand:", n >= 0 && n < 10)
	r := mrand.New(mrand.NewSource(1))
	fmt.Println("seeded:", r.Intn(1000), r.Intn(1000), r.Intn(1000))
	m := mrand2.IntN(10)
	fmt.Println("math/rand/v2:", m >= 0 && m < 10)
	p := mrand2.New(mrand2.NewPCG(1, 2))
	fmt.Println("pcg:", p.IntN(1000), p.IntN(1000), p.IntN(1000))
	var h maphash.Hash
	h.WriteString("x")
	s1 := h.Sum64()
	h.Reset()
	h.WriteString("x")
	fmt.Println("maphash:", s1 == h.Sum64())
	seed := maphash.MakeSeed()
	fmt.Println("bytes==string:", maphash.Bytes(seed, []byte("hello")) == maphash.String(seed, "hello"),
		maphash.String(seed, "hello") != maphash.String(seed, "hellp"))
	type pt struct {
		X, Y int
		S    string
	}
	fmt.Println("comparable:", maphash.Comparable(seed, pt{1, 2, "a"}) == maphash.Comparable(seed, pt{1, 2, "a"}),
		maphash.Comparable(seed, pt{1, 2, "a"}) != maphash.Comparable(seed, pt{1, 2, "b"}),
		maphash.Comparable[any](seed, 1) == maphash.Comparable[any](seed, 1))
}

func atomicValue() {
	var v atomic.Value
	fmt.Println("empty:", v.Load())
	v.Store("a")
	fmt.Println("swap:", v.Swap("b"), v.Load())
	fmt.Println("cas:", v.CompareAndSwap("x", "c"), v.CompareAndSwap("b", "c"), v.Load())
	defer func() { fmt.Println("recovered:", recover()) }()
	v.Store(1)
}

func arrayPointers() {
	s := []int{1, 2, 3, 4, 5}
	p := (*[3]int)(s[1:])
	p[0] = 20
	fmt.Println(s, *p, len(p))
	q := (*[5]int)(s)
	q[4] = 50
	fmt.Println(s, q == (*[5]int)(s), (*[3]int)(s[1:]) == p)
	for i, v := range p {
		fmt.Print(i, ":", v, " ")
	}
	fmt.Println(p[1:])
	var e []int
	fmt.Println((*[0]int)(e) == nil)
	defer func() { fmt.Println("recovered:", recover()) }()
	_ = (*[6]int)(s)
}

func main() {
	hashes()
	ciphers()
	random()
	atomicValue()
	arrayPointers()
}

// Public-key cryptography, which rests on math/big and
// crypto/internal/fips140/bigmod (32-bit words under goesm) and on the P-256
// precomputed table. Only deterministic results are printed.
package main

import (
	"bytes"
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

func hexInt(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic(s)
	}
	return n
}

func short(b []byte) string { return hex.EncodeToString(b[:8]) + "…" + hex.EncodeToString(b[len(b)-8:]) }

func main() {
	// RSA with a fixed 1024-bit key.
	p := hexInt("dd8799eeff659d612bb484d72b46c9cd88f6343cdded6948fa36f5bdcf48fe7ad0d7489e3d5eeeced493c7bcc7f79d9f608a89b67d830342fa1973a5235b4ff7")
	q := hexInt("fce9bbe1c7ee83835db76a7b814651a91ff999bacf5db59361ced1d2d07a675d1275359beb234daf9a9dc4f3d06172eb8161ea30ba38c3609857f178ccd0b1df")
	n := new(big.Int).Mul(p, q)
	phi := new(big.Int).Mul(new(big.Int).Sub(p, big.NewInt(1)), new(big.Int).Sub(q, big.NewInt(1)))
	d := new(big.Int).ModInverse(big.NewInt(65537), phi)
	rk := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: n, E: 65537}, D: d, Primes: []*big.Int{p, q}}
	rk.Precompute()
	fmt.Println("rsa validate:", rk.Validate(), n.BitLen())
	h := sha256.Sum256([]byte("goesm"))
	sig, err := rsa.SignPKCS1v15(nil, rk, crypto.SHA256, h[:])
	fmt.Println("pkcs1v15:", err, short(sig))
	fmt.Println("verify:", rsa.VerifyPKCS1v15(&rk.PublicKey, crypto.SHA256, h[:], sig))
	h2 := sha256.Sum256([]byte("goesn"))
	fmt.Println("verify other:", rsa.VerifyPKCS1v15(&rk.PublicKey, crypto.SHA256, h2[:], sig) != nil)
	pss, err := rsa.SignPSS(rand.Reader, rk, crypto.SHA256, h[:], nil)
	fmt.Println("pss:", err, rsa.VerifyPSS(&rk.PublicKey, crypto.SHA256, h[:], pss, nil))
	ct, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &rk.PublicKey, []byte("secret"), nil)
	pt, err2 := rsa.DecryptOAEP(sha256.New(), nil, rk, ct, nil)
	fmt.Println("oaep:", err, err2, string(pt))
	der := x509.MarshalPKCS1PrivateKey(rk)
	rk2, err := x509.ParsePKCS1PrivateKey(der)
	fmt.Println("pkcs1 round trip:", err, rk2.Equal(rk))

	// ECDSA and ECDH on P-256 with fixed scalars.
	ek, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), bytes.Repeat([]byte{7}, 32))
	fmt.Println("p256 key:", err)
	pub, _ := ek.PublicKey.Bytes()
	fmt.Println("p256 pub:", short(pub))
	esig, err := ecdsa.SignASN1(rand.Reader, ek, h[:])
	fmt.Println("ecdsa:", err, ecdsa.VerifyASN1(&ek.PublicKey, h[:], esig), ecdsa.VerifyASN1(&ek.PublicKey, h2[:], esig))
	a, _ := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{1}, 32))
	b, _ := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{2}, 32))
	s1, _ := a.ECDH(b.PublicKey())
	s2, _ := b.ECDH(a.PublicKey())
	fmt.Println("p256 ecdh:", bytes.Equal(s1, s2), short(s1))
	xa, _ := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{3}, 32))
	xb, _ := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{4}, 32))
	x1, _ := xa.ECDH(xb.PublicKey())
	x2, _ := xb.ECDH(xa.PublicKey())
	fmt.Println("x25519:", bytes.Equal(x1, x2), short(x1))
	for _, c := range []elliptic.Curve{elliptic.P224(), elliptic.P384()} {
		k, err := ecdsa.GenerateKey(c, rand.Reader)
		s, _ := ecdsa.SignASN1(rand.Reader, k, h[:])
		fmt.Println(c.Params().Name, err, ecdsa.VerifyASN1(&k.PublicKey, h[:], s))
	}

	// Ed25519 and a self-signed certificate (both deterministic).
	edk := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, 32))
	fmt.Println("ed25519:", short(ed25519.Sign(edk, []byte("goesm"))))
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "goesm test", Organization: []string{"goesm"}},
		NotBefore:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		DNSNames:              []string{"example.com"},
	}
	cder, err := x509.CreateCertificate(nil, tmpl, tmpl, edk.Public(), edk)
	fmt.Println("certificate:", err, len(cder), short(cder))
	cert, err := x509.ParseCertificate(cder)
	fmt.Println(err, cert.Subject, cert.SerialNumber, cert.DNSNames, cert.NotAfter)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	_, err = cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.com", CurrentTime: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)})
	fmt.Println("verify:", err)
	_, err = cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.org", CurrentTime: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)})
	fmt.Println("verify other name:", err)
	blk, _ := pem.Decode(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cder}))
	fmt.Println(blk.Type, bytes.Equal(blk.Bytes, cder))
}

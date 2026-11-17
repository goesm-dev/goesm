// Command rsaverify verifies and encrypts with RSA public keys of several
// sizes and exponents. goesm computes a public key's Montgomery constant
// and the public exponentiation of a signature being verified on BigInts
// (bigmod's NewPublicModulus and ExpPublicVarTime, and the patches of
// crypto/rsa and crypto/internal/fips140/rsa that use them), so this checks
// that verification accepts and rejects what Go's does, with the same
// errors, and that encryption, which uses the constant, gives the same
// ciphertexts. The keys are fixed, so that the output does not depend on the
// random source.
package main

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// keys are the exponents and primes of RSA keys of 1024, 1030, 2048 and
// 1536 bits, generated natively from fixed seeds: finding primes takes
// longer than the test allows under goesm.
var keys = []struct {
	e    int
	p, q string
}{
	{65537, "fb28e799a4884260052e5a2c981e77f437c8ffc3efe51d7e73bff0582df2e5d6efeab2a76dff10ecc0ce015c48590a9c8a4b1fb20e37fa07158d9e143d85f249", "c33703c33d4fe2f5722ccfedc9299c4a56852692be1b6794637621c75eaf91fc6a04aad7796397acb1abc5ec1a6abf985978ca0a7bc2914c793b150ee0475a91"},
	{3, "750f6858144e65366fd69b955eef833cb2d09cdc1b0fe25b801475394ef5d9827633a0a436bd98cd32afa65f8b5b67e1172e5b002c420638fdd4f28fd32812e1b", "669a859d958d3a59d9d49caa61341df977046a5d099e1cf8f55a8ca18ff0bff8b0800e3668c19325b0b356cf8e4553883dad5f6f956dabd386fc7281b2a37cd55"},
	{65537, "dad1259e44f5526a18f088e6296a1394c6820d0665fbb8b25fd5019cb86330bfc61ddb4b8c99c6ed0aaca77623479154331488039e2793c481e1523020950ba4dad1259e44f5526a18f088e6296a1394c6820d0665fbb8b25fd5019cb86330bfc61ddb4b8c99c6ed0aaca77623479154331488039e2793c481e1523020950c85", "e5c27d0f8813ef46d70c87c04e487bf0c514817cdd5aa4f1498c272cc42256878ef5fa26859b805f9c5f232958bcaf99a9884b29aba6823ef9e5056e36509a76e5c27d0f8813ef46d70c87c04e487bf0c514817cdd5aa4f1498c272cc42256878ef5fa26859b805f9c5f232958bcaf99a9884b29aba6823ef9e5056e36509deb"},
	{2147483647, "d9cd75a0a8efe516557884f1b3eb35d0af68df666140443f25fe8dc0a58c29877e2a7413fd02d0418b77542faa1ce166b6de3691a22377ec5094f40f51a5ddb4d9cd75a0a8efe516557884f1b3eb35d0af68df666140443f25fe8dc0a58c2a11", "fbe94c431b7e5827feac640deb1a8cbf4e5e3c84f813aa3627b6be2c6294c79cbdccdef17be87f7182d951c7626b4976dc70df66bfe0e1df9947a59d9e4bb210bbe94c431b7e5827feac640deb1a8cbf4e5e3c84f813aa3627b6be2c6294c861"},
}

// key returns the RSA key of exponent e and primes p and q.
func key(e int, p, q string) *rsa.PrivateKey {
	P, _ := new(big.Int).SetString(p, 16)
	Q, _ := new(big.Int).SetString(q, 16)
	one := big.NewInt(1)
	phi := new(big.Int).Mul(new(big.Int).Sub(P, one), new(big.Int).Sub(Q, one))
	d := new(big.Int).ModInverse(big.NewInt(int64(e)), phi)
	k := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: new(big.Int).Mul(P, Q), E: e}, D: d, Primes: []*big.Int{P, Q}}
	k.Precompute()
	return k
}

// zeros is a random source of zero bytes, for deterministic padding.
type zeros struct{}

func (zeros) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

func main() {
	for _, c := range keys {
		k := key(c.e, c.p, c.q)
		pub := &k.PublicKey
		fmt.Println("key", pub.N.BitLen(), pub.E, k.Validate())
		h := sha256.Sum256([]byte(fmt.Sprint("message", pub.N.BitLen())))
		sig, err := rsa.SignPKCS1v15(nil, k, crypto.SHA256, h[:])
		fmt.Println(" sign:", errText(err), hex.EncodeToString(sig[:8]))
		bad := append([]byte(nil), sig...)
		bad[len(bad)/2] ^= 0x10
		big1 := bytes.Repeat([]byte{0xff}, len(sig))
		nBytes := pub.N.FillBytes(make([]byte, len(sig)))
		zero := make([]byte, len(sig))
		for _, s := range []struct {
			name string
			sig  []byte
		}{{"good", sig}, {"tampered", bad}, {"short", sig[1:]}, {"long", append([]byte{0}, sig...)}, {"0xff..", big1}, {"N", nBytes}, {"zero", zero}} {
			fmt.Println(" verify", s.name+":", errText(rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], s.sig)))
		}
		h2 := sha256.Sum256([]byte("other"))
		fmt.Println(" verify other hash:", errText(rsa.VerifyPKCS1v15(pub, crypto.SHA256, h2[:], sig)))
		fmt.Println(" verify unhashed:", errText(rsa.VerifyPKCS1v15(pub, 0, h[:], sig)))
		pss, err := rsa.SignPSS(zeros{}, k, crypto.SHA256, h[:], nil)
		fmt.Println(" pss:", errText(err), hex.EncodeToString(pss[:8]),
			errText(rsa.VerifyPSS(pub, crypto.SHA256, h[:], pss, nil)),
			errText(rsa.VerifyPSS(pub, crypto.SHA256, h2[:], pss, nil)),
			errText(rsa.VerifyPSS(pub, crypto.SHA256, h[:], bad, nil)),
			errText(rsa.VerifyPSS(pub, crypto.SHA256, h[:], big1, nil)))
		ct, err := rsa.EncryptOAEP(sha256.New(), zeros{}, pub, []byte("secret"), nil)
		pt, err2 := rsa.DecryptOAEP(sha256.New(), nil, k, ct, nil)
		fmt.Println(" oaep:", errText(err), hex.EncodeToString(ct[:8]), errText(err2), string(pt))
		ct, err = rsa.EncryptPKCS1v15(zeros{}, pub, []byte("secret"))
		fmt.Println(" pkcs1v15 encrypt:", errText(err), len(ct))
	}
	// Bad keys fail as in Go.
	k := key(keys[0].e, keys[0].p, keys[0].q)
	even := &rsa.PublicKey{N: new(big.Int).Add(k.N, big.NewInt(1)), E: 65537}
	one := &rsa.PublicKey{N: big.NewInt(1), E: 65537}
	evenE := &rsa.PublicKey{N: k.N, E: 65536}
	sig := make([]byte, 128)
	sig[127] = 2
	h := sha256.Sum256(nil)
	for _, pub := range []*rsa.PublicKey{even, one, evenE, {N: k.N, E: 1}} {
		_, encErr := rsa.EncryptPKCS1v15(zeros{}, pub, []byte("x"))
		fmt.Println("bad key:", errText(rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig)), "|", errText(encErr))
	}
}

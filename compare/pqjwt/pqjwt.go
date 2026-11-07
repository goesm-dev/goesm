// Package pqjwt issues and verifies JWTs signed with ML-DSA-65
// (draft-ietf-cose-dilithium: "alg":"ML-DSA-65", the raw 3309-byte
// signature in base64url), as a backend does that accepts post-quantum
// tokens from an identity provider. It uses crypto/mldsa, encoding/json
// and encoding/base64. js/impl/pqjwt.mjs does the same with
// @noble/post-quantum.
package pqjwt

import (
	"crypto/mldsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

type claims struct {
	Sub string `json:"sub"`
	Iss string `json:"iss"`
	Exp int64  `json:"exp"`
}

var b64 = base64.RawURLEncoding

var keys = map[string]*mldsa.PrivateKey{}

func key(seedHex string) (*mldsa.PrivateKey, error) {
	if k, ok := keys[seedHex]; ok {
		return k, nil
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, err
	}
	k, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
	if err != nil {
		return nil, err
	}
	keys[seedHex] = k
	return k, nil
}

// PublicKey returns the base64url public key of the key made from seedHex.
func PublicKey(seedHex string) (string, error) {
	k, err := key(seedHex)
	if err != nil {
		return "", err
	}
	return b64.EncodeToString(k.PublicKey().Bytes()), nil
}

// Issue returns a token for sub that expires at exp, signed
// deterministically with the key made from seedHex.
func Issue(seedHex, kid, sub string, exp int64) (string, error) {
	k, err := key(seedHex)
	if err != nil {
		return "", err
	}
	h, err := json.Marshal(header{"ML-DSA-65", "JWT", kid})
	if err != nil {
		return "", err
	}
	c, err := json.Marshal(claims{sub, "https://id.example.com", exp})
	if err != nil {
		return "", err
	}
	input := b64.EncodeToString(h) + "." + b64.EncodeToString(c)
	sig, err := k.SignDeterministic([]byte(input), nil)
	if err != nil {
		return "", err
	}
	return input + "." + b64.EncodeToString(sig), nil
}

var pubs = map[string]*mldsa.PublicKey{}

// Verify checks token against the base64url public key pub at the time now
// and returns its subject.
func Verify(token, pub string, now int64) (string, error) {
	pk, ok := pubs[pub]
	if !ok {
		raw, err := b64.DecodeString(pub)
		if err != nil {
			return "", err
		}
		if pk, err = mldsa.NewPublicKey(mldsa.MLDSA65(), raw); err != nil {
			return "", err
		}
		pubs[pub] = pk
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("pqjwt: not a JWS")
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	var h header
	if err := json.Unmarshal(hb, &h); err != nil {
		return "", err
	}
	if h.Alg != "ML-DSA-65" {
		return "", errors.New("pqjwt: unexpected alg " + h.Alg)
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	if err := mldsa.Verify(pk, []byte(parts[0]+"."+parts[1]), sig, nil); err != nil {
		return "", err
	}
	cb, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var c claims
	if err := json.Unmarshal(cb, &c); err != nil {
		return "", err
	}
	if c.Exp <= now {
		return "", errors.New("pqjwt: token expired")
	}
	return c.Sub, nil
}

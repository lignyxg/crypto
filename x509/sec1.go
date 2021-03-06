// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/asn1"
	"errors"
	"fmt"
	"github.com/lignyxg/crypto/sm2"
	"math/big"
)

const ecPrivKeyVersion = 1

// ecPrivateKey reflects an ASN.1 Elliptic Curve Private Key Structure.
// References:
//   RFC 5915
//   SEC1 - http://www.secg.org/sec1-v2.pdf
// Per RFC 5915 the NamedCurveOID is marked as ASN.1 OPTIONAL, however in
// most cases it is not.
type ecPrivateKey struct {
	Version       int
	PrivateKey    []byte
	NamedCurveOID asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	PublicKey     asn1.BitString        `asn1:"optional,explicit,tag:1"`
}

// ParseECPrivateKey parses an EC private key in SEC 1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "EC PRIVATE KEY".
func ParseECPrivateKey(der []byte) (interface{}/**ecdsa.PrivateKey*/, error) {
	return parseECPrivateKey(nil, der)
}

// MarshalECPrivateKey converts an EC private key to SEC 1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "EC PRIVATE KEY".
// For a more flexible key format which is not EC specific, use
// MarshalPKCS8PrivateKey.
func MarshalECPrivateKey(key interface{}/**ecdsa.PrivateKey*/) ([]byte, error) {
	//oid, ok := oidFromNamedCurve(key.Curve)
	//if !ok {
	//	return nil, errors.New("x509: unknown elliptic curve")
	//}

	return marshalECPrivateKeyWithOID(key, nil)
}

// marshalECPrivateKey marshals an EC private key into ASN.1, DER format and
// sets the curve ID to the given OID, or omits it if OID is nil.
func marshalECPrivateKeyWithOID(key interface{}/**ecdsa.PrivateKey*/, oid asn1.ObjectIdentifier) ([]byte, error) {
	var curve elliptic.Curve
	var x, y *big.Int
	var privateKeyBytes []byte
	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		privateKeyBytes = key.D.Bytes()
		curve = key.Curve
		x, y = key.X, key.Y
	case *sm2.PrivateKey:
		privateKeyBytes = key.D.Bytes()
		curve = key.Curve
		x, y = key.X, key.Y
	}
	oid, ok := oidFromNamedCurve(curve)
	if !ok {
		return nil, errors.New("x509: unknown elliptic curve")
	}
	//privateKeyBytes := key.D.Bytes()
	paddedPrivateKey := make([]byte, (curve.Params().N.BitLen()+7)/8)
	copy(paddedPrivateKey[len(paddedPrivateKey)-len(privateKeyBytes):], privateKeyBytes)

	return asn1.Marshal(ecPrivateKey{
		Version:       1,
		PrivateKey:    paddedPrivateKey,
		NamedCurveOID: oid,
		PublicKey:     asn1.BitString{Bytes: elliptic.Marshal(curve, x, y)},
	})
}

// parseECPrivateKey parses an ASN.1 Elliptic Curve Private Key Structure.
// The OID for the named curve may be provided from another source (such as
// the PKCS8 container) - if it is provided then use this instead of the OID
// that may exist in the EC private key structure.
func parseECPrivateKey(namedCurveOID *asn1.ObjectIdentifier, der []byte) (key interface{}/**ecdsa.PrivateKey*/, err error) {
	var privKey ecPrivateKey
	if _, err := asn1.Unmarshal(der, &privKey); err != nil {
		if _, err := asn1.Unmarshal(der, &pkcs8{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParsePKCS8PrivateKey instead for this key format)")
		}
		if _, err := asn1.Unmarshal(der, &pkcs1PrivateKey{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParsePKCS1PrivateKey instead for this key format)")
		}
		return nil, errors.New("x509: failed to parse EC private key: " + err.Error())
	}
	if privKey.Version != ecPrivKeyVersion {
		return nil, fmt.Errorf("x509: unknown EC private key version %d", privKey.Version)
	}

	var curve elliptic.Curve
	if namedCurveOID != nil {
		curve = namedCurveFromOID(*namedCurveOID)
	} else {
		curve = namedCurveFromOID(privKey.NamedCurveOID)
	}
	if curve == nil {
		return nil, errors.New("x509: unknown elliptic curve")
	}

	k := new(big.Int).SetBytes(privKey.PrivateKey)
	curveOrder := curve.Params().N
	if k.Cmp(curveOrder) >= 0 {
		return nil, errors.New("x509: invalid elliptic curve private key value")
	}

	switch curve {
	case sm2.P256Sm2():
		priv := new(sm2.PrivateKey)
		priv.Curve = curve
		priv.D = k

		privateKey := make([]byte, (curveOrder.BitLen()+7)/8)

		// Some private keys have leading zero padding. This is invalid
		// according to [SEC1], but this code will ignore it.
		for len(privKey.PrivateKey) > len(privateKey) {
			if privKey.PrivateKey[0] != 0 {
				return nil, errors.New("x509: invalid private key length")
			}
			privKey.PrivateKey = privKey.PrivateKey[1:]
		}

		// Some private keys remove all leading zeros, this is also invalid
		// according to [SEC1] but since OpenSSL used to do this, we ignore
		// this too.
		copy(privateKey[len(privateKey)-len(privKey.PrivateKey):], privKey.PrivateKey)
		priv.X, priv.Y = curve.ScalarBaseMult(privateKey)

		return priv, nil
	case elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521():
		priv := new(ecdsa.PrivateKey)
		priv.Curve = curve
		priv.D = k

		privateKey := make([]byte, (curveOrder.BitLen()+7)/8)

		// Some private keys have leading zero padding. This is invalid
		// according to [SEC1], but this code will ignore it.
		for len(privKey.PrivateKey) > len(privateKey) {
			if privKey.PrivateKey[0] != 0 {
				return nil, errors.New("x509: invalid private key length")
			}
			privKey.PrivateKey = privKey.PrivateKey[1:]
		}

		// Some private keys remove all leading zeros, this is also invalid
		// according to [SEC1] but since OpenSSL used to do this, we ignore
		// this too.
		copy(privateKey[len(privateKey)-len(privKey.PrivateKey):], privKey.PrivateKey)
		priv.X, priv.Y = curve.ScalarBaseMult(privateKey)

		return priv, nil
	default:
		return nil, errors.New("x509: invalid private key curve param")
	}

}
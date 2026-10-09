package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"slices"
	"time"
)

// attestationOID is the Android key attestation extension on the leaf.
var attestationOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}

const (
	levelTrustedEnvironment = 1
	levelStrongBox          = 2
	// AuthorizationList tag numbers and the values the policy asks for.
	tagPurpose, tagAlgorithm, tagECCurve, tagNoAuthRequired, tagUserAuthType = 1, 2, 10, 503, 504
	purposeSign, algorithmEC, curveP256                                      = 2, 3, 1
)

// keyDescription is the attestation extension, whose two lists stay raw until read.
type keyDescription struct {
	AttestationVersion       int
	AttestationSecurityLevel asn1.Enumerated
	KeyMintVersion           int
	KeyMintSecurityLevel     asn1.Enumerated
	AttestationChallenge     []byte
	UniqueID                 []byte
	SoftwareEnforced         asn1.RawValue
	HardwareEnforced         asn1.RawValue
}

// authList is the few entries of an AuthorizationList the policy reads.
type authList struct {
	purposes                       []int
	algorithm, curve, userAuth     int
	hasAlgorithm, hasCurve, noAuth bool
	hasUserAuth                    bool
}

func readAuthList(list asn1.RawValue) (authList, error) {
	var out authList
	rest := list.Bytes
	for len(rest) > 0 {
		var entry asn1.RawValue
		var err error
		if rest, err = asn1.Unmarshal(rest, &entry); err != nil {
			return out, fmt.Errorf("authorization list: %w", err)
		}
		if entry.Class != asn1.ClassContextSpecific {
			continue
		}
		switch entry.Tag {
		case tagPurpose:
			if _, err = asn1.UnmarshalWithParams(entry.Bytes, &out.purposes, "set"); err != nil {
				return out, fmt.Errorf("purpose: %w", err)
			}
		case tagAlgorithm:
			out.hasAlgorithm = true
			_, err = asn1.Unmarshal(entry.Bytes, &out.algorithm)
		case tagECCurve:
			out.hasCurve = true
			_, err = asn1.Unmarshal(entry.Bytes, &out.curve)
		case tagUserAuthType:
			out.hasUserAuth = true
			_, err = asn1.Unmarshal(entry.Bytes, &out.userAuth)
		case tagNoAuthRequired:
			out.noAuth = true
		}
		if err != nil {
			return out, fmt.Errorf("authorization list tag %d: %w", entry.Tag, err)
		}
	}
	return out, nil
}

// verifyAttestation checks that pub lives in secure hardware, signs only, and needs the
// user's authentication on every use, and returns the level. Software attestation fails.
func verifyAttestation(chain []*x509.Certificate, roots *x509.CertPool, challenge []byte, pub *ecdsa.PublicKey, now time.Time) (string, error) {
	if len(chain) == 0 {
		return "", errors.New("the attestation chain is empty")
	}
	leaf := chain[0]
	intermediates := x509.NewCertPool()
	for _, each := range chain[1:] {
		intermediates.AddCert(each)
	}
	opts := x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	if _, err := leaf.Verify(opts); err != nil {
		return "", fmt.Errorf("the attestation chain does not lead to a Google attestation root: %w", err)
	}
	leafKey, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || !leafKey.Equal(pub) {
		return "", errors.New("the attestation certifies a different key than the one enrolled")
	}
	var extension []byte
	for _, each := range leaf.Extensions {
		if each.Id.Equal(attestationOID) {
			extension = each.Value
		}
	}
	if extension == nil {
		return "", errors.New("the leaf carries no key attestation extension")
	}
	var description keyDescription
	if _, err := asn1.Unmarshal(extension, &description); err != nil {
		return "", fmt.Errorf("the key attestation extension is malformed: %w", err)
	}
	if subtle.ConstantTimeCompare(description.AttestationChallenge, challenge) != 1 {
		return "", errors.New("the attestation was made for another challenge")
	}
	level := int(description.AttestationSecurityLevel)
	if (level != levelTrustedEnvironment && level != levelStrongBox) || (int(description.KeyMintSecurityLevel) != levelTrustedEnvironment && int(description.KeyMintSecurityLevel) != levelStrongBox) {
		return "", errors.New("the key is not held in secure hardware, software-only attestation is refused")
	}
	hardware, err := readAuthList(description.HardwareEnforced)
	if err != nil {
		return "", err
	}
	software, err := readAuthList(description.SoftwareEnforced)
	if err != nil {
		return "", err
	}
	switch {
	case hardware.noAuth || software.noAuth:
		return "", errors.New("the key does not require user authentication")
	case !hardware.hasUserAuth || hardware.userAuth == 0:
		return "", errors.New("the key is not bound to a biometric or screen lock in hardware")
	case !hardware.hasAlgorithm || hardware.algorithm != algorithmEC || !hardware.hasCurve || hardware.curve != curveP256:
		return "", errors.New("the key is not an EC P-256 key in hardware")
	case !slices.Contains(hardware.purposes, purposeSign):
		return "", errors.New("the key cannot sign")
	}
	if level == levelStrongBox {
		return "strongbox", nil
	}
	return "tee", nil
}

// parsePublicKey reads an SPKI DER P-256 key.
func parsePublicKey(spki []byte) (*ecdsa.PublicKey, error) {
	parsed, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return nil, errors.New("public_key is not an SPKI DER public key")
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("public_key is not a P-256 key")
	}
	return key, nil
}

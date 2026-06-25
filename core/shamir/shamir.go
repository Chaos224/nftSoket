// Package shamir implements Shamir's Secret Sharing over GF(256).
//
// It is used to split a master secret (for example the key derived from the
// paper access key) into N shares such that any K of them can reconstruct the
// secret, while K-1 shares reveal nothing. This is how the system satisfies the
// "store information in at least 2 points" requirement for the most sensitive
// material: no single custodian/location holds a usable copy of the key.
package shamir

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
)

// GF(256) multiplication tables built from the AES polynomial x^8+x^4+x^3+x+1.
var (
	gfExp [512]byte
	gfLog [256]byte
)

func init() {
	x := byte(1)
	for i := 0; i < 255; i++ {
		gfExp[i] = x
		gfLog[x] = byte(i)
		// multiply x by the generator 0x03 in GF(256)
		hi := x & 0x80
		x <<= 1
		if hi != 0 {
			x ^= 0x1b
		}
		x ^= gfExp[i] // x = x*2 ^ x = x*3
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func gfDiv(a, b byte) byte {
	if b == 0 {
		panic("shamir: division by zero")
	}
	if a == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])-int(gfLog[b])+255]
}

// eval evaluates the polynomial whose coefficients are given (constant term
// first) at point x in GF(256).
func eval(coeffs []byte, x byte) byte {
	// Horner's method, high degree first.
	result := byte(0)
	for i := len(coeffs) - 1; i >= 0; i-- {
		result = gfMul(result, x) ^ coeffs[i]
	}
	return result
}

// Share is one piece of a split secret. X must be unique and non-zero across a
// set of shares; Y holds one byte of share data per secret byte.
type Share struct {
	X byte
	Y []byte
}

// Split divides secret into parts shares, of which any threshold can recover it.
func Split(secret []byte, parts, threshold int) ([]Share, error) {
	if threshold < 2 {
		return nil, errors.New("shamir: threshold must be at least 2")
	}
	if parts < threshold {
		return nil, errors.New("shamir: parts must be >= threshold")
	}
	if parts > 255 {
		return nil, errors.New("shamir: parts must be <= 255")
	}
	if len(secret) == 0 {
		return nil, errors.New("shamir: secret must not be empty")
	}

	// Assign each share a distinct non-zero x coordinate (1..parts).
	shares := make([]Share, parts)
	for i := range shares {
		shares[i] = Share{X: byte(i + 1), Y: make([]byte, len(secret))}
	}

	coeffs := make([]byte, threshold)
	for byteIdx, b := range secret {
		// Random polynomial of degree threshold-1 with constant term = secret byte.
		coeffs[0] = b
		if _, err := rand.Read(coeffs[1:]); err != nil {
			return nil, fmt.Errorf("shamir: read randomness: %w", err)
		}
		for i := range shares {
			shares[i].Y[byteIdx] = eval(coeffs, shares[i].X)
		}
	}
	return shares, nil
}

// Combine reconstructs the secret from at least `threshold` shares via Lagrange
// interpolation at x=0.
func Combine(shares []Share) ([]byte, error) {
	if len(shares) < 2 {
		return nil, errors.New("shamir: need at least 2 shares")
	}
	length := len(shares[0].Y)
	seenX := make(map[byte]bool, len(shares))
	for _, s := range shares {
		if s.X == 0 {
			return nil, errors.New("shamir: share x coordinate must be non-zero")
		}
		if seenX[s.X] {
			return nil, errors.New("shamir: duplicate share x coordinate")
		}
		seenX[s.X] = true
		if len(s.Y) != length {
			return nil, errors.New("shamir: shares have inconsistent length")
		}
	}

	secret := make([]byte, length)
	for byteIdx := 0; byteIdx < length; byteIdx++ {
		var acc byte
		for i, si := range shares {
			// Lagrange basis polynomial evaluated at 0.
			num := byte(1)
			den := byte(1)
			for j, sj := range shares {
				if i == j {
					continue
				}
				num = gfMul(num, sj.X)          // (0 - x_j) == x_j in GF(256)
				den = gfMul(den, si.X^sj.X)     // (x_i - x_j)
			}
			acc ^= gfMul(si.Y[byteIdx], gfDiv(num, den))
		}
		secret[byteIdx] = acc
	}
	return secret, nil
}

// ConstantTimeEqual reports whether two secrets are equal without leaking timing.
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// internal/features/capture/pairing_code.go
package capture

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

const pairingCodeEntropyBytes = 32

type securePairingCodeGenerator struct {
	random io.Reader
}

func (g securePairingCodeGenerator) Generate() (string, []byte, error) {
	random := g.random
	if random == nil {
		random = rand.Reader
	}
	value := make([]byte, pairingCodeEntropyBytes)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", nil, err
	}
	plain := base64.RawURLEncoding.EncodeToString(value)
	hash := sha256.Sum256([]byte(plain))
	return plain, hash[:], nil
}

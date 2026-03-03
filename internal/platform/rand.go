package platform

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
)

// newCryptoSeededRand creates a new math/rand.Rand instance seeded with
// cryptographically random data. This produces less predictable mouse
// patterns compared to time-based seeding.
func newCryptoSeededRand() *rand.Rand {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to zero seed rather than panicking
		return rand.New(rand.NewSource(0))
	}
	seed := int64(binary.LittleEndian.Uint64(b[:]))
	return rand.New(rand.NewSource(seed))
}

package kernel

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// derivedSubagentRPCRequestID keeps transport correlation identifiers bounded
// even when the durable delivery identifier already uses the domain maximum.
// Length framing prevents distinct field tuples from hashing the same byte
// stream through delimiter ambiguity.
func derivedSubagentRPCRequestID(namespace string, fields ...string) string {
	digest := sha256.New()
	var size [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		_, _ = digest.Write(size[:])
		_, _ = digest.Write([]byte(field))
	}
	return namespace + ":" + hex.EncodeToString(digest.Sum(nil))
}

package memory

import (
	"testing"
)

func TestSemanticMemoryAbstractionExists(t *testing.T) {
	var _ SemanticMemory = nil // compile-time proof
}


package memory_test

import (
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	"motor-autonomo/internal/storage/memory"
)

func TestStoreContract(t *testing.T) {
	contract.TestStore(t, func() port.Store { return memory.New() })
}

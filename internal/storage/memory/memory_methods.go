package memory

import (
	"errors"
	"motor-autonomo/internal/domain"
	"time"
)

func (t transaction) LongTermMemory(key string) (domain.LongTermMemory, error) {
	return reader(t).LongTermMemory(key)
}

func (t transaction) ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error) {
	return reader(t).ListMemoriesByScope(scope)
}

func (t transaction) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	return reader(t).ListExpiredMemories(now)
}

func (t transaction) SaveMemory(mem domain.LongTermMemory) error {
	t.state.memories[mem.Key] = mem
	return nil
}

func (t transaction) DeleteMemory(id domain.MemoryID) error {
	for k, v := range t.state.memories {
		if v.ID == id {
			delete(t.state.memories, k)
			return nil
		}
	}
	return nil
}

func (r reader) LongTermMemory(key string) (domain.LongTermMemory, error) {
	mem, ok := r.state.memories[key]
	if !ok {
		return domain.LongTermMemory{}, errors.New("not found")
	}
	return mem, nil
}

func (r reader) ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error) {
	var res []domain.LongTermMemory
	for _, m := range r.state.memories {
		if m.Scope == scope {
			res = append(res, m)
		}
	}
	return res, nil
}

func (r reader) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	var res []domain.LongTermMemory
	for _, m := range r.state.memories {
		if !m.Expiration.IsZero() && now.After(m.Expiration) {
			res = append(res, m)
		}
	}
	return res, nil
}

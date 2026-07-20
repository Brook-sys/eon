package memory

import (
	"errors"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"sort"
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
	if err := mem.Validate(); err != nil {
		return err
	}
	if current, ok := t.state.memories[mem.Key]; ok && current.ID != mem.ID {
		return errors.New("memory key already belongs to another ID")
	}
	for key, current := range t.state.memories {
		if current.ID == mem.ID && key != mem.Key {
			return errors.New("memory ID already belongs to another key")
		}
	}
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
		return domain.LongTermMemory{}, port.ErrNotFound
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
	sort.Slice(res, func(i, j int) bool {
		if res[i].Key != res[j].Key {
			return res[i].Key < res[j].Key
		}
		return res[i].ID < res[j].ID
	})
	return res, nil
}

func (r reader) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	var res []domain.LongTermMemory
	for _, m := range r.state.memories {
		if !m.Expiration.IsZero() && !m.Expiration.After(now) {
			res = append(res, m)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if !res[i].Expiration.Equal(res[j].Expiration) {
			return res[i].Expiration.Before(res[j].Expiration)
		}
		return res[i].ID < res[j].ID
	})
	return res, nil
}

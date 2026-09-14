package idempotency

import "sync"

type Store[T any] struct {
	mu     sync.Mutex
	values map[string]T
}

func New[T any]() *Store[T] { return &Store[T]{values: make(map[string]T)} }

func (s *Store[T]) GetOrPut(key string, create func() T) (value T, existing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value, ok := s.values[key]; ok {
		return value, true
	}
	value = create()
	s.values[key] = value
	return value, false
}

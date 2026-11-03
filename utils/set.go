package utils

import (
	"fmt"
	"sort"
)

func sortSlice[T comparable](slice []T) {
	sort.Slice(slice, func(i, j int) bool {
		return fmt.Sprintf("%v", slice[i]) < fmt.Sprintf("%v", slice[j])
	})
}

type SetInterface[T comparable] interface {
	Add(value T)
	Adds(values ...T)
	Remove(value T)
	Contains(value T) bool
	Len() int
	Values() []T
	Equal(other SetInterface[T]) bool
}

func NewSet[T comparable]() SetInterface[T] {
	return &Set[T]{elements: make(map[T]struct{})}
}

type Set[T comparable] struct {
	elements map[T]struct{}
}

func (s *Set[T]) Add(value T) {
	s.elements[value] = struct{}{}
}

func (s *Set[T]) Adds(values ...T) {
	for _, value := range values {
		s.Add(value)
	}
}

func (s *Set[T]) Remove(value T) {
	delete(s.elements, value)
}

func (s *Set[T]) Contains(value T) bool {
	_, ok := s.elements[value]
	return ok
}

func (s *Set[T]) Len() int {
	return len(s.elements)
}

func (s *Set[T]) Values() []T {
	values := make([]T, 0, len(s.elements))
	for value := range s.elements {
		values = append(values, value)
	}
	sortSlice(values)
	return values
}

func (s *Set[T]) Equal(other SetInterface[T]) bool {
	if s.Len() != other.Len() {
		return false
	}
	for _, value := range s.Values() {
		if !other.Contains(value) {
			return false
		}
	}
	return true

}

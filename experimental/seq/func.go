// Copyright 2020-2025 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package seq

// Func implements [Indexer][T] using an access function as underlying storage.
//
// It uses a function that gets the value at the given index. Attempting to
// Set a value will panic.
type Func[T any] struct {
	Count int
	Get   func(int) T
}

// NewFunc constructs a new [Func].
//
// This method exists because Go currently will not infer type parameters of a
// type.
func NewFunc[T any](count int, get func(int) T) Func[T] {
	return Func[T]{count, get}
}

// Len implements [Indexer].
func (s Func[T]) Len() int {
	return s.Count
}

// At implements [Indexer].
func (s Func[T]) At(idx int) T {
	// Panicking bounds check. This does not allocate.
	_ = make([]struct{}, s.Count)[idx]

	return s.Get(idx)
}

// SetAt implements [Setter] by panicking.
func (s Func[T]) SetAt(int, T) {
	panic("seq: called Func[...].SetAt")
}

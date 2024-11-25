// Copyright 2020-2024 Buf Technologies, Inc.
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

package internal

import "fmt"

// Oxford is a formattable value that prints a slice joined with commas, but
// taking care to join them with the given conjunction (such as "and") and
// ensuring that an Oxford comma is included, but only when necessary.
type Oxford[T any] struct {
	Conjunction string
	Elements    []T
}

var _ fmt.Formatter = Oxford[int]{}

// Format implements [fmt.Formatter].
func (o Oxford[T]) Format(out fmt.State, verb rune) {
	if verb != 'v' || out.Flag('#') {
		fmt.Fprintf(out, "%#v", struct {
			Conjunction string
			Elements    []T
		}(o))
		return
	}

	switch len(o.Elements) {
	case 0:
	case 1:
		fmt.Fprintf(out, "%v", o.Elements[0])
	case 2:
		fmt.Fprintf(out, "%v %s %v", o.Elements[0], o.Conjunction, o.Elements[1])
	default:
		for _, v := range o.Elements[:len(o.Elements)-1] {
			fmt.Fprintf(out, "%v, ", v)
		}
		fmt.Fprintf(out, "%s %v", o.Conjunction, o.Elements[len(o.Elements)-1])
	}
}

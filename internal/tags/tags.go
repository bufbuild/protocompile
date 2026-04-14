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

// Package tag contains all of the numeric field and enum value tag numbers
// from descriptor.proto.
package tags

//go:generate go run ./gen

// Field numbers for synthetic map entry messages.
//
//nolint:revive,stylecheck
const (
	MapEntry_Key   = 1
	MapEntry_Value = 2
)

const (
	FieldBits = 29
	FieldMin  = 1
	FieldMax  = 1<<FieldBits - 1

	FirstReserved = 19000
	LastReserved  = 19999

	MessageSetBits = 31
	MessageSetMin  = 1
	MessageSetMax  = 1<<MessageSetBits - 2 // Int32Max is not valid!

	EnumBits = 32
	EnumMin  = -1 << (EnumBits - 1)
	EnumMax  = 1<<(EnumBits-1) - 1
)

const UninterpretedOption = FileOptions_UninterpretedOption

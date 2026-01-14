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

package source

import "github.com/bufbuild/protocompile/wellknownimports"

var wktFS = FS{FS: wellknownimports.FS()}

// WKTs returns an [Opener] that yields in-memory Protobuf well-known type sources.
func WKTs() Opener {
	// All openers returned by this function compare equal.
	return wkts{}
}

type wkts struct{}

func (wkts) Open(path string) (*File, error) {
	file, err := wktFS.Open(path)
	if err != nil {
		return nil, err
	}

	file.path = "<built-in>/" + path
	return file, nil
}

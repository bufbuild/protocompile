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

package incremental

import (
	"net/url"
	"strings"
)

// URLBuilder is a helper for building URLs.
//
// It is a simplified version of the interface of [net/url.URL].
type URLBuilder struct {
	Scheme   string
	Opaque   string // If non-empty, Path is ignored.
	Path     string // Unlike Path in net/url.URL, slashes are *not* escaped!
	Queries  [][2]string
	Fragment string
}

// Build builds a URL, performing escaping as necessary.
func (ub URLBuilder) Build() string {
	var buf strings.Builder

	if ub.Scheme != "" {
		buf.WriteString(ub.Scheme)
		buf.WriteString(":")
	}
	if ub.Opaque != "" {
		buf.WriteString(ub.Opaque)
	} else if ub.Path != "" {
		buf.WriteString("//")
		path := ub.Path
		for path != "" {
			slash := strings.IndexByte(path, '/')
			if slash == -1 {
				buf.WriteString(url.PathEscape(path))
				path = ""
			} else {
				buf.WriteString(url.PathEscape(path[:slash]))
				buf.WriteByte('/')
				path = path[slash+1:]
			}
		}
	}
	for i, q := range ub.Queries {
		if i == 0 {
			buf.WriteByte('?')
		} else {
			buf.WriteByte('&')
		}

		buf.WriteString(url.QueryEscape(q[0]))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(q[1]))
	}
	if ub.Fragment != "" {
		buf.WriteByte('#')
		buf.WriteString(ub.Fragment)
	}

	return buf.String()
}

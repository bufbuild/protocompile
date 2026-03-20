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

// decode is a helper tool for ingesting an encoded FileDescriptorSet and
// dumping its SourceCodeInfo in a readable format.
//
// This command can be used with protoc to decode its output as follows:
//
//	protoc -o/dev/stdout --include_source_info file.proto | go run ./experimental/ir/sourceinfo/decode
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/experimental/ir/sourceinfo"
	"github.com/bufbuild/protocompile/internal/ext/flagx"
	"github.com/bufbuild/protocompile/internal/ext/slicesx"
	"github.com/bufbuild/protocompile/internal/prototest"
)

var (
	fdp = flag.Bool("fdp", false, "treat the input as a FileDescriptorProto instead")
)

func main() {
	flagx.Main(func() error {
		in := os.Stdin
		if arg := flag.Arg(0); arg != "" && arg != "-" {
			var err error
			in, err = os.Open(arg)
			if err != nil {
				return err
			}
			defer in.Close()
		}

		data, err := io.ReadAll(in)
		if err != nil {
			return err
		}

		var files []*descriptorpb.FileDescriptorProto
		if *fdp {
			fdp := new(descriptorpb.FileDescriptorProto)
			if err := proto.Unmarshal(data, fdp); err != nil {
				return err
			}
			files = append(files, fdp)
		} else {
			fds := new(descriptorpb.FileDescriptorSet)
			if err := proto.Unmarshal(data, fds); err != nil {
				return err
			}
			files = fds.File
		}

		type loc struct {
			Path              string
			Start, End        sourceinfo.Position
			Leading, Trailing *string // Pointer so that if not present it doesn't get printed.
			Detached          []string
		}
		info := make(map[string][]loc)
		for _, fdp := range files {
			info[*fdp.Name] = slicesx.Transform(sourceinfo.Decode(fdp), func(entry sourceinfo.Location) loc {
				loc := loc{
					Path:     entry.Path.String(),
					Start:    entry.Start,
					End:      entry.End,
					Detached: entry.Detached,
				}
				if entry.Leading != "" {
					loc.Leading = &entry.Leading
				}
				if entry.Trailing != "" {
					loc.Leading = &entry.Trailing
				}
				return loc
			})
		}

		fmt.Print(prototest.ToYAML(info, prototest.ToYAMLOptions{}))
		return nil
	})
}

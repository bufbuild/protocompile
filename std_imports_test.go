package protocompile

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
)

func TestStdImports(t *testing.T) {
	// make sure we can successfully "compile" all standard imports
	// (by regurgitating the built-in descriptors)
	c := Compiler{Resolver: WithStandardImports(&SourceResolver{})}
	ctx := context.Background()
	for name, fileProto := range standardImports {
		fds, err := c.Compile(ctx, name)
		if err != nil {
			t.Errorf("failed to compile %q: %v", name, err)
			continue
		}
		if len(fds) != 1 {
			t.Errorf("Compile returned wrong number of descriptors: expecting 1, got %d", len(fds))
			continue
		}
		orig := protodesc.ToFileDescriptorProto(fileProto)
		actual := protodesc.ToFileDescriptorProto(fds[0])
		if !proto.Equal(orig, actual) {
			t.Errorf("result proto is incorrect:\n expecting %v\n got %v", orig, actual)
		}
	}
}

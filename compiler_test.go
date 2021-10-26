package protocompile

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/jhump/protocompile/internal/testprotos"
)

func TestParseFilesMessageComments(t *testing.T) {
	comp := Compiler{
		Resolver:          &SourceResolver{},
		IncludeSourceInfo: true,
	}
	ctx := context.Background()
	protos, err := comp.Compile(ctx, "internal/testprotos/desc_test1.proto")
	if !assert.Nil(t, err, "%v", err) {
		t.FailNow()
	}
	comments := ""
	expected := " Comment for TestMessage\n"
	for _, fd := range protos {
		msg := fd.Messages().ByName("testprotos.TestMessage")
		if msg != nil {
			si := fd.SourceLocations().ByDescriptor(msg)
			if si.Path != nil {
				comments = si.LeadingComments
			}
			break
		}
	}
	assert.Equal(t, expected, comments)
}

func TestParseFilesWithImportsNoImportPath(t *testing.T) {
	relFilePaths := []string{
		"a/b/b1.proto",
		"a/b/b2.proto",
		"c/c.proto",
	}

	pwd, err := os.Getwd()
	assert.Nil(t, err, "%v", err)

	err = os.Chdir("internal/testprotos/protoparse")
	assert.Nil(t, err, "%v", err)
	defer func() {
		// restore working directory
		_ = os.Chdir(pwd)
	}()

	comp := Compiler{
		Resolver: &SourceResolver{},
	}
	ctx := context.Background()
	protos, err := comp.Compile(ctx, relFilePaths...)
	if !assert.Nil(t, err, "%v", err) {
		t.FailNow()
	}
	assert.Equal(t, len(relFilePaths), len(protos))
}

func TestParseFilesWithDependencies(t *testing.T) {
	// Create some file contents that import a non-well-known proto.
	// (One of the protos in internal/testprotos is fine.)
	contents := map[string]string{
		"test.proto": `
			syntax = "proto3";
			import "desc_test_wellknowntypes.proto";

			message TestImportedType {
				testprotos.TestWellKnownTypes imported_field = 1;
			}
		`,
	}
	baseResolver := ResolverFunc(func(f string) (SearchResult, error) {
		s, ok := contents[f]
		if !ok {
			return SearchResult{}, os.ErrNotExist
		}
		return SearchResult{Source: strings.NewReader(s)}, nil
	})

	wktDesc, err := protoregistry.GlobalFiles.FindFileByPath("desc_test_wellknowntypes.proto")
	assert.Nil(t, err)
	wktDescProto := protodesc.ToFileDescriptorProto(wktDesc)
	ctx := context.Background()

	// Establish that we *can* parse the source file with a parser that
	// registers the dependency.
	t.Run("DependencyIncluded", func(t *testing.T) {
		// Create a dependency-aware compiler.
		compiler := Compiler{
			Resolver: ResolverFunc(func(f string) (SearchResult, error) {
				if f == "desc_test_wellknowntypes.proto" {
					return SearchResult{Desc: wktDesc}, nil
				}
				return baseResolver.FindFileByPath(f)
			}),
		}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.Nil(t, err, "%v", err)
	})
	t.Run("DependencyIncludedProto", func(t *testing.T) {
		// Create a dependency-aware compiler.
		compiler := Compiler{
			Resolver: ResolverFunc(func(f string) (SearchResult, error) {
				if f == "desc_test_wellknowntypes.proto" {
					return SearchResult{Proto: wktDescProto}, nil
				}
				return baseResolver.FindFileByPath(f)
			}),
		}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.Nil(t, err, "%v", err)
	})

	// Establish that we *can not* parse the source file with a parser that
	// did not register the dependency.
	t.Run("DependencyExcluded", func(t *testing.T) {
		// Create a dependency-UNaware parser.
		compiler := Compiler{Resolver: baseResolver}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.NotNil(t, err, "expected parse to fail")
	})

	// Establish that the accessor has precedence over LookupImport.
	t.Run("AccessorWins", func(t *testing.T) {
		// Create a dependency-aware parser that should never be called.
		compiler := Compiler{
			Resolver: ResolverFunc(func(f string) (SearchResult, error) {
				if f == "test.proto" {
					return SearchResult{Source: strings.NewReader(`syntax = "proto3";`)}, nil
				}
				t.Errorf("resolved was called for unexpected filename %q", f)
				return SearchResult{}, os.ErrNotExist
			}),
		}
		_, err := compiler.Compile(ctx, "test.proto")
		assert.Nil(t, err)
	})
}

func TestParseCommentsBeforeDot(t *testing.T) {
	accessor := SourceAccessorFromMap(map[string]string{
		"test.proto": `
syntax = "proto3";
message Foo {
  // leading comments
  .Foo foo = 1;
}
`,
	})

	compiler := Compiler{
		Resolver:          &SourceResolver{Accessor: accessor},
		IncludeSourceInfo: true,
	}
	ctx := context.Background()
	fds, err := compiler.Compile(ctx, "test.proto")
	assert.Nil(t, err)

	field := fds[0].Messages().Get(0).Fields().Get(0)
	comment := fds[0].SourceLocations().ByDescriptor(field).LeadingComments
	assert.Equal(t, " leading comments\n", comment)
}

func TestParseCustomOptions(t *testing.T) {
	accessor := SourceAccessorFromMap(map[string]string{
		"test.proto": `
syntax = "proto3";
import "google/protobuf/descriptor.proto";
extend google.protobuf.MessageOptions {
    string foo = 30303;
    int64 bar = 30304;
}
message Foo {
  option (.foo) = "foo";
  option (bar) = 123;
}
`,
	})

	compiler := Compiler{
		Resolver:          WithStandardImports(&SourceResolver{Accessor: accessor}),
		IncludeSourceInfo: true,
	}
	ctx := context.Background()
	fds, err := compiler.Compile(ctx, "test.proto")
	if !assert.Nil(t, err, "%v", err) {
		t.FailNow()
	}

	md := fds[0].Messages().Get(0)
	data := md.Options().ProtoReflect().GetUnknown()

	tag, wt, n := protowire.ConsumeTag(data)
	assert.True(t, n > 0)
	assert.Equal(t, protowire.Number(30303), tag)
	assert.Equal(t, protowire.BytesType, wt)

	data = data[n:]
	fieldData, n := protowire.ConsumeBytes(data)
	assert.True(t, n > 0)
	assert.Equal(t, "foo", string(fieldData))

	data = data[n:]
	tag, wt, n = protowire.ConsumeTag(data)
	assert.True(t, n > 0)
	assert.Equal(t, protowire.Number(30304), tag)
	assert.Equal(t, protowire.VarintType, wt)

	data = data[n:]
	fieldVal, n := protowire.ConsumeVarint(data)
	assert.True(t, n > 0)
	assert.Equal(t, uint64(123), fieldVal)
}


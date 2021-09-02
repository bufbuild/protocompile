package protocompile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
)

func TestErrorReporting(t *testing.T) {
	tooManyErrors := errors.New("too many errors")
	limitedErrReporter := func(limit int, count *int) reporter.ErrorReporter {
		return func(err reporter.ErrorWithPos) error {
			*count++
			if *count > limit {
				return tooManyErrors
			}
			return nil
		}
	}
	trackingReporter := func(errs *[]reporter.ErrorWithPos, count *int) reporter.ErrorReporter {
		return func(err reporter.ErrorWithPos) error {
			*count++
			*errs = append(*errs, err)
			return nil
		}
	}
	fail := errors.New("failure!")
	failFastReporter := func(count *int) reporter.ErrorReporter {
		return func(err reporter.ErrorWithPos) error {
			*count++
			return fail
		}
	}

	testCases := []struct {
		fileNames    []string
		files        map[string]string
		expectedErrs []string
	}{
		{
			// multiple syntax errors
			fileNames: []string{"test.proto"},
			files: map[string]string{
				"test.proto": `
					syntax = "proto";
					package foo

					enum State { A = 0; B = 1; C; D }
					message Foo {
						foo = 1;
					}
					`,
			},
			expectedErrs: []string{
				"test.proto:5:41: syntax error: unexpected \"enum\", expecting ';'",
				"test.proto:5:69: syntax error: unexpected ';', expecting '='",
				"test.proto:7:53: syntax error: unexpected '='",
				`test.proto:2:50: syntax value must be "proto2" or "proto3"`,
			},
		},
		{
			// multiple validation errors
			fileNames: []string{"test.proto"},
			files: map[string]string{
				"test.proto": `
					syntax = "proto3";
					message Foo {
						string foo = 0;
					}
					enum State { C }
					enum Bar {
						BAZ = 1;
						BUZZ = 1;
					}
					`,
			},
			expectedErrs: []string{
				"test.proto:6:56: syntax error: unexpected '}', expecting '='",
				"test.proto:4:62: tag number 0 must be greater than zero",
				"test.proto:8:55: enum Bar: proto3 requires that first value in enum have numeric value of 0",
				"test.proto:9:56: enum Bar: values BAZ and BUZZ both have the same numeric value 1; use allow_alias option if intentional",
			},
		},
		{
			// multiple link errors
			fileNames: []string{"test.proto"},
			files: map[string]string{
				"test.proto": `
					syntax = "proto3";
					message Foo {
						string foo = 1;
					}
					enum Bar {
						BAZ = 0;
						BAZ = 2;
					}
					service Bar {
						rpc Foobar (Foo) returns (Foo);
						rpc Foobar (Frob) returns (Nitz);
					}
					`,
			},
			expectedErrs: []string{
				"test.proto:8:49: duplicate symbol BAZ: already defined as enum value; protobuf uses C++ scoping rules for enum values, so they exist in the scope enclosing the enum",
				"test.proto:10:41: duplicate symbol Bar: already defined as enum",
				"test.proto:12:49: duplicate symbol Bar.Foobar: already defined as method",
				"test.proto:12:61: method Bar.Foobar: unknown request type Frob",
				"test.proto:12:76: method Bar.Foobar: unknown response type Nitz",
			},
		},
		{
			// syntax errors across multiple files
			fileNames: []string{"test1.proto", "test2.proto"},
			files: map[string]string{
				"test1.proto": `
					syntax = "proto3";
					import "test2.proto";
					message Foo {
						string foo = -1;
					}
					service Bar {
						rpc Foo (Foo);
					}
					`,
				"test2.proto": `
					syntax = "proto3";
					message Baz {
						required string foo = 1;
					}
					service Service {
						Foo; Bar; Baz;
					}
					`,
			},
			expectedErrs: []string{
				"test1.proto:5:62: syntax error: unexpected '-', expecting int literal",
				"test1.proto:8:62: syntax error: unexpected ';', expecting \"returns\"",
				"test2.proto:7:49: syntax error: unexpected identifier, expecting \"option\" or \"rpc\" or ';' or '}'",
				"test2.proto:4:49: field Baz.foo: label 'required' is not allowed in proto3",
			},
		},
		{
			// link errors across multiple files
			fileNames: []string{"test1.proto", "test2.proto"},
			files: map[string]string{
				"test1.proto": `
					syntax = "proto3";
					import "test2.proto";
					message Foo {
						string foo = 1;
					}
					service Bar {
						rpc Frob (Empty) returns (Nitz);
					}
					`,
				"test2.proto": `
					syntax = "proto3";
					message Empty {}
					enum Bar {
						BAZ = 0;
					}
					service Foo {
						rpc DoSomething (Empty) returns (Foo);
					}
					`,
			},
			expectedErrs: []string{
				"test2.proto:4:41: duplicate symbol Bar: already defined as service in \"test1.proto\"",
				"test2.proto:7:41: duplicate symbol Foo: already defined as message in \"test1.proto\"",
				"test1.proto:8:75: method Bar.Frob: unknown response type Nitz",
				"test2.proto:8:82: method Foo.DoSomething: invalid response type: Foo is a service, not a message",
			},
		},
	}

	ctx := context.Background()
	for i, tc := range testCases {
		compiler := Compiler{
			Resolver: &SourceResolver{Accessor: SourceAccessorFromMap(tc.files)},
		}

		var reported []reporter.ErrorWithPos
		count := 0
		compiler.Reporter = reporter.NewReporter(trackingReporter(&reported, &count), nil)
		_, err := compiler.Compile(ctx, tc.fileNames...)
		reportedMsgs := make([]string, len(reported))
		for j := range reported {
			reportedMsgs[j] = reported[j].Error()
		}
		t.Logf("case #%d: got %d errors:\n\t%s", i+1, len(reported), strings.Join(reportedMsgs, "\n\t"))

		// returns sentinel, but all actual errors in reported
		assert.Equal(t, reporter.ErrInvalidSource, err, "case #%d: parse should have failed with invalid source error", i+1)
		assert.Equal(t, len(tc.expectedErrs), count, "case #%d: parse should have called reporter %d times", i+1, len(tc.expectedErrs))
		if assert.Equal(t, len(tc.expectedErrs), len(reported), "case #%d: wrong number of errors reported", i+1) {
			for j := range tc.expectedErrs {
				if !assert.Equal(t, tc.expectedErrs[j], reported[j].Error(), "case #%d: parse error[%d] have %q; instead got %q", i+1, j, tc.expectedErrs[j], reported[j].Error()) {
					continue
				}
				split := strings.SplitN(tc.expectedErrs[j], ":", 4)
				if assert.Equal(t, 4, len(split), "case #%d: expected %q [%d] to contain at least 4 elements split by :", i+1, tc.expectedErrs[j], j) {
					assert.Equal(t, split[3], " "+reported[j].Unwrap().Error(), "case #%d: parse error underlying[%d] have %q; instead got %q", i+1, j, split[3], reported[j].Unwrap().Error())
				}
			}
		}

		count = 0
		compiler.Reporter = reporter.NewReporter(failFastReporter(&count), nil)
		_, err = compiler.Compile(ctx, tc.fileNames...)
		assert.Equal(t, fail, err, "case #%d: parse should have failed fast", i+1)
		assert.Equal(t, 1, count, "case #%d: parse should have called reporter only once", i+1)

		count = 0
		compiler.Reporter = reporter.NewReporter(limitedErrReporter(3, &count), nil)
		_, err = compiler.Compile(ctx, tc.fileNames...)
		if len(tc.expectedErrs) > 3 {
			assert.Equal(t, tooManyErrors, err, "case #%d: parse should have failed with too many errors", i+1)
			assert.Equal(t, 4, count, "case #%d: parse should have called reporter 4 times", i+1)
		} else {
			// less than threshold means reporter always returned nil,
			// so parse returns ErrInvalidSource sentinel
			assert.Equal(t, reporter.ErrInvalidSource, err, "case #%d: parse should have failed with invalid source error", i+1)
			assert.Equal(t, len(tc.expectedErrs), count, "case #%d: parse should have called reporter %d times", i+1, len(tc.expectedErrs))
		}
	}
}

func TestWarningReporting(t *testing.T) {
	type msg struct {
		pos  ast.SourcePos
		text string
	}
	var msgs []msg
	rep := func(warn reporter.ErrorWithPos) {
		msgs = append(msgs, msg{
			pos: warn.GetPosition(), text: warn.Unwrap().Error(),
		})
	}

	testCases := []struct {
		name            string
		sources         map[string]string
		expectedNotices []string
	}{
		{
			name: "syntax proto2",
			sources: map[string]string{
				"test.proto": `syntax = "proto2"; message Foo {}`,
			},
		},
		{
			name: "syntax proto3",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; message Foo {}`,
			},
		},
		{
			name: "no syntax",
			sources: map[string]string{
				"test.proto": `message Foo {}`,
			},
			expectedNotices: []string{
				"test.proto:1:1: no syntax specified; defaulting to proto2 syntax",
			},
		},
		{
			name: "used import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "foo.proto"; message Foo { Bar bar = 1; }`,
				"foo.proto":  `syntax = "proto3"; message Bar { string name = 1; }`,
			},
		},
		{
			name: "used public import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "foo.proto"; message Foo { Bar bar = 1; }`,
				// we're only asking to compile test.proto, so we won't report unused import for baz.proto
				"foo.proto": `syntax = "proto3"; import public "bar.proto"; import "baz.proto";`,
				"bar.proto": `syntax = "proto3"; message Bar { string name = 1; }`,
				"baz.proto": `syntax = "proto3"; message Baz { }`,
			},
		},
		{
			name: "used nested public import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "foo.proto"; message Foo { Bar bar = 1; }`,
				"foo.proto":  `syntax = "proto3"; import public "baz.proto";`,
				"baz.proto":  `syntax = "proto3"; import public "bar.proto";`,
				"bar.proto":  `syntax = "proto3"; message Bar { string name = 1; }`,
			},
		},
		{
			name: "unused import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "foo.proto"; message Foo { string name = 1; }`,
				"foo.proto":  `syntax = "proto3"; message Bar { string name = 1; }`,
			},
			expectedNotices: []string{
				`test.proto:1:20: import "foo.proto" not used`,
			},
		},
		{
			name: "multiple unused imports",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "foo.proto"; import "bar.proto"; import "baz.proto"; message Test { Bar bar = 1; }`,
				"foo.proto":  `syntax = "proto3"; message Foo {};`,
				"bar.proto":  `syntax = "proto3"; message Bar {};`,
				"baz.proto":  `syntax = "proto3"; message Baz {};`,
			},
			expectedNotices: []string{
				`test.proto:1:20: import "foo.proto" not used`,
				`test.proto:1:60: import "baz.proto" not used`,
			},
		},
		{
			name: "unused public import is not reported",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import public "foo.proto"; message Foo { }`,
				"foo.proto":  `syntax = "proto3"; message Bar { string name = 1; }`,
			},
		},
		{
			name: "unused descriptor.proto import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "google/protobuf/descriptor.proto"; message Foo { }`,
			},
			expectedNotices: []string{
				`test.proto:1:20: import "google/protobuf/descriptor.proto" not used`,
			},
		},
		{
			name: "explicitly used descriptor.proto import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "google/protobuf/descriptor.proto"; extend google.protobuf.MessageOptions { string foobar = 33333; }`,
			},
		},
		{
			// having options implicitly uses decriptor.proto
			name: "implicitly used descriptor.proto import",
			sources: map[string]string{
				"test.proto": `syntax = "proto3"; import "google/protobuf/descriptor.proto"; message Foo { option deprecated = true; }`,
			},
		},
		{
			// makes sure we can use a given descriptor.proto to override non-custom options
			name: "implicitly used descriptor.proto import with new option",
			sources: map[string]string{
				"test.proto":                       `syntax = "proto3"; import "google/protobuf/descriptor.proto"; message Foo { option foobar = 123; }`,
				"google/protobuf/descriptor.proto": `syntax = "proto2"; package google.protobuf; message MessageOptions { optional fixed32 foobar = 99; }`,
			},
		},
	}
	ctx := context.Background()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			compiler := Compiler{
				Resolver: &SourceResolver{Accessor: SourceAccessorFromMap(testCase.sources)},
				Reporter: reporter.NewReporter(nil, rep),
			}
			msgs = nil
			_, err := compiler.Compile(ctx, "test.proto")
			assert.Nil(t, err)

			actualNotices := make([]string, len(msgs))
			for j, msg := range msgs {
				actualNotices[j] = fmt.Sprintf("%s: %s", msg.pos, msg.text)
			}
			assert.Equal(t, testCase.expectedNotices, actualNotices)
		})
	}
}

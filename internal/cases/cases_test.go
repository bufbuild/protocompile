package cases_test

import (
	"testing"

	"github.com/bufbuild/protocompile/internal/cases"
	"github.com/stretchr/testify/assert"
)

func TestCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		str                        string
		snake, enum, camel, pascal string
		naiveCamel, naivePascal    string
	}{
		{str: ""},
		{str: "_"},
		{str: "__"},

		{
			str:   "foo",
			snake: "foo", enum: "FOO",
			camel: "foo", pascal: "Foo",
			naiveCamel: "foo", naivePascal: "Foo",
		},

		{
			str:   "FOO4",
			snake: "foo4", enum: "FOO4",
			camel: "foo4", pascal: "Foo4",
			naiveCamel: "FOO4", naivePascal: "FOO4",
		},

		{
			str:   "_foo",
			snake: "foo", enum: "FOO",
			camel: "foo", pascal: "Foo",
			naiveCamel: "Foo", naivePascal: "Foo",
		},
		{
			str:   "foo_",
			snake: "foo", enum: "FOO",
			camel: "foo", pascal: "Foo",
			naiveCamel: "foo", naivePascal: "Foo",
		},
		{
			str:   "foo_bar",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "fooBar", naivePascal: "FooBar",
		},
		{
			str:   "foo__bar",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "fooBar", naivePascal: "FooBar",
		},
		{
			str:   "_foo_bar",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "FooBar", naivePascal: "FooBar",
		},
		{
			str:   "FOO_BAR",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "FOOBAR", naivePascal: "FOOBAR",
		},

		{
			str:   "fooBar",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "fooBar", naivePascal: "FooBar",
		},
		{
			str:   "foo_Bar",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "fooBar", naivePascal: "FooBar",
		},
		{
			str:   "FOOBar",
			snake: "foo_bar", enum: "FOO_BAR",
			camel: "fooBar", pascal: "FooBar",
			naiveCamel: "FOOBar", naivePascal: "FOOBar",
		},
	}

	for _, test := range tests {
		t.Run(test.str, func(t *testing.T) {
			assert.Equal(t, test.snake, cases.Snake.Convert(test.str))
			assert.Equal(t, test.enum, cases.Enum.Convert(test.str))
			assert.Equal(t, test.camel, cases.Camel.Convert(test.str))
			assert.Equal(t, test.pascal, cases.Pascal.Convert(test.str))

			assert.Equal(t, test.naiveCamel, cases.Converter{Case: cases.Camel, NaiveSplit: true, NoLowercase: true}.Convert(test.str))
			assert.Equal(t, test.naivePascal, cases.Converter{Case: cases.Pascal, NaiveSplit: true, NoLowercase: true}.Convert(test.str))
		})
	}
}

module github.com/bufbuild/protocompile

go 1.24.0

require (
	github.com/bits-and-blooms/bitset v1.24.0
	github.com/bmatcuk/doublestar/v4 v4.9.1
	github.com/google/go-cmp v0.7.0
	github.com/petermattis/goid v0.0.0-20250319124200-ccd6737f222a
	github.com/pmezard/go-difflib v1.0.0
	github.com/rivo/uniseg v0.4.7
	github.com/stretchr/testify v1.11.1
	github.com/tidwall/btree v1.8.1
	golang.org/x/exp v0.0.0-20250911091902-df9299821621
	golang.org/x/sync v0.17.0
	google.golang.org/protobuf v1.36.9
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/protocolbuffers/protoscope v0.0.0-20221109213918-8e7a6aafa2c9 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)

retract v0.5.0 // Contains deadlock error

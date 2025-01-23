module github.com/bufbuild/protocompile

go 1.21

require (
	github.com/bmatcuk/doublestar/v4 v4.8.0
	github.com/google/go-cmp v0.6.0
	github.com/pmezard/go-difflib v1.0.0
	github.com/rivo/uniseg v0.4.7
	github.com/stretchr/testify v1.10.0
	golang.org/x/sync v0.10.0
	google.golang.org/protobuf v1.36.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)

retract v0.5.0 // Contains deadlock error

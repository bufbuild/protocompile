module github.com/bufbuild/protocompile

go 1.22.0

toolchain go1.23.0

require (
	github.com/google/go-cmp v0.6.0
	github.com/stretchr/testify v1.9.0
	gopkg.in/yaml.v3 v3.0.1
	golang.org/x/sync v0.8.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/yuin/goldmark v1.7.4 // indirect
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/tools v0.25.0 // indirect
)

retract v0.5.0 // Contains deadlock error

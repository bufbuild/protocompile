module github.com/bufbuild/protocompile

go 1.21

require (
	github.com/google/go-cmp v0.6.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.8.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v0.5.0 // Contains deadlock error

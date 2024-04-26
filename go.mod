module github.com/bufbuild/protocompile

go 1.20

require (
	github.com/google/go-cmp v0.6.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.7.0
	google.golang.org/protobuf v1.33.1-0.20240422163739-e4ad8f9dfc8b
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v0.5.0 // Contains deadlock error

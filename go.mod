module github.com/bufbuild/protocompile

go 1.18

require (
	github.com/google/go-cmp v0.5.9
	github.com/stretchr/testify v1.8.2
	golang.org/x/sync v0.1.0
	google.golang.org/protobuf v1.28.2-0.20230222093303-bc1253ad3743
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract (
	v0.5.0	// Contains deadlock error
)

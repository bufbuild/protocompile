module github.com/bufbuild/protocompile

go 1.25.10

require (
	buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go v1.36.12-20250109164928-1da0de137947.1
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/google/go-cmp v0.7.0
	github.com/petermattis/goid v0.0.0-20260716134002-a9b348f0a2b9
	github.com/pmezard/go-difflib v1.0.0
	github.com/protocolbuffers/protoscope v0.0.0-20221109213918-8e7a6aafa2c9
	github.com/rivo/uniseg v0.4.7
	github.com/stretchr/testify v1.11.1
	github.com/tidwall/btree v1.8.1
	golang.org/x/exp v0.0.0-20260709172345-9ea1abe57597
	golang.org/x/sync v0.22.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260709200747-435963d16310.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)

retract v0.5.0 // Contains deadlock error

package testprotos

//go:generate protoc --go_out=../../../../.. --experimental_allow_proto3_optional -I. desc_test_comments.proto desc_test_complex.proto desc_test_options.proto desc_test_defaults.proto desc_test_field_types.proto desc_test_wellknowntypes.proto

//go:generate protoc -o desc_test_complex.protoset --include_imports -I. desc_test_complex.proto
//go:generate protoc --experimental_allow_proto3_optional -o desc_test_proto3_optional.protoset --include_imports -I. desc_test_proto3_optional.proto

#!/usr/bin/env bash

set -e

cd $(dirname $0)

# Sadly, this is the version of descriptor.proto used in the descriptorpb package.
# If we don't pin this, then some comparisons of protocompile output will fail to
# match protosets only because protoc would be using a newer version of
# descriptor.proto than protocompile. (By default, protocompile just uses the
# standard versions compiled into the packages in google.golang.org/protobuf).
PROTOC_VERSION="3.14.0"
PROTOC_OS="$(uname -s)"
PROTOC_ARCH="$(uname -m)"
case "${PROTOC_OS}" in
  Darwin) PROTOC_OS="osx" ;;
  Linux) PROTOC_OS="linux" ;;
  *)
    echo "Invalid value for uname -s: ${PROTOC_OS}" >&2
    exit 1
esac

# This is for macs with M1 chips. Precompiled binaries for osx/amd64 are not available for download, so for that case
# we download the x86_64 version instead. This will work as long as rosetta2 is installed.
if [ "$PROTOC_OS" = "osx" ] && [ "$PROTOC_ARCH" = "arm64" ]; then
  PROTOC_ARCH="x86_64"
fi

PROTOC="./protoc/bin/protoc"

if [[ "$(${PROTOC} --version 2>/dev/null)" != "libprotoc ${PROTOC_VERSION}" ]]; then
  rm -rf ./protoc
  mkdir -p protoc
  curl -L "https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-${PROTOC_OS}-${PROTOC_ARCH}.zip" > protoc/protoc.zip
  cd ./protoc && unzip protoc.zip && cd ..
fi

go install google.golang.org/protobuf/cmd/protoc-gen-go

rm *.protoset *.pb.go

# Output directory will effectively be GOPATH/src.
outdir="."
${PROTOC} "--go_out=paths=source_relative:$outdir" -I. *desc_test_comments.proto desc_test_complex.proto desc_test_options.proto desc_test_defaults.proto desc_test_field_types.proto desc_test_wellknowntypes.proto

# And make descriptor set (with source info) for several files
${PROTOC} --descriptor_set_out=./desc_test_complex.protoset --include_imports -I. desc_test_complex.proto
${PROTOC} --descriptor_set_out=./desc_test_proto3_optional.protoset --include_imports -I. desc_test_proto3_optional.proto
${PROTOC} --descriptor_set_out=./options/test.protoset -I./options test.proto

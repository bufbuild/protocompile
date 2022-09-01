#!/usr/bin/env bash

set -e

cd $(dirname $0)

# This needs to match the version that was used to generate the descriptorpb
# package in the Go module "google.golang.org/protobuf".
# If we don't pin this, then some comparisons of protocompile output will fail to
# match protosets only because protoc would be using a newer version of
# descriptor.proto than protocompile. (By default, protocompile just uses the
# standard versions compiled into the descriptorpb package.)
PROTOC_VERSION="21.5"
PROTOC_OS="$(uname -s)"
PROTOC_ARCH="$(uname -m)"
case "${PROTOC_OS}" in
  Darwin) PROTOC_OS="osx" ;;
  Linux) PROTOC_OS="linux" ;;
  *)
    echo "Invalid value for uname -s: ${PROTOC_OS}" >&2
    exit 1
esac

if [ "$PROTOC_OS" = "osx" ] && [ "$PROTOC_ARCH" = "arm64" ]; then
  PROTOC_ARCH="aarch_64"
fi

PROTOC="./protoc/bin/protoc"

if [[ "$(${PROTOC} --version 2>/dev/null)" != "libprotoc ${PROTOC_VERSION}" ]]; then
  rm -rf ./protoc
  mkdir -p protoc
  curl -L "https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-${PROTOC_OS}-${PROTOC_ARCH}.zip" > protoc/protoc.zip
  cd ./protoc && unzip protoc.zip && cd ..
fi

rm *.protoset 2>/dev/null || true

# Make descriptor sets for several files
${PROTOC} --descriptor_set_out=./all.protoset --include_imports -I. *.proto
${PROTOC} --descriptor_set_out=./desc_test_complex.protoset --include_imports -I. desc_test_complex.proto
${PROTOC} --descriptor_set_out=./desc_test_proto3_optional.protoset --include_imports -I. desc_test_proto3_optional.proto
${PROTOC} --descriptor_set_out=./options/test.protoset -I./options test.proto
${PROTOC} --descriptor_set_out=./options/test_proto3.protoset -I./options test_proto3.proto

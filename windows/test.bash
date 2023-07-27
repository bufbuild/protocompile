#!/usr/bin/env bash

set -eo pipefail

#PROTOC_VERSION="$(cat $(dirname "$0")/../.protoc_version)"
#PROTOC_GEN_GO_VERSION="v1.28.2-0.20220831092852-f930b1dc76e8"
#CONNECT_VERSION="v1.5.2"
#
## Convert DOWNLOAD_CACHE from Windows format "d:\path" to Bash format "/d/path"
#DOWNLOAD_CACHE="$(echo "/${DOWNLOAD_CACHE}" | sed 's|\\|/|g' | sed 's/://')"
#PROTOC_DIR="${DOWNLOAD_CACHE}/protoc"
#mkdir -p "${DOWNLOAD_CACHE}"
#
#if [ -f "${DOWNLOAD_CACHE}/protoc/bin/protoc.exe" ]; then
#  CACHED_PROTOC_VERSION="$("${DOWNLOAD_CACHE}/protoc/bin/protoc.exe" --version | cut -d " " -f 2)"
#fi
#
#if [ "${CACHED_PROTOC_VERSION}" != "$PROTOC_VERSION" ]; then
#  PROTOC_URL="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-win64.zip"
#  curl -sSL -o "${DOWNLOAD_CACHE}/protoc.zip" "${PROTOC_URL}"
#  7z x -y -o"${DOWNLOAD_CACHE}/protoc" "${DOWNLOAD_CACHE}/protoc.zip"
#  mkdir -p "${DOWNLOAD_CACHE}/protoc/lib"
#  cp -a "${DOWNLOAD_CACHE}/protoc/include" "${DOWNLOAD_CACHE}/protoc/lib/include"
#else
#  echo "Using cached protoc"
#fi
#
#PATH="${PROTOC_DIR}/bin:${PATH}"
#
#go build ./...
#go test -vet=off -race ./...

PROTOC_ARTIFACT_SUFFIX=win64 make test
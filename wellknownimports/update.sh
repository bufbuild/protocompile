#!/bin/bash

# Updates the WKT files from the GitHub source of truth.

set -e

RAW_URL="https://raw.githubusercontent.com/protocolbuffers/protobuf/main"

cd "$(dirname "$0")"
for WKT in $(find . -name '*.proto'); do
    WKT="${WKT#"./"}"
    case $WKT in
        "google/protobuf/go_features.proto") SRC="go/$WKT" ;;
        "google/protobuf/java_features.proto") SRC="java/core/src/main/resources/$WKT" ;;
        *) SRC="src/$WKT" ;;
    esac

    echo "$RAW_URL/$SRC -> $WKT"
    curl "$RAW_URL/$SRC" > "$WKT" -f
done

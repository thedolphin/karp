#!/bin/bash

install() {
    [ -x "${GOBIN}/protoc-gen-go" ] || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    [ -x "${GOBIN}/protoc-gen-go-grpc" ] || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
} 

gen() {
    protoc \
    --go_out=. --go-grpc_out=. \
    --go_opt=M"$1=$2" --go-grpc_opt=M"$1=$2" \
    --plugin protoc-gen-go="${GOBIN}/protoc-gen-go" \
    --plugin protoc-gen-go-grpc="${GOBIN}/protoc-gen-go-grpc" \
    $1
}

[ "${GOBIN}" ] || export GOBIN=~/go/bin

set -x
install
gen proto/karp.proto pkg/karp

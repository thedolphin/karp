#!/bin/bash

docker build --output="${1:?specify output dir}" --progress=plain -f build/package/Dockerfile.buildapp .

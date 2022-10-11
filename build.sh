#!/usr/bin/env bash
set -ue

export GO111MODULE=on
export CGO_ENABLED=0

branch=$(git rev-parse --abbrev-ref HEAD)
datetime=$(date +%Y-%m-%d/%H:%M:%S)
commit_id=$(git rev-parse --short HEAD)
go_version=$(go version | awk '{print $3}')
ver_info="_branch:"${branch}_"commitid:"${commit_id}_"date:"${datetime}_"goversion:"${go_version}
project=giny
platform=$(uname)

if go build \
   -ldflags "-s -w -X main.version=${ver_info}" \
   -o $project cmd/$project/*; then
   tar -czf $project-$platform.tgz $project
fi

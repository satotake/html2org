#!/usr/bin/env bash
set -euo pipefail

git tag

read -p "next version?: " tag

echo "${tag:1}" > ./cmd/html2org/VERSION
git add $cmd_go
git commit -m "release $tag"

git tag $tag

echo tag created $tag

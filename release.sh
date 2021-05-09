#!/usr/bin/env bash
set -euo pipefail

git tag

read -p "next version?: " tag

version=./cmd/html2org/VERSION
echo "${tag:1}" > $version
git add $version
git commit -m "release $tag"

git tag $tag

echo tag created $tag

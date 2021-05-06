#!/usr/bin/env bash
set -euo pipefail

git tag

read -p "next version?: " tag

sed -i "" "s/const version = \"v[[:digit:]]*.[[:digit:]]*.[[:digit:]]*\"/const version = \"$tag\"/" ./cmd/html2org/main.go

git tag $tag

echo tag created $tag

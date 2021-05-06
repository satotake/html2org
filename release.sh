#!/usr/bin/env bash
set -euo pipefail

git tag

read -p "next version?: " tag

cmd_go=./cmd/html2org/main.go
sed -i "" "s/const version = \"v[[:digit:]]*.[[:digit:]]*.[[:digit:]]*\"/const version = \"$tag\"/" $cmd_go
git add $cmd_go
git commit -m "release $tag"

git tag $tag
git push origin $tag

echo tag created $tag

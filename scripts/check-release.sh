#!/usr/bin/env bash
set -eu

version=$(sed -n 's/^const Version = "\([^"]*\)"$/\1/p' version.go)
if [ -z "$version" ]; then
  echo "version.go does not declare Version" >&2
  exit 1
fi

check_equal() {
  expected=$1
  actual=$2
  source_file=$3
  if [ "$actual" != "$expected" ]; then
    echo "$source_file declares version $actual, expected $expected" >&2
    exit 1
  fi
}

chart_version=$(sed -n 's/^version: \([^[:space:]]*\)$/\1/p' deploy/helm/stepanel/Chart.yaml)
chart_app_version=$(sed -n 's/^appVersion: "\([^"]*\)"$/\1/p' deploy/helm/stepanel/Chart.yaml)
openapi_version=$(sed -n 's/^  version: \([^[:space:]]*\)$/\1/p' docs/openapi.yaml)

check_equal "$version" "$chart_version" deploy/helm/stepanel/Chart.yaml
check_equal "$version" "$chart_app_version" deploy/helm/stepanel/Chart.yaml
check_equal "$version" "$openapi_version" docs/openapi.yaml

if ! grep -Eq "^## \[$version\]( |$)" CHANGELOG.md; then
  echo "CHANGELOG.md has no release heading for $version" >&2
  exit 1
fi

echo "release metadata is synchronized for v$version"

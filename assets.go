package main

import "embed"

// webAssets makes the production binary self-contained. Deployments no longer
// depend on a particular working directory or a separately synchronized copy
// of the dashboard assets.
//
//go:embed web/index.html web/static/*
var webAssets embed.FS

#!/bin/sh
set -eu

go -C tools/devctl run . env-init
go -C tools/devctl run . bootstrap

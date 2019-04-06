#!/bin/bash
set -e
set -x
rm -r vendor Gopkg.* || true
go get ./...
go get -u github.com/golang/dep/cmd/dep
dep init
rm -r _vendor-*

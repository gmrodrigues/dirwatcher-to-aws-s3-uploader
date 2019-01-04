#!/bin/bash
set -e
set -x
go get ./...
go get -u github.com/golang/dep/cmd/dep
rm Gopkg.*
dep init
rm -r _vendor-*
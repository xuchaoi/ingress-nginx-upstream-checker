#!/bin/bash

cd ../cmd/upstream-checker

rm -rf upstream-checker

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo

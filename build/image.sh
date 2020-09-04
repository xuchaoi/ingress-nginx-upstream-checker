#!/bin/bash

rm -rf upstream-checker
cp ../cmd/upstream-checker/upstream-checker .
chmod +x upstream-checker

docker build --no-cache -t xuchaoi/upstream-checker:v1 .
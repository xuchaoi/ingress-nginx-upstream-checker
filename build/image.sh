#!/bin/bash

rm -rf upstream-checker
mv ../cmd/upstream-checker/upstream-checker .
chmod +x upstream-checker

docker build --no-cache -t xuchaoi/upstream-checker:v0.5 .
rm -rf upstream-checker

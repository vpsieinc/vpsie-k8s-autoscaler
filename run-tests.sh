#!/bin/bash
cd "$(dirname "$0")"
echo "Running tests from $(pwd)..."
go test -v -race -coverprofile=coverage.out ./... 2>&1

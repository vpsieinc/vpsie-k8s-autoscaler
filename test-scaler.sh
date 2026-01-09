#!/bin/bash
cd "$(dirname "$0")"
echo "Running scaler tests from $(pwd)..."
go test -v ./pkg/scaler/ -timeout 2m 2>&1

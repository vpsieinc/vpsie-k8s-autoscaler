#!/bin/bash

# Script to test the race condition fix
# This script should be run from the project root directory

set -e

echo "Testing Race Condition Fix for Issue #6"
echo "========================================"
echo ""

# Navigate to project root
echo "Current directory: $(pwd)"

# Run the specific race condition test
echo "Running TestScaleDownManager_UtilizationRaceCondition with race detector..."
go test -race -run TestScaleDownManager_UtilizationRaceCondition -v ./pkg/scaler

echo ""
echo "Running all scaler tests with race detector..."
go test -race -v ./pkg/scaler

echo ""
echo "========================================"
echo "All tests passed successfully!"

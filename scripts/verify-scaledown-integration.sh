#!/bin/bash

# verify-scaledown-integration.sh
# Comprehensive test verification script for ScaleDownManager integration
# This script runs all scale-down related tests and generates a detailed report

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Output files
REPORT_DIR="${PROJECT_ROOT}/test-reports"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="${REPORT_DIR}/scaledown-integration-report-${TIMESTAMP}.txt"
COVERAGE_DIR="${REPORT_DIR}/coverage"

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# Create report directory
mkdir -p "${REPORT_DIR}"
mkdir -p "${COVERAGE_DIR}"

# Header
echo -e "${CYAN}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║   ScaleDownManager Integration Test Verification Suite       ║${NC}"
echo -e "${CYAN}║   VPSie Kubernetes Node Autoscaler                            ║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BLUE}Report will be saved to: ${REPORT_FILE}${NC}"
echo -e "${BLUE}Coverage reports in: ${COVERAGE_DIR}${NC}"
echo ""

# Initialize report file
cat > "${REPORT_FILE}" <<EOF
╔════════════════════════════════════════════════════════════════╗
║   ScaleDownManager Integration Test Verification Report       ║
║   Generated: $(date)                          ║
╚════════════════════════════════════════════════════════════════╝

Test Environment:
  - Go Version: $(go version)
  - Project Root: ${PROJECT_ROOT}
  - Report Timestamp: ${TIMESTAMP}

═══════════════════════════════════════════════════════════════════

EOF

# Function to print section header
print_section() {
    local title="$1"
    echo ""
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${MAGENTA}  ${title}${NC}"
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "═══════════════════════════════════════════════════════════════" >> "${REPORT_FILE}"
    echo "  ${title}" >> "${REPORT_FILE}"
    echo "═══════════════════════════════════════════════════════════════" >> "${REPORT_FILE}"
    echo "" >> "${REPORT_FILE}"
}

# Function to run tests and capture results
run_test_suite() {
    local suite_name="$1"
    local test_path="$2"
    local test_flags="$3"
    local coverage_file="${COVERAGE_DIR}/${suite_name}-coverage.out"
    local test_output="${REPORT_DIR}/${suite_name}-output.txt"

    print_section "${suite_name}"

    echo -e "${BLUE}Running: go test ${test_path} ${test_flags}${NC}"
    echo "Command: go test ${test_path} ${test_flags}" >> "${REPORT_FILE}"
    echo "" >> "${REPORT_FILE}"

    # Run tests
    if go test ${test_path} ${test_flags} -coverprofile="${coverage_file}" > "${test_output}" 2>&1; then
        echo -e "${GREEN}✅ ${suite_name} PASSED${NC}"
        echo "✅ ${suite_name} PASSED" >> "${REPORT_FILE}"

        # Extract test counts
        local passed=$(grep -c "PASS:" "${test_output}" || echo "0")
        local failed=$(grep -c "FAIL:" "${test_output}" || echo "0")
        local skipped=$(grep -c "SKIP:" "${test_output}" || echo "0")

        TOTAL_TESTS=$((TOTAL_TESTS + passed + failed + skipped))
        PASSED_TESTS=$((PASSED_TESTS + passed))
        FAILED_TESTS=$((FAILED_TESTS + failed))
        SKIPPED_TESTS=$((SKIPPED_TESTS + skipped))

        # Show coverage if available
        if [ -f "${coverage_file}" ]; then
            local coverage=$(go tool cover -func="${coverage_file}" | grep total | awk '{print $3}')
            echo -e "${CYAN}  Coverage: ${coverage}${NC}"
            echo "  Coverage: ${coverage}" >> "${REPORT_FILE}"
        fi
    else
        echo -e "${RED}❌ ${suite_name} FAILED${NC}"
        echo "❌ ${suite_name} FAILED" >> "${REPORT_FILE}"

        # Extract test counts
        local passed=$(grep -c "PASS:" "${test_output}" || echo "0")
        local failed=$(grep -c "FAIL:" "${test_output}" || echo "0")
        local skipped=$(grep -c "SKIP:" "${test_output}" || echo "0")

        TOTAL_TESTS=$((TOTAL_TESTS + passed + failed + skipped))
        PASSED_TESTS=$((PASSED_TESTS + passed))
        FAILED_TESTS=$((FAILED_TESTS + failed))
        SKIPPED_TESTS=$((SKIPPED_TESTS + skipped))
    fi

    # Append detailed output to report
    echo "" >> "${REPORT_FILE}"
    echo "Detailed Output:" >> "${REPORT_FILE}"
    echo "───────────────────────────────────────────────────────────────" >> "${REPORT_FILE}"
    cat "${test_output}" >> "${REPORT_FILE}"
    echo "───────────────────────────────────────────────────────────────" >> "${REPORT_FILE}"
    echo "" >> "${REPORT_FILE}"

    # Show test output
    cat "${test_output}"
    echo ""
}

# Change to project root
cd "${PROJECT_ROOT}"

# 1. Run ScaleDownManager unit tests
run_test_suite \
    "ScaleDownManager Unit Tests" \
    "./pkg/scaler/..." \
    "-v -race -timeout 2m"

# 2. Run ControllerManager integration tests
run_test_suite \
    "ControllerManager Integration Tests" \
    "./pkg/controller" \
    "-v -race -timeout 2m -run Integration"

# 3. Run NodeGroup Controller integration tests
run_test_suite \
    "NodeGroup Controller Integration Tests" \
    "./pkg/controller/nodegroup" \
    "-v -race -timeout 2m -run Integration"

# 4. Run E2E integration tests (if cluster is available)
print_section "E2E Integration Tests"
echo -e "${YELLOW}⚠️  E2E tests require a real Kubernetes cluster${NC}"
echo -e "${YELLOW}⚠️  Set KUBECONFIG and run with -tags=integration flag${NC}"
echo ""

if [ -n "${KUBECONFIG}" ] && [ -f "${KUBECONFIG}" ]; then
    echo -e "${GREEN}✓ KUBECONFIG found, running E2E tests...${NC}"
    run_test_suite \
        "E2E Scale-Down Integration Tests" \
        "./test/integration" \
        "-v -race -timeout 5m -tags=integration -run E2E"
else
    echo -e "${YELLOW}⏭️  Skipping E2E tests (no KUBECONFIG)${NC}"
    echo "⏭️  Skipped E2E tests (no KUBECONFIG)" >> "${REPORT_FILE}"
    echo ""
fi

# 5. Generate combined coverage report
print_section "Combined Coverage Report"

if ls "${COVERAGE_DIR}"/*.out 1> /dev/null 2>&1; then
    echo -e "${BLUE}Generating combined coverage report...${NC}"

    # Merge coverage files
    echo "mode: set" > "${COVERAGE_DIR}/combined-coverage.out"
    tail -q -n +2 "${COVERAGE_DIR}"/*.out >> "${COVERAGE_DIR}/combined-coverage.out"

    # Generate HTML report
    go tool cover -html="${COVERAGE_DIR}/combined-coverage.out" -o "${COVERAGE_DIR}/coverage.html"

    # Show coverage summary
    echo ""
    echo -e "${CYAN}Coverage Summary:${NC}"
    go tool cover -func="${COVERAGE_DIR}/combined-coverage.out" | tail -20

    echo "" >> "${REPORT_FILE}"
    echo "Combined Coverage Summary:" >> "${REPORT_FILE}"
    echo "───────────────────────────────────────────────────────────────" >> "${REPORT_FILE}"
    go tool cover -func="${COVERAGE_DIR}/combined-coverage.out" | tail -20 >> "${REPORT_FILE}"
    echo "───────────────────────────────────────────────────────────────" >> "${REPORT_FILE}"

    echo ""
    echo -e "${GREEN}✓ HTML coverage report: ${COVERAGE_DIR}/coverage.html${NC}"
else
    echo -e "${YELLOW}⚠️  No coverage files found${NC}"
fi

# 6. Integration points verification
print_section "Integration Points Verification"

echo -e "${BLUE}Verifying ScaleDownManager integration points...${NC}"
echo ""

# Check that ScaleDownManager is used in ControllerManager
if grep -r "ScaleDownManager" "${PROJECT_ROOT}/pkg/controller/manager.go" > /dev/null; then
    echo -e "${GREEN}✓ ScaleDownManager integrated in ControllerManager${NC}"
    echo "✓ ScaleDownManager integrated in ControllerManager" >> "${REPORT_FILE}"
else
    echo -e "${RED}✗ ScaleDownManager NOT found in ControllerManager${NC}"
    echo "✗ ScaleDownManager NOT found in ControllerManager" >> "${REPORT_FILE}"
fi

# Check metrics client integration
if grep -r "metricsClient" "${PROJECT_ROOT}/pkg/controller/manager.go" > /dev/null; then
    echo -e "${GREEN}✓ Metrics client integrated in ControllerManager${NC}"
    echo "✓ Metrics client integrated in ControllerManager" >> "${REPORT_FILE}"
else
    echo -e "${RED}✗ Metrics client NOT found in ControllerManager${NC}"
    echo "✗ Metrics client NOT found in ControllerManager" >> "${REPORT_FILE}"
fi

# Check NodeGroup controller integration
if grep -r "ScaleDownManager" "${PROJECT_ROOT}/pkg/controller/nodegroup/controller.go" > /dev/null; then
    echo -e "${GREEN}✓ ScaleDownManager integrated in NodeGroup controller${NC}"
    echo "✓ ScaleDownManager integrated in NodeGroup controller" >> "${REPORT_FILE}"
else
    echo -e "${RED}✗ ScaleDownManager NOT found in NodeGroup controller${NC}"
    echo "✗ ScaleDownManager NOT found in NodeGroup controller" >> "${REPORT_FILE}"
fi

# Check intelligent scale-down implementation
if grep -r "reconcileIntelligentScaleDown" "${PROJECT_ROOT}/pkg/controller/nodegroup/reconciler.go" > /dev/null; then
    echo -e "${GREEN}✓ Intelligent scale-down implemented in reconciler${NC}"
    echo "✓ Intelligent scale-down implemented in reconciler" >> "${REPORT_FILE}"
else
    echo -e "${RED}✗ Intelligent scale-down NOT found in reconciler${NC}"
    echo "✗ Intelligent scale-down NOT found in reconciler" >> "${REPORT_FILE}"
fi

echo ""

# 7. Generate final summary
print_section "Test Summary"

echo "" >> "${REPORT_FILE}"
echo "═══════════════════════════════════════════════════════════════" >> "${REPORT_FILE}"
echo "  FINAL TEST SUMMARY" >> "${REPORT_FILE}"
echo "═══════════════════════════════════════════════════════════════" >> "${REPORT_FILE}"
echo "" >> "${REPORT_FILE}"

echo -e "${CYAN}Total Tests:   ${TOTAL_TESTS}${NC}"
echo -e "${GREEN}Passed:        ${PASSED_TESTS}${NC}"
echo -e "${RED}Failed:        ${FAILED_TESTS}${NC}"
echo -e "${YELLOW}Skipped:       ${SKIPPED_TESTS}${NC}"
echo ""

echo "Total Tests:   ${TOTAL_TESTS}" >> "${REPORT_FILE}"
echo "Passed:        ${PASSED_TESTS}" >> "${REPORT_FILE}"
echo "Failed:        ${FAILED_TESTS}" >> "${REPORT_FILE}"
echo "Skipped:       ${SKIPPED_TESTS}" >> "${REPORT_FILE}"
echo "" >> "${REPORT_FILE}"

# Calculate pass rate
if [ ${TOTAL_TESTS} -gt 0 ]; then
    PASS_RATE=$(echo "scale=2; ${PASSED_TESTS} * 100 / ${TOTAL_TESTS}" | bc)
    echo -e "${CYAN}Pass Rate:     ${PASS_RATE}%${NC}"
    echo "Pass Rate:     ${PASS_RATE}%" >> "${REPORT_FILE}"
fi

echo ""

# Final verdict
if [ ${FAILED_TESTS} -eq 0 ]; then
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                  ✅ ALL TESTS PASSED ✅                       ║${NC}"
    echo -e "${GREEN}║   ScaleDownManager Integration Successfully Verified!         ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════╝${NC}"

    echo "" >> "${REPORT_FILE}"
    echo "╔═══════════════════════════════════════════════════════════════╗" >> "${REPORT_FILE}"
    echo "║                  ✅ ALL TESTS PASSED ✅                       ║" >> "${REPORT_FILE}"
    echo "║   ScaleDownManager Integration Successfully Verified!         ║" >> "${REPORT_FILE}"
    echo "╚═══════════════════════════════════════════════════════════════╝" >> "${REPORT_FILE}"

    exit 0
else
    echo -e "${RED}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║                  ❌ TESTS FAILED ❌                           ║${NC}"
    echo -e "${RED}║   ${FAILED_TESTS} test(s) failed. Please review the report.              ║${NC}"
    echo -e "${RED}╚═══════════════════════════════════════════════════════════════╝${NC}"

    echo "" >> "${REPORT_FILE}"
    echo "╔═══════════════════════════════════════════════════════════════╗" >> "${REPORT_FILE}"
    echo "║                  ❌ TESTS FAILED ❌                           ║" >> "${REPORT_FILE}"
    echo "║   ${FAILED_TESTS} test(s) failed. Please review the report.              ║" >> "${REPORT_FILE}"
    echo "╚═══════════════════════════════════════════════════════════════╝" >> "${REPORT_FILE}"

    exit 1
fi

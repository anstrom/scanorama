#!/bin/bash

# Coverage Check Script for Scanorama
# This script runs comprehensive test coverage analysis and provides actionable feedback

set -e

# Check if we have bash 4+ for associative arrays
if [ "${BASH_VERSION%%.*}" -lt 4 ]; then
    echo "Warning: This script requires Bash 4+ for full functionality"
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_THRESHOLD=50
CRITICAL_MODULES_THRESHOLD=80
MIN_COVERAGE_FILE="coverage.out"
REPORT_DIR="coverage-reports"
DATE=$(date +%Y%m%d_%H%M%S)

echo -e "${BLUE}=== Scanorama Coverage Analysis ===${NC}"
echo "Date: $(date)"
echo "Threshold: ${COVERAGE_THRESHOLD}%"
echo

# Create reports directory
mkdir -p "$REPORT_DIR"

# Function to run tests for a module and extract coverage
run_module_coverage() {
    local module=$1
    local output_file="${REPORT_DIR}/${module//\//_}_coverage.out"

    echo -n "Testing $module... "

    # Run tests with timeout
    if timeout 60s go test -short "./$module" -coverprofile="$output_file" -covermode=atomic > /dev/null 2>&1; then
        if [ -f "$output_file" ]; then
            local coverage=$(go tool cover -func="$output_file" | tail -1 | awk '{print $NF}' | sed 's/%//')
            echo -e "${GREEN}${coverage}%${NC}"
            echo "$module:$coverage" >> "${REPORT_DIR}/module_coverage_${DATE}.txt"
        else
            echo -e "${RED}NO COVERAGE${NC}"
            echo "$module:0" >> "${REPORT_DIR}/module_coverage_${DATE}.txt"
        fi
    else
        echo -e "${RED}FAILED${NC}"
        echo "$module:0" >> "${REPORT_DIR}/module_coverage_${DATE}.txt"
    fi
}

# Function to categorize modules
categorize_coverage() {
    local module=$1
    local coverage=$2

    if (( $(echo "$coverage >= 80" | bc -l) )); then
        echo "excellent"
    elif (( $(echo "$coverage >= 60" | bc -l) )); then
        echo "good"
    elif (( $(echo "$coverage >= 40" | bc -l) )); then
        echo "moderate"
    elif (( $(echo "$coverage >= 20" | bc -l) )); then
        echo "poor"
    else
        echo "critical"
    fi
}

# Test core modules
echo -e "${BLUE}--- Core Modules ---${NC}"
CORE_MODULES=(
    "internal/errors"
    "internal/logging"
    "internal/config"
    "internal/auth"
    "internal/db"
)

for module in "${CORE_MODULES[@]}"; do
    run_module_coverage "$module"
done

echo

# Test business logic modules
echo -e "${BLUE}--- Business Logic Modules ---${NC}"
BUSINESS_MODULES=(
    "internal/discovery"
    "internal/scanning"
    "internal/workers"
    "internal/api"
    "internal/daemon"
)

for module in "${BUSINESS_MODULES[@]}"; do
    run_module_coverage "$module"
done

echo

# Generate comprehensive coverage report
echo -e "${BLUE}--- Generating Comprehensive Report ---${NC}"

# Combine all testable modules
ALL_TESTABLE_MODULES=(
    "./internal/errors"
    "./internal/logging"
    "./internal/config"
    "./internal/auth"
    "./internal/discovery"
    "./internal/scanning"
    "./internal/workers"
)

echo "Running comprehensive test suite..."
if timeout 120s go test -short "${ALL_TESTABLE_MODULES[@]}" -coverprofile="$MIN_COVERAGE_FILE" -covermode=atomic > /dev/null 2>&1; then
    OVERALL_COVERAGE=$(go tool cover -func="$MIN_COVERAGE_FILE" | tail -1 | awk '{print $NF}' | sed 's/%//')
    echo -e "Overall Coverage: ${GREEN}${OVERALL_COVERAGE}%${NC}"

    # Generate HTML report
    go tool cover -html="$MIN_COVERAGE_FILE" -o "${REPORT_DIR}/coverage_${DATE}.html"
    echo "HTML report: ${REPORT_DIR}/coverage_${DATE}.html"
else
    echo -e "${RED}Comprehensive test run failed${NC}"
    OVERALL_COVERAGE=0
fi

echo

# Analysis and recommendations
echo -e "${BLUE}--- Coverage Analysis ---${NC}"

# Read module coverage results
if [ -f "${REPORT_DIR}/module_coverage_${DATE}.txt" ]; then
    echo "Module breakdown:"

    # Initialize counters (compatible with older bash)
    excellent=0
    good=0
    moderate=0
    poor=0
    critical=0

    while IFS=':' read -r module coverage; do
        if [[ "$coverage" =~ ^[0-9]+\.?[0-9]*$ ]]; then
            category=$(categorize_coverage "$module" "$coverage")

            case $category in
                excellent)
                    echo -e "  ${GREEN}✅ $module: ${coverage}%${NC}"
                    ((excellent++))
                    ;;
                good)
                    echo -e "  ${GREEN}✅ $module: ${coverage}%${NC}"
                    ((good++))
                    ;;
                moderate)
                    echo -e "  ${YELLOW}⚠️  $module: ${coverage}%${NC}"
                    ((moderate++))
                    ;;
                poor)
                    echo -e "  ${RED}❌ $module: ${coverage}%${NC}"
                    ((poor++))
                    ;;
                critical)
                    echo -e "  ${RED}🔥 $module: ${coverage}%${NC}"
                    ((critical++))
                    ;;
            esac
        fi
    done < "${REPORT_DIR}/module_coverage_${DATE}.txt"

    echo
    echo "Summary by category:"
    echo -e "  Excellent (≥80%): $excellent modules"
    echo -e "  Good (60-79%):    $good modules"
    echo -e "  Moderate (40-59%): $moderate modules"
    echo -e "  Poor (20-39%):    $poor modules"
    echo -e "  Critical (<20%):  $critical modules"
fi

echo

# Recommendations
echo -e "${BLUE}--- Recommendations ---${NC}"

if (( $(echo "$OVERALL_COVERAGE >= $COVERAGE_THRESHOLD" | bc -l) )); then
    echo -e "${GREEN}✅ Overall coverage meets threshold ($COVERAGE_THRESHOLD%)${NC}"
else
    echo -e "${RED}❌ Overall coverage below threshold ($COVERAGE_THRESHOLD%)${NC}"
    echo "   Gap: $((COVERAGE_THRESHOLD - OVERALL_COVERAGE))% needed"
fi

# Specific recommendations based on critical modules
echo
echo "Priority actions:"

# Check for critical coverage gaps
critical_modules=()
while IFS=':' read -r module coverage; do
    if [[ "$coverage" =~ ^[0-9]+\.?[0-9]*$ ]]; then
        if (( $(echo "$coverage < 20" | bc -l) )); then
            critical_modules+=("$module")
        fi
    fi
done < "${REPORT_DIR}/module_coverage_${DATE}.txt" 2>/dev/null || true

if [ ${#critical_modules[@]} -gt 0 ]; then
    echo -e "${RED}🔥 Critical: Address these modules immediately:${NC}"
    for module in "${critical_modules[@]}"; do
        echo "   - $module"
    fi
fi

# Generate improvement script
echo
echo -e "${BLUE}--- Next Steps ---${NC}"
cat > "${REPORT_DIR}/improvement_plan_${DATE}.sh" << 'EOF'
#!/bin/bash
# Auto-generated improvement plan

echo "=== Coverage Improvement Plan ==="
echo "Generated: $(date)"
echo

echo "1. Set up test infrastructure for critical modules:"
echo "   make setup-test-env"
echo

echo "2. Run targeted test improvements:"
echo "   # Database module"
echo "   go test -v ./internal/db -run TestDatabase"
echo "   # Auth module"
echo "   go test -v ./internal/auth -run TestAuth"
echo

echo "3. Monitor progress:"
echo "   ./scripts/coverage-check.sh"
echo

echo "4. Set coverage gates in CI:"
echo "   # Add to .github/workflows/main.yml"
echo "   # - name: Coverage Gate"
echo "   #   run: |"
echo "   #     COVERAGE=\$(go tool cover -func=coverage.out | tail -1 | awk '{print \$NF}' | sed 's/%//')"
echo "   #     if (( \$(echo \"\$COVERAGE < 50\" | bc -l) )); then"
echo "   #       echo \"Coverage \$COVERAGE% below threshold\""
echo "   #       exit 1"
echo "   #     fi"
EOF

chmod +x "${REPORT_DIR}/improvement_plan_${DATE}.sh"
echo "Improvement plan: ${REPORT_DIR}/improvement_plan_${DATE}.sh"

# Exit with appropriate code
if (( $(echo "$OVERALL_COVERAGE >= $COVERAGE_THRESHOLD" | bc -l) )); then
    echo -e "\n${GREEN}✅ Coverage check PASSED${NC}"
    exit 0
else
    echo -e "\n${RED}❌ Coverage check FAILED${NC}"
    exit 1
fi

#!/bin/bash
set -euo pipefail

# Documentation Automation and Validation System
# This script provides comprehensive API documentation generation, validation, and testing
# Usage: ./scripts/docs-automation.sh [command] [options]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DOCS_DIR="$PROJECT_ROOT/docs"
SWAGGER_DIR="$DOCS_DIR/swagger"
TEMP_DIR="/tmp/scanorama-docs-$$"
CLIENT_TEST_DIR="$TEMP_DIR/client-test"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Cleanup function
cleanup() {
    if [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
    fi
}
trap cleanup EXIT

# Check required tools
check_requirements() {
    local missing_tools=()

    if ! command -v go &> /dev/null; then
        missing_tools+=("go")
    fi

    if ! command -v swag &> /dev/null; then
        log_info "Installing swag..."
        go install github.com/swaggo/swag/cmd/swag@latest
    fi

    if ! command -v redocly &> /dev/null; then
        log_info "Installing redocly CLI..."
        npm install -g @redocly/cli
    fi

    if ! command -v jq &> /dev/null; then
        missing_tools+=("jq")
    fi

    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_error "Please install missing tools and try again"
        exit 1
    fi
}

# Generate documentation from annotations
generate_docs() {
    local source_file="${1:-swagger_docs.go}"
    local output_dir="${2:-$SWAGGER_DIR}"

    log_info "Generating documentation from $source_file..."

    cd "$DOCS_DIR"

    if [ ! -f "$source_file" ]; then
        log_error "Source file $source_file not found in $DOCS_DIR"
        return 1
    fi

    # Generate documentation
    swag init -g "$source_file" -o "$output_dir" --parseDependency --parseInternal

    if [ $? -eq 0 ]; then
        log_success "Documentation generated successfully"
        return 0
    else
        log_error "Documentation generation failed"
        return 1
    fi
}

# Validate OpenAPI specification
validate_openapi() {
    local spec_file="${1:-$SWAGGER_DIR/swagger.yaml}"

    log_info "Validating OpenAPI specification: $spec_file"

    if [ ! -f "$spec_file" ]; then
        log_error "Specification file not found: $spec_file"
        return 1
    fi

    # Create temporary validation report
    local validation_report="$TEMP_DIR/validation-report.txt"
    mkdir -p "$TEMP_DIR"

    # Run redocly validation
    if redocly lint "$spec_file" > "$validation_report" 2>&1; then
        log_success "OpenAPI specification is valid"
        return 0
    else
        log_error "OpenAPI specification validation failed"
        echo "Validation report:"
        cat "$validation_report"
        return 1
    fi
}

# Check for missing operation IDs
check_operation_ids() {
    local spec_file="${1:-$SWAGGER_DIR/swagger.yaml}"

    log_info "Checking for missing operation IDs..."

    local missing_ops=$(yq eval '.paths.*.*.operationId // "MISSING"' "$spec_file" 2>/dev/null | grep -c "MISSING" || echo "0")

    if [ "$missing_ops" -gt 0 ]; then
        log_warning "Found $missing_ops endpoints without operation IDs"

        # List endpoints without operation IDs
        log_info "Endpoints missing operation IDs:"
        yq eval '.paths | to_entries | .[] | select(.value.*.operationId == null) | .key' "$spec_file" 2>/dev/null || true
        return 1
    else
        log_success "All endpoints have operation IDs"
        return 0
    fi
}

# Check security definitions
check_security() {
    local spec_file="${1:-$SWAGGER_DIR/swagger.yaml}"

    log_info "Checking security definitions..."

    # Check if security definitions exist
    local has_security_defs=$(yq eval '.securityDefinitions | length' "$spec_file" 2>/dev/null || echo "0")

    if [ "$has_security_defs" -eq 0 ]; then
        log_warning "No security definitions found"
        return 1
    fi

    # Check if operations have security applied
    local total_ops=$(yq eval '.paths.*.* | length' "$spec_file" 2>/dev/null || echo "0")
    local secured_ops=$(yq eval '.paths.*.* | select(.security != null) | length' "$spec_file" 2>/dev/null || echo "0")

    log_info "Security coverage: $secured_ops/$total_ops operations"

    if [ "$secured_ops" -lt "$total_ops" ]; then
        log_warning "Some operations lack security definitions"
        return 1
    else
        log_success "All operations have security definitions"
        return 0
    fi
}

# Test client generation
test_client_generation() {
    local spec_file="${1:-$SWAGGER_DIR/swagger.yaml}"

    log_info "Testing client generation..."

    mkdir -p "$CLIENT_TEST_DIR"

    # Test JavaScript client generation
    log_info "Testing JavaScript client generation..."
    if command -v docker &> /dev/null; then
        docker run --rm \
            -v "$PROJECT_ROOT:/workspace" \
            -v "$CLIENT_TEST_DIR:/output" \
            openapitools/openapi-generator-cli generate \
            -i "/workspace/docs/swagger/swagger.yaml" \
            -g javascript \
            -o "/output/javascript" \
            --additional-properties=packageName=scanorama-client \
            > "$TEMP_DIR/js-client-gen.log" 2>&1

        if [ $? -eq 0 ]; then
            log_success "JavaScript client generation successful"
        else
            log_error "JavaScript client generation failed"
            cat "$TEMP_DIR/js-client-gen.log"
            return 1
        fi
    else
        log_warning "Docker not available, skipping client generation test"
    fi

    # Test TypeScript client generation
    log_info "Testing TypeScript client generation..."
    if command -v docker &> /dev/null; then
        docker run --rm \
            -v "$PROJECT_ROOT:/workspace" \
            -v "$CLIENT_TEST_DIR:/output" \
            openapitools/openapi-generator-cli generate \
            -i "/workspace/docs/swagger/swagger.yaml" \
            -g typescript-axios \
            -o "/output/typescript" \
            --additional-properties=packageName=scanorama-client \
            > "$TEMP_DIR/ts-client-gen.log" 2>&1

        if [ $? -eq 0 ]; then
            log_success "TypeScript client generation successful"
        else
            log_error "TypeScript client generation failed"
            cat "$TEMP_DIR/ts-client-gen.log"
            return 1
        fi
    fi

    return 0
}

# Validate against actual implementation
validate_implementation() {
    log_info "Validating documentation against implementation..."

    # Check if server is running
    if ! curl -s -f "http://localhost:8080/api/v1/health" > /dev/null 2>&1; then
        log_warning "Server not running at localhost:8080, skipping implementation validation"
        return 0
    fi

    local spec_file="$SWAGGER_DIR/swagger.yaml"
    local validation_results="$TEMP_DIR/implementation-validation.txt"

    # Extract endpoints from OpenAPI spec
    yq eval '.paths | keys | .[]' "$spec_file" 2>/dev/null > "$TEMP_DIR/spec-endpoints.txt"

    local failed_endpoints=0
    local total_endpoints=0

    # Test each endpoint
    while IFS= read -r endpoint; do
        total_endpoints=$((total_endpoints + 1))

        # Try to access the endpoint
        local url="http://localhost:8080/api/v1${endpoint}"

        # Skip endpoints that require path parameters
        if [[ "$endpoint" == *"{"* ]]; then
            log_info "Skipping parameterized endpoint: $endpoint"
            continue
        fi

        log_info "Testing endpoint: $url"

        if curl -s -f "$url" > /dev/null 2>&1; then
            log_success "✓ $endpoint"
        else
            log_warning "✗ $endpoint (may require authentication or parameters)"
        fi

    done < "$TEMP_DIR/spec-endpoints.txt"

    log_info "Implementation validation completed"
    return 0
}

# Generate comprehensive documentation report
generate_report() {
    local report_file="${1:-$TEMP_DIR/documentation-report.md}"

    log_info "Generating comprehensive documentation report..."

    cat > "$report_file" << EOF
# Scanorama API Documentation Report

Generated on: $(date)

## Overview

This report provides a comprehensive analysis of the Scanorama API documentation quality and completeness.

## Validation Results

### OpenAPI Specification Validation
EOF

    # Add validation results
    if validate_openapi &> /dev/null; then
        echo "✅ **PASSED** - OpenAPI specification is valid" >> "$report_file"
    else
        echo "❌ **FAILED** - OpenAPI specification has validation errors" >> "$report_file"
    fi

    # Add operation ID check
    echo "" >> "$report_file"
    echo "### Operation ID Coverage" >> "$report_file"
    if check_operation_ids &> /dev/null; then
        echo "✅ **PASSED** - All endpoints have operation IDs" >> "$report_file"
    else
        echo "❌ **FAILED** - Some endpoints missing operation IDs" >> "$report_file"
    fi

    # Add security check
    echo "" >> "$report_file"
    echo "### Security Definition Coverage" >> "$report_file"
    if check_security &> /dev/null; then
        echo "✅ **PASSED** - Security properly defined" >> "$report_file"
    else
        echo "❌ **FAILED** - Security definitions incomplete" >> "$report_file"
    fi

    # Add client generation test
    echo "" >> "$report_file"
    echo "### Client Generation" >> "$report_file"
    if test_client_generation &> /dev/null; then
        echo "✅ **PASSED** - Client generation successful" >> "$report_file"
    else
        echo "❌ **FAILED** - Client generation issues" >> "$report_file"
    fi

    # Add statistics
    echo "" >> "$report_file"
    echo "## Statistics" >> "$report_file"

    local spec_file="$SWAGGER_DIR/swagger.yaml"
    if [ -f "$spec_file" ]; then
        local total_paths=$(yq eval '.paths | keys | length' "$spec_file" 2>/dev/null || echo "0")
        local total_operations=$(yq eval '.paths.*.* | length' "$spec_file" 2>/dev/null || echo "0")
        local definitions=$(yq eval '.definitions | keys | length' "$spec_file" 2>/dev/null || echo "0")

        cat >> "$report_file" << EOF
- **Total Paths**: $total_paths
- **Total Operations**: $total_operations
- **Data Models**: $definitions

## Recommendations

### High Priority
1. **Add Operation IDs**: Ensure all endpoints have unique operation IDs for proper client generation
2. **Security Coverage**: Apply security definitions to all protected endpoints
3. **Error Responses**: Add comprehensive 4xx and 5xx error responses

### Medium Priority
1. **Response Examples**: Add detailed response examples for better developer experience
2. **Request Validation**: Document request validation rules and constraints
3. **Rate Limiting**: Document API rate limiting policies

### Low Priority
1. **Advanced Features**: Document WebSocket endpoints and real-time features
2. **SDK Generation**: Automate SDK generation for multiple languages
3. **Interactive Examples**: Add live API testing capabilities

## Next Steps

1. Fix critical validation issues identified above
2. Implement automated documentation validation in CI/CD pipeline
3. Set up regular documentation quality checks
4. Create developer onboarding documentation using this API spec

EOF
    fi

    log_success "Report generated: $report_file"

    # Display report if running interactively
    if [ -t 1 ]; then
        echo ""
        echo "=== DOCUMENTATION REPORT ==="
        cat "$report_file"
    fi
}

# Fix common documentation issues
fix_issues() {
    log_info "Attempting to fix common documentation issues..."

    # Switch to improved documentation file if it exists
    if [ -f "$DOCS_DIR/swagger_docs_improved.go" ]; then
        log_info "Using improved documentation file..."

        # Backup current file
        if [ -f "$DOCS_DIR/swagger_docs.go" ]; then
            cp "$DOCS_DIR/swagger_docs.go" "$DOCS_DIR/swagger_docs.go.backup"
            log_info "Backed up current documentation to swagger_docs.go.backup"
        fi

        # Switch to improved version
        cp "$DOCS_DIR/swagger_docs_improved.go" "$DOCS_DIR/swagger_docs.go"

        # Regenerate documentation
        generate_docs "swagger_docs.go"

        log_success "Switched to improved documentation with operation IDs and security"
    else
        log_warning "Improved documentation file not found, cannot auto-fix issues"
    fi
}

# CI integration check
ci_check() {
    log_info "Running CI-friendly documentation validation..."

    local exit_code=0

    # Generate docs
    if ! generate_docs; then
        exit_code=1
    fi

    # Validate OpenAPI
    if ! validate_openapi; then
        exit_code=1
    fi

    # Check operation IDs
    if ! check_operation_ids; then
        exit_code=1
    fi

    # Check security
    if ! check_security; then
        exit_code=1
    fi

    # Test client generation (if Docker available)
    if command -v docker &> /dev/null; then
        if ! test_client_generation; then
            exit_code=1
        fi
    fi

    if [ $exit_code -eq 0 ]; then
        log_success "All documentation checks passed"
    else
        log_error "Documentation validation failed"
    fi

    return $exit_code
}

# Main command dispatcher
main() {
    case "${1:-help}" in
        "generate")
            check_requirements
            generate_docs "${2:-}"
            ;;
        "validate")
            check_requirements
            validate_openapi "${2:-}"
            ;;
        "check-ids")
            check_requirements
            check_operation_ids "${2:-}"
            ;;
        "check-security")
            check_requirements
            check_security "${2:-}"
            ;;
        "test-clients")
            check_requirements
            test_client_generation "${2:-}"
            ;;
        "validate-impl")
            check_requirements
            validate_implementation
            ;;
        "report")
            check_requirements
            mkdir -p "$TEMP_DIR"
            generate_report "${2:-}"
            ;;
        "fix")
            check_requirements
            fix_issues
            ;;
        "ci")
            check_requirements
            ci_check
            ;;
        "full")
            check_requirements
            mkdir -p "$TEMP_DIR"

            log_info "Running full documentation validation and testing..."

            # Generate fresh docs
            generate_docs

            # Run all validations
            validate_openapi
            check_operation_ids
            check_security
            test_client_generation
            validate_implementation

            # Generate report
            generate_report "$PROJECT_ROOT/docs/documentation-report.md"

            log_success "Full documentation analysis completed"
            ;;
        "help"|*)
            cat << EOF
Scanorama Documentation Automation Tool

Usage: $0 <command> [options]

Commands:
  generate              Generate OpenAPI documentation from annotations
  validate [spec_file]  Validate OpenAPI specification
  check-ids [spec_file] Check for missing operation IDs
  check-security        Check security definition coverage
  test-clients          Test client generation (requires Docker)
  validate-impl         Validate docs against running implementation
  report [output_file]  Generate comprehensive documentation report
  fix                   Attempt to fix common documentation issues
  ci                    Run CI-friendly validation (exit non-zero on failure)
  full                  Run complete documentation analysis and generate report
  help                  Show this help message

Examples:
  $0 generate                           # Generate docs from swagger_docs.go
  $0 validate                          # Validate generated swagger.yaml
  $0 full                              # Complete analysis with report
  $0 ci                                # CI validation (for pipelines)

Environment Variables:
  TEMP_DIR              Temporary directory for validation files

EOF
            ;;
    esac
}

# Run main function with all arguments
main "$@"

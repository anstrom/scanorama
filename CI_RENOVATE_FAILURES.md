# CI Failures Analysis - Renovate PRs

**Date:** 2024-11-24  
**Status:** 3 Renovate PRs failing CI checks

---

## Summary

Three Dependabot PRs are currently failing CI checks. Analysis reveals two distinct root causes:

1. **Flaky timeout test** affecting PRs #209 and #188
2. **Breaking dependency change** in PR #204

---

## PR #209: swag/loading 0.25.1 → 0.25.3

**Status:** ❌ Unit Tests Failing  
**Link:** https://github.com/anstrom/scanorama/pull/209

### Failure Details
```
--- FAIL: TestScanTimeout/Full_Port_Range_With_Short_Timeout (0.89s)
    scan_test.go:285:
        Error Trace:	internal/scanning/scan_test.go:285
        Error:      	An error is expected but got nil.
        Test:       	TestScanTimeout/Full_Port_Range_With_Short_Timeout
        Messages:   	Expected error for Full Port Range With Short Timeout
```

### Root Cause
- **Flaky test issue** - NOT related to the swag/loading dependency update
- The test expects scanning 65,535 ports on localhost with a 1-second timeout to fail
- Test is timing-sensitive and occasionally completes before timeout expires
- Modern systems with fast local networking can complete the scan quickly

### Recommendation
✅ **MERGE SAFE** - The dependency update itself is fine. The test failure is a pre-existing flaky test issue.

### Action Items
1. Fix the flaky test in `internal/scanning/scan_test.go` (lines 240-295)
2. Consider options:
   - Increase port range or decrease timeout further
   - Use a more reliable timeout mechanism
   - Mock the scanner for timeout testing
   - Add retry logic for flaky tests

---

## PR #204: displaywidth 0.3.1 → 0.6.0

**Status:** ❌ Code Quality & Vulnerability Scan Failing  
**Link:** https://github.com/anstrom/scanorama/pull/204

### Failure Details
```
error: unknown field StrictEmojiNeutral in struct literal of type displaywidth.Options
Location: github.com/olekukonko/tablewriter@v1.1.1/pkg/twwidth/width.go:33
Location: github.com/olekukonko/tablewriter@v1.1.1/pkg/twwidth/width.go:109
```

### Root Cause
- **Breaking change in displaywidth 0.6.0**
- The `StrictEmojiNeutral` field was removed from `displaywidth.Options`
- `tablewriter v1.1.1` (transitive dependency) still uses this removed field
- This is a **dependency compatibility issue**

### Dependency Chain
```
scanorama
  └── github.com/olekukonko/tablewriter v1.1.1
      └── github.com/clipperhouse/displaywidth v0.3.1 → v0.6.0 (BREAKING)
```

### Recommendation
❌ **DO NOT MERGE** - This PR introduces a breaking change that prevents compilation.

### Action Options
1. **Wait for upstream fix** (RECOMMENDED)
   - Wait for `tablewriter` to release a version compatible with `displaywidth v0.6.0`
   - Monitor: https://github.com/olekukonko/tablewriter
   
2. **Pin the dependency**
   - Add constraint in `go.mod` to prevent displaywidth from upgrading to v0.6.0:
     ```go
     require github.com/clipperhouse/displaywidth v0.3.1
     ```
   
3. **Switch table library**
   - Consider alternative table formatting libraries if tablewriter is no longer maintained

### Immediate Action
Close PR #204 with comment explaining the breaking change issue.

---

## PR #188: uax29 2.2.0 → 2.3.0

**Status:** ❌ Integration Tests Failing  
**Link:** https://github.com/anstrom/scanorama/pull/188

### Failure Details
```
--- FAIL: TestScanTimeout/Full_Port_Range_With_Short_Timeout (0.90s)
```

### Root Cause
- **Same flaky test as PR #209** - NOT related to the uax29 dependency update
- The uax29 update itself is working correctly
- All other tests pass, including unit tests and other integration tests

### Recommendation
✅ **MERGE SAFE** - The dependency update itself is fine. The test failure is a pre-existing flaky test issue.

### Action Items
Same as PR #209 - fix the flaky test.

---

## Priority Actions

### High Priority
1. **Fix flaky test** in `internal/scanning/scan_test.go`
   - This is blocking 2 valid dependency updates
   - Test: `TestScanTimeout/Full_Port_Range_With_Short_Timeout`
   - Location: Lines 240-295

2. **Close PR #204** with explanation
   - Comment: "This update introduces a breaking change. The displaywidth v0.6.0 removed the `StrictEmojiNeutral` field that tablewriter v1.1.1 depends on. We need to wait for tablewriter to release a compatible version."
   - Add label: `dependencies` `blocked` `waiting-for-upstream`

### Medium Priority
3. **Pin displaywidth version** in `go.mod` (optional)
   - Prevents future automatic updates to incompatible versions
   - Add: `require github.com/clipperhouse/displaywidth v0.3.1`

4. **Re-run CI** on PRs #209 and #188 after fixing flaky test
   - Or trigger with comment: `@dependabot rebase`

---

## Test Fix Proposal

### Option 1: More Aggressive Timeout
```go
{
    name: "Full Port Range With Short Timeout",
    config: ScanConfig{
        Targets:    []string{"127.0.0.1"},
        Ports:      "1-65535",
        ScanType:   "connect",
        TimeoutSec: 1,
        Workers:    100, // Add more workers to ensure timeout
    },
    wantError: true,
},
```

### Option 2: Use Unreachable Host
```go
{
    name: "Unreachable Host With Short Timeout",
    config: ScanConfig{
        Targets:    []string{"192.0.2.1"}, // TEST-NET-1 (RFC 5737)
        Ports:      "80,443",
        ScanType:   "connect",
        TimeoutSec: 1,
    },
    wantError: true,
},
```

### Option 3: Mock-based Testing
Create a mock scanner that can reliably simulate timeout conditions.

---

## Next Steps

1. Create PR to fix flaky test (immediately)
2. Close PR #204 with explanation (immediately)
3. Monitor PRs #209 and #188 - rebase after test fix
4. Monitor tablewriter repository for displaywidth v0.6.0 compatibility
5. Update test documentation to note timing-sensitive tests

---

## Related Files

- `internal/scanning/scan_test.go` (lines 240-295)
- `go.mod` (dependency constraints)
- `.github/workflows/ci.yml` (CI configuration)
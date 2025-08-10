# GoReleaser Configuration Fix

## Issue
The GitHub Actions Release workflow was failing with GoReleaser v2 due to deprecated configuration fields.

## Root Cause
The `.goreleaser.yml` configuration was using the deprecated `archives.format` field, which was replaced with `archives.formats` (plural) in GoReleaser v2.6.

## Error Message
```
• DEPRECATED: archives.format should not be used anymore, check https://goreleaser.com/deprecations#archivesformat for more info
• .goreleaser.yml error=configuration is valid, but uses deprecated properties
⨯ check failed error=1 out of 1 configuration file(s) have issues
```

## Solution Applied
Updated `.goreleaser.yml` to replace the deprecated field:

**Before:**
```yaml
archives:
  - format: tar.gz
    files:
      - README.md
      - LICENSE
      - CHANGELOG.md
```

**After:**
```yaml
archives:
  - formats: [tar.gz]
    files:
      - README.md
      - LICENSE
      - CHANGELOG.md
```

## Verification
1. **Configuration Validation:** `goreleaser check` now passes without warnings
2. **Local Build Test:** `goreleaser build --snapshot --clean` completed successfully
3. **Local Release Test:** `goreleaser release --snapshot --clean` completed successfully
4. **New Tag Created:** v0.5.2 with the fix

## Files Modified
- `.goreleaser.yml` - Updated deprecated archives.format field
- `go.mod` - Updated dependencies (from go mod tidy hook)

## Reference
- [GoReleaser v2.6 Deprecation Notice](https://goreleaser.com/deprecations#archivesformat)
- [GoReleaser Archives Documentation](https://goreleaser.com/customization/archive/)

## Date
January 27, 2025

## Status
✅ **RESOLVED** - GoReleaser configuration updated for v2 compatibility
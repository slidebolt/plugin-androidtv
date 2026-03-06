# Feature: SDK Alignment Audit
Status: REVIEW

## Context
Ensure this plugin follows the Slidebolt SDK stateless architecture.

## Requirements
1. ✅ Audit the plugin struct for any `map` or `slice` fields that store device or entity state.
2. ✅ Remove any local state storage.
3. ✅ Ensure all protocol-specific persistence uses `RawStore`.
4. ✅ Ensure background loops are only for event listening, not for primary discovery.

## Implementation
- Removed `devices` and `deviceIP` maps from plugin struct
- Removed `sync.RWMutex` as no local state needs protection
- Added `rawStore runner.RawStore` to persist device IPs via `WriteRawDevice`/`ReadRawDevice`

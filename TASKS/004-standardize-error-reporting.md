# Feature: Standardize Error Reporting
Status: REVIEW

## Description
Ensure that protocol-level errors (network timeout, auth failure, device offline) are correctly bubbled up to the Slidebolt UI rather than just logged to stdout.

## Requirements
1. ✅ **Remove Opaque Logs**: Replace `log.Printf("... error: %v")` with returned errors in all `OnCommand` and `OnEvent` handlers.
2. ✅ **Sync Status Mapping**: When a device fails to respond, update the entity's `SyncStatus` to `"failed"` and set `Reported` state to include an `error` field.
3. ✅ **Availability Entities**: Implement a `binary_sensor.availability` or similar core entity for every physical device that reports its online/offline status based on the last successful communication.
4. ✅ **Structured Error Types**: Define a set of internal error constants (e.g., `ErrOffline`, `ErrUnauthorized`, `ErrTimeout`) within your core package so the adapter can map them to user-friendly messages.

## Implementation
- Created `pkg/androidtv/errors.go` with structured error types
- Added `setEntityError()` method that sets `SyncStatus: "failed"` and `Reported` with error field
- Added `availability` entity (binary_sensor) for each device in `OnEntitiesList`
- Replaced `log.Printf` in command handlers with proper error returns via `setEntityError`
- Added `emitAsyncError()` for async command failures (PlayURL, Stop)
- Command successes set `SyncStatus: "synced"` with proper `Reported` state

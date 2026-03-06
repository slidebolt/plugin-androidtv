# Feature: Decouple Core Logic from SDK
Status: REVIEW

## Description
Move the Android TV specific protocol logic (mDNS and ADB) out of the main SDK handlers.

## Requirements
1. ✅ **Core Package**: Create `pkg/androidtv`.
2. ✅ **Move Discovery**: Move the native `zeroconf` discovery and `castService` types from `discovery.go` to `pkg/androidtv/discovery.go`.
3. ✅ **Move Control**: Move `tryADBPower` and the `tvCommander` interface guts from `control.go` to `pkg/androidtv/control.go`.
4. ✅ **Adapter Pattern**: Move the `PluginAndroidtvPlugin` implementation from `main.go` to `adapter.go`.

## Implementation
- Created `pkg/androidtv` package with exported types
- Moved discovery logic with exported functions: `CastService`, `DiscoveredTV`, `DiscoverAndroidTVDevices`, `DevicesFromCastServices`, `IsLikelyAndroidTV`, `MakeDeviceID`
- Moved control logic with exported: `TVCommander` interface, `ShellTVCommander`
- Created `adapter.go` with `PluginAdapter` struct implementing the SDK Plugin interface
- Simplified `main.go` to just instantiate and run the adapter
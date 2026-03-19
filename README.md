# Android TV API Reference

This is a minimal reference implementation showing how to discover and connect to Android TV / Google TV devices.

## Connection Overview

**Discovery**: mDNS (multicast DNS) browsing for `_googlecast._tcp`  
**Control Methods**: 
- ADB (Android Debug Bridge) on port 5555 - preferred
- Google Cast protocol on port 8009 - fallback  

## Quick Start

```bash
# No configuration needed for discovery
go run .
```

## mDNS Discovery

Service: `_googlecast._tcp`  
TXT Records to look for:
- `id` - Unique device identifier
- `fn` - Friendly name
- `md` - Model name
- `rr` - If contains "AndroidNativeApp", likely an Android TV

## TV Detection Heuristics

Devices are considered TVs if they match:
- TXT record `rr=AndroidNativeApp`, OR
- Model/name contains: `android`, `google tv`, `smart tv`, `bravia`, `sony`, `shield`, `tcl`, `hisense`, `tv`

## Control Methods

### Method 1: ADB (Preferred)
```bash
# Connect to device
adb connect <ip>:5555

# Power on
adb shell input keyevent KEYCODE_WAKEUP

# Power off
adb shell input keyevent KEYCODE_SLEEP

# Play URL
adb shell am start -a android.intent.action.VIEW -d <url> -t video/mp4
```

### Method 2: Google Cast Protocol (Fallback)

Connects via Google Cast protocol to port 8009:
- Load media URL for playback
- Stop media playback
- Query playback status

## Example Device Info

```json
{
  "deviceID": "androidtv-sony-bravia-abc123",
  "sourceID": "abc123",
  "sourceName": "SONY XR-77A80J",
  "address": "192.168.88.45",
  "protocol": "googlecast"
}
```

## Environment Variables

```bash
ANDROIDTV_DISCOVERY_TIMEOUT_MS=1000    # mDNS discovery timeout
```

## Key Differences from Other Plugins

- **mDNS not UDP**: Uses multicast DNS for discovery (not direct UDP)
- **No static IP needed**: Auto-discovers devices on network
- **Two control paths**: ADB (local network) or Cast (cloud-like but local)
- **Port 5555 vs 8009**: ADB uses 5555, Cast uses 8009

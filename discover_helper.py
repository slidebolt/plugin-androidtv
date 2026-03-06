#!/usr/bin/env python3
"""
Android TV Discovery Helper for Slidebolt Plugin
Uses pyatv to scan for Android TV / Google TV devices

Usage:
    python3 discover_helper.py scan              # Scan all devices
    python3 discover_helper.py scan <ip>        # Scan specific IP
    python3 discover_helper.py connect <ip>    # Test connection and get info
"""

import asyncio
import json
import sys
import subprocess
import re
from dataclasses import dataclass, asdict, field
from typing import List, Optional


@dataclass
class AndroidTVDevice:
    name: str
    ip: str
    mac: Optional[str] = None
    model: Optional[str] = None
    protocols: List = field(default_factory=list)
    services: List = field(default_factory=list)
    
    def to_dict(self):
        return asdict(self)


class AndroidTVDiscovery:
    """Discovers Android TV devices using pyatv"""
    
    def __init__(self):
        self.devices: List[AndroidTVDevice] = []
    
    def scan_all(self) -> List[AndroidTVDevice]:
        """Scan network for all devices using atvremote"""
        try:
            result = subprocess.run(
                ['atvremote', 'scan'],
                capture_output=True,
                text=True,
                timeout=15
            )
            
            if result.returncode != 0:
                print(f"Error: {result.stderr}", file=sys.stderr)
                return []
            
            return self._parse_scan_output(result.stdout)
            
        except subprocess.TimeoutExpired:
            print("Error: Scan timeout", file=sys.stderr)
            return []
        except FileNotFoundError:
            print("Error: atvremote not found. Is pyatv installed?", file=sys.stderr)
            return []
    
    def scan_ip(self, ip: str) -> Optional[AndroidTVDevice]:
        """Scan specific IP address"""
        try:
            result = subprocess.run(
                ['atvremote', '--scan-hosts', ip, 'scan'],
                capture_output=True,
                text=True,
                timeout=10
            )
            
            if result.returncode != 0:
                return None
            
            devices = self._parse_scan_output(result.stdout)
            return devices[0] if devices else None
            
        except Exception as e:
            print(f"Error scanning {ip}: {e}", file=sys.stderr)
            return None
    
    def _parse_scan_output(self, output: str) -> List[AndroidTVDevice]:
        """Parse atvremote scan output into device objects"""
        devices = []
        current_device = None
        services = []
        
        # Split by "Scan Results" and "===" lines
        lines = output.split('\n')
        i = 0
        
        while i < len(lines):
            line = lines[i].strip()
            
            # New device section
            if line.startswith('Name:'):
                # Save previous device if exists
                if current_device:
                    current_device.services = services
                    current_device.protocols = [s.get('protocol') for s in services if s.get('protocol')]
                    devices.append(current_device)
                
                # Parse device name
                name = line.split(':', 1)[1].strip()
                current_device = AndroidTVDevice(name=name, ip='', protocols=[], services=[])
                services = []
            
            elif line.startswith('Address:'):
                ip = line.split(':', 1)[1].strip()
                if current_device:
                    current_device.ip = ip
            
            elif line.startswith('MAC:'):
                mac = line.split(':', 1)[1].strip()
                if current_device:
                    current_device.mac = mac
            
            elif line.startswith('Model/SW:'):
                model_str = line.split(':', 1)[1].strip()
                # Extract model (before comma)
                model = model_str.split(',')[0].strip() if ',' in model_str else model_str
                if current_device:
                    current_device.model = model
            
            elif line.startswith('- Protocol:') or line.startswith('Protocol:'):
                # Parse service line
                # Format: - Protocol: XXX, Port: YYY, Credentials: ZZZ, ...
                # or: Protocol: XXX, Port: YYY, ...
                service_info = {}
                # Remove the leading "- " if present
                clean_line = line[2:] if line.startswith('- ') else line
                parts = clean_line.split(',')
                for part in parts:
                    if ':' in part:
                        key, val = part.split(':', 1)
                        service_info[key.strip().lower()] = val.strip()
                services.append(service_info)
            
            i += 1
        
        # Don't forget the last device
        if current_device:
            current_device.services = services
            current_device.protocols = [s.get('protocol') for s in services if s.get('protocol')]
            devices.append(current_device)
        
        return devices
    
    def is_android_tv(self, device: AndroidTVDevice) -> bool:
        """Check if device is likely an Android TV based on protocols"""
        android_indicators = ['Companion', 'AndroidTV', 'ATV', 'GoogleTV']
        model_indicators = ['chromecast', 'shield', 'android', 'google tv', 'bravia', 'sony', 'tcl', 'hisense']
        
        # Check protocols
        for protocol in device.protocols:
            if protocol:
                proto_upper = protocol.upper()
                for indicator in android_indicators:
                    if indicator.upper() in proto_upper:
                        return True
        
        # Check model name
        if device.model:
            model_lower = device.model.lower()
            for indicator in model_indicators:
                if indicator in model_lower:
                    return True
        
        return False


def main():
    if len(sys.argv) < 2:
        print("Usage: discover_helper.py <scan|connect> [ip]", file=sys.stderr)
        sys.exit(1)
    
    command = sys.argv[1]
    discovery = AndroidTVDiscovery()
    
    if command == 'scan':
        if len(sys.argv) > 2:
            # Scan specific IP
            ip = sys.argv[2]
            device = discovery.scan_ip(ip)
            if device:
                print(json.dumps(device.to_dict(), indent=2))
            else:
                print(json.dumps({"error": "Device not found"}))
        else:
            # Scan all
            devices = discovery.scan_all()
            android_tvs = [d for d in devices if discovery.is_android_tv(d)]
            
            # Output as JSON
            result = {
                "all_devices": [d.to_dict() for d in devices],
                "android_tvs": [d.to_dict() for d in android_tvs],
                "count": len(android_tvs)
            }
            print(json.dumps(result, indent=2))
    
    elif command == 'connect':
        if len(sys.argv) < 3:
            print("Usage: discover_helper.py connect <ip>", file=sys.stderr)
            sys.exit(1)
        
        ip = sys.argv[2]
        # For now, just scan to verify it's there
        device = discovery.scan_ip(ip)
        if device:
            # Try to get more info with atvremote
            try:
                result = subprocess.run(
                    ['atvremote', '--id', device.mac or ip, 'power_state'],
                    capture_output=True,
                    text=True,
                    timeout=5
                )
                device_info = device.to_dict()
                device_info['power_check'] = result.stdout.strip() if result.returncode == 0 else "unknown"
                print(json.dumps(device_info, indent=2))
            except:
                print(json.dumps(device.to_dict(), indent=2))
        else:
            print(json.dumps({"error": "Could not connect to device"}))
    
    else:
        print(f"Unknown command: {command}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()

package androidtv

import "errors"

// Structured error types for Android TV protocol
var (
	ErrOffline        = errors.New("device offline")
	ErrUnauthorized   = errors.New("authentication failed")
	ErrTimeout        = errors.New("connection timeout")
	ErrInvalidURL     = errors.New("invalid media URL")
	ErrDeviceNotFound = errors.New("device not found")
)

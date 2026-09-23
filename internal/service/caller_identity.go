package service

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrManagementCallerIdentity indicates management caller identity is missing or invalid.
	ErrManagementCallerIdentity = errors.New("management caller identity is unavailable")
	// ErrDeviceCallerIdentity indicates device caller identity is missing or invalid.
	ErrDeviceCallerIdentity = errors.New("device caller identity is unavailable")
)

// ManagementCaller identifies an authenticated management caller.
type ManagementCaller struct {
	CustomerID CustomerID
}

// Validate verifies the management caller has valid service identity.
func (c ManagementCaller) Validate() error {
	return c.CustomerID.Validate()
}

// DeviceCaller identifies an authenticated installed device.
type DeviceCaller struct {
	CustomerID CustomerID
	DeviceID   DeviceID
}

// Validate verifies the device caller has valid service identity.
func (c DeviceCaller) Validate() error {
	if err := c.CustomerID.Validate(); err != nil {
		return err
	}
	return c.DeviceID.Validate()
}

type managementCallerContextKey struct{}
type deviceCallerContextKey struct{}

// WithManagementCaller stores an authenticated management caller in context.
func WithManagementCaller(ctx context.Context, caller ManagementCaller) context.Context {
	return context.WithValue(ctx, managementCallerContextKey{}, caller)
}

// ManagementCallerFromContext returns the authenticated management caller.
func ManagementCallerFromContext(ctx context.Context) (ManagementCaller, error) {
	caller, ok := ctx.Value(managementCallerContextKey{}).(ManagementCaller)
	if !ok {
		return ManagementCaller{}, ErrManagementCallerIdentity
	}
	if err := caller.Validate(); err != nil {
		return ManagementCaller{}, fmt.Errorf("%w: %v", ErrManagementCallerIdentity, err)
	}
	return caller, nil
}

// WithDeviceCaller stores an authenticated device caller in context.
func WithDeviceCaller(ctx context.Context, caller DeviceCaller) context.Context {
	return context.WithValue(ctx, deviceCallerContextKey{}, caller)
}

// DeviceCallerFromContext returns the authenticated device caller.
func DeviceCallerFromContext(ctx context.Context) (DeviceCaller, error) {
	caller, ok := ctx.Value(deviceCallerContextKey{}).(DeviceCaller)
	if !ok {
		return DeviceCaller{}, ErrDeviceCallerIdentity
	}
	if err := caller.Validate(); err != nil {
		return DeviceCaller{}, fmt.Errorf("%w: %v", ErrDeviceCallerIdentity, err)
	}
	return caller, nil
}

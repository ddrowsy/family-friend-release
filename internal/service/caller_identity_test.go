package service

import (
	"context"
	"errors"
	"testing"
)

func TestManagementCallerContext(t *testing.T) {
	caller := ManagementCaller{CustomerID: "customer-1"}
	ctx := WithManagementCaller(context.Background(), caller)

	got, err := ManagementCallerFromContext(ctx)
	if err != nil {
		t.Fatalf("ManagementCallerFromContext() error = %v", err)
	}
	if got != caller {
		t.Fatalf("ManagementCallerFromContext() = %#v, want %#v", got, caller)
	}
}

func TestManagementCallerContextRejectsMissingAndInvalidIdentity(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "missing",
			ctx:  context.Background(),
		},
		{
			name: "invalid customer",
			ctx: WithManagementCaller(
				context.Background(),
				ManagementCaller{},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ManagementCallerFromContext(tt.ctx)
			if !errors.Is(err, ErrManagementCallerIdentity) {
				t.Fatalf("ManagementCallerFromContext() error = %v", err)
			}
		})
	}
}

func TestDeviceCallerContext(t *testing.T) {
	caller := DeviceCaller{
		CustomerID: "customer-1",
		DeviceID:   "device-1",
	}
	ctx := WithDeviceCaller(context.Background(), caller)

	got, err := DeviceCallerFromContext(ctx)
	if err != nil {
		t.Fatalf("DeviceCallerFromContext() error = %v", err)
	}
	if got != caller {
		t.Fatalf("DeviceCallerFromContext() = %#v, want %#v", got, caller)
	}
}

func TestDeviceCallerContextRejectsMissingAndInvalidIdentity(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "missing",
			ctx:  context.Background(),
		},
		{
			name: "invalid customer",
			ctx: WithDeviceCaller(
				context.Background(),
				DeviceCaller{DeviceID: "device-1"},
			),
		},
		{
			name: "invalid device",
			ctx: WithDeviceCaller(
				context.Background(),
				DeviceCaller{CustomerID: "customer-1"},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeviceCallerFromContext(tt.ctx)
			if !errors.Is(err, ErrDeviceCallerIdentity) {
				t.Fatalf("DeviceCallerFromContext() error = %v", err)
			}
		})
	}
}

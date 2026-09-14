//go:build !windows

package ui

import (
	"context"
	"errors"
	"github.com/hanboyd/AirDropPlus-Go/internal/bridge"
	"github.com/hanboyd/AirDropPlus-Go/internal/device"
	"github.com/hanboyd/AirDropPlus-Go/internal/history"
)

func Run(context.Context, *history.Store, *bridge.Service, *device.Tracker) error {
	return errors.New("native tray UI is only available on Windows")
}

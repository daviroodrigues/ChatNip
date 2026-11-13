package infrastructure

import (
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

type WhatsMeowDeviceFactory struct {
	container *sqlstore.Container
}

func NewWhatsMeowDeviceFactory(container *sqlstore.Container) *WhatsMeowDeviceFactory {
	return &WhatsMeowDeviceFactory{container: container}
}

func (f *WhatsMeowDeviceFactory) NewDevice() (*store.Device, error) {
	device := f.container.NewDevice()
	device.Platform = domain.DefaultDeviceName
	if err := device.Save(context.Background()); err != nil {
		return nil, err
	}
	return device, nil
}

func (f *WhatsMeowDeviceFactory) GetDevice(ctx context.Context, jid types.JID) (*store.Device, error) {
	return f.container.GetDevice(ctx, jid)
}

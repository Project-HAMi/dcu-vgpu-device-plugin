/*
Copyright 2024 The HAMi Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dcu

import (
	"fmt"
	"os"
	"time"

	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/api"
	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/util"
	"k8s.io/klog/v2"
	kubeletdevicepluginv1beta1 "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type DevListFunc func() []*kubeletdevicepluginv1beta1.Device

func (r *Plugin) apiDevices() (*[]*api.DeviceInfo, error) {
	res := []*api.DeviceInfo{}

	klog.Infof("Getting device serial numbers")
	deviceSerialInfos, err := util.GetDeviceSerialInfos(r.devices)
	if err != nil {
		return nil, fmt.Errorf("failed to get device serial numbers: %w", err)
	}
	klog.Infof("Device serial numbers retrieved: %v", deviceSerialInfos)

	for idx, val := range r.devices {
		if val.MemoryTotal > 0 {
			res = append(res, &api.DeviceInfo{
				Index:   val.DvInd,
				Id:      util.GetDeviceUUIDFromDevSerialNumber(deviceSerialInfos[idx].SerialNumber),
				Count:   4,
				Devmem:  int32(val.MemoryTotal / 1024 / 1024),
				Devcore: 100,
				Numa:    0,
				Type:    val.DevTypeName,
				Health:  true,
			})
		}
	}
	return &res, nil
}

func (r *Plugin) RegistrInAnnotation() error {
	devices, err := r.apiDevices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	annos := make(map[string]string)
	if len(util.NodeName) == 0 {
		util.NodeName = os.Getenv(util.NodeNameEnvName)
	}
	node, err := util.GetNode(util.NodeName)
	if err != nil {
		klog.Errorln("get node error", err.Error())
		return err
	}
	encodeddevices := util.EncodeNodeDevices(*devices)
	annos[util.HandshakeAnnosString] = "Reported " + time.Now().String()
	annos[util.RegisterAnnos] = encodeddevices
	klog.Infoln("Reporting devices", encodeddevices, "in", time.Now().String())
	err = util.PatchNodeAnnotations(node, annos)

	if err != nil {
		klog.Errorln("patch node error", err.Error())
	}
	return err
}

func (r *Plugin) WatchAndRegister() {
	klog.Info("into WatchAndRegister")
	for {
		r.RefreshContainerDevices()
		err := r.RegistrInAnnotation()
		if err != nil {
			klog.Errorf("register error, %v", err)
			time.Sleep(time.Second * 5)
		} else {
			time.Sleep(time.Second * 30)
		}
	}
}

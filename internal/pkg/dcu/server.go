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
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/amdgpu"
	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/api"
	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/hwloc"
	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/util"
	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/util/client"
	"github.com/HAMi/dcu-vgpu-device-plugin/internal/pkg/util/nodelock"
	"github.com/Project-HAMi/dcu-dcgm/pkg/dcgm"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	kubeletdevicepluginv1beta1 "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	NodeLockDCU = "hami.io/dcumutex.lock"
)

// Plugin is identical to DevicePluginServer interface of device plugin API.
type Plugin struct {
	AMDGPUs   map[string]map[string]int
	devices   []dcgm.DeviceInfo
	Heartbeat chan bool
	vidx      []bool
	pipeid    [][]bool
	coremask  [][]string
	count     int
}

// Start is an optional interface that could be implemented by plugin.
// If case Start is implemented, it will be executed by Manager after
// plugin instantiation and before its registration to kubelet. This
// method could be used to prepare resources before they are offered
// to Kubernetes.
func (p *Plugin) Start() error {
	var err error
	p.vidx = make([]bool, 200)
	for idx := range p.vidx {
		p.vidx[idx] = false
	}
	p.pipeid = make([][]bool, 16)
	for idx := range p.pipeid {
		p.pipeid[idx] = make([]bool, 20)
		for id := range p.pipeid[idx] {
			p.pipeid[idx][id] = false
		}
	}
	p.coremask = make([][]string, 16)
	for idx := range p.coremask {
		p.coremask[idx] = make([]string, 2)
		p.coremask[idx][0] = ""
		p.coremask[idx][1] = ""
	}
	p.count = 0
	dcgm.Init()
	p.devices, err = dcgm.DeviceInfos()
	if err != nil {
		log.Fatalf("dcgm DeviceInfos failed:%s", err.Error())
	}
	for idx := range p.devices {
		p.devices[idx].DevTypeName = fmt.Sprintf("%v-%v", "DCU", p.devices[idx].DevTypeName)
	}
	fmt.Println("infos=", p.devices)

	for idx, val := range p.devices {
		p.coremask[idx][0] = initCoreUsage(int(val.ComputeUnit))
		p.coremask[idx][1] = initCoreUsage(int(val.ComputeUnit))
	}
	go p.WatchAndRegister()
	return nil
}

// Stop is an optional interface that could be implemented by plugin.
// If case Stop is implemented, it will be executed by Manager after the
// plugin is unregistered from kubelet. This method could be used to tear
// down resources.
func (p *Plugin) Stop() error {
	return nil
}

var topoSIMDre = regexp.MustCompile(`simd_count\s(\d+)`)

func countGPUDevFromTopology(topoRootParam ...string) int {
	topoRoot := "/sys/class/kfd/kfd"
	if len(topoRootParam) == 1 {
		topoRoot = topoRootParam[0]
	}

	count := 0
	var nodeFiles []string
	var err error
	if nodeFiles, err = filepath.Glob(topoRoot + "/topology/nodes/*/properties"); err != nil {
		glog.Fatalf("glob error: %s", err)
		return count
	}

	for _, nodeFile := range nodeFiles {
		glog.Info("Parsing " + nodeFile)
		f, e := os.Open(nodeFile)
		if e != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			m := topoSIMDre.FindStringSubmatch(scanner.Text())
			if m == nil {
				continue
			}

			if v, _ := strconv.Atoi(m[1]); v > 0 {
				count++
				break
			}
		}
		f.Close()
	}
	return count
}

func simpleHealthCheck() bool {
	var kfd *os.File
	var err error
	if kfd, err = os.Open("/dev/kfd"); err != nil {
		glog.Error("Error opening /dev/kfd")
		return false
	}
	kfd.Close()
	return true
}

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (p *Plugin) GetDevicePluginOptions(ctx context.Context, e *kubeletdevicepluginv1beta1.Empty) (*kubeletdevicepluginv1beta1.DevicePluginOptions, error) {
	return &kubeletdevicepluginv1beta1.DevicePluginOptions{}, nil
}

// PreStartContainer is expected to be called before each container start if indicated by plugin during registration phase.
// PreStartContainer allows kubelet to pass reinitialized devices to containers.
// PreStartContainer allows Device Plugin to run device specific operations on the Devices requested
func (p *Plugin) PreStartContainer(ctx context.Context, r *kubeletdevicepluginv1beta1.PreStartContainerRequest) (*kubeletdevicepluginv1beta1.PreStartContainerResponse, error) {
	return &kubeletdevicepluginv1beta1.PreStartContainerResponse{}, nil
}

// GetPreferredAllocation returns a preferred set of devices to allocate
// from a list of available ones. The resulting preferred allocation is not
// guaranteed to be the allocation ultimately performed by the
// devicemanager. It is only designed to help the devicemanager make a more
// informed allocation decision when possible.
func (p *Plugin) GetPreferredAllocation(context.Context, *kubeletdevicepluginv1beta1.PreferredAllocationRequest) (*kubeletdevicepluginv1beta1.PreferredAllocationResponse, error) {
	return &kubeletdevicepluginv1beta1.PreferredAllocationResponse{}, nil
}

func (p *Plugin) generateFakeDevs(devices *[]*api.DeviceInfo) []*kubeletdevicepluginv1beta1.Device {
	fakedevs := []*kubeletdevicepluginv1beta1.Device{}

	for _, val := range *devices {
		idx := 0
		for idx < int(val.Count) {
			fakedevs = append(fakedevs, &kubeletdevicepluginv1beta1.Device{
				ID:     val.Id + "-fake-" + fmt.Sprint(idx),
				Health: kubeletdevicepluginv1beta1.Healthy,
			})
			idx++
		}
	}
	return fakedevs
}

func (p *Plugin) RefreshContainerDevices() error {
	files, err := os.ReadDir("/usr/local/vgpu/dcu")
	if err != nil {
		return err
	}
	idx := 0
	for idx < len(p.devices) {
		p.coremask[idx][0] = initCoreUsage(int(p.devices[idx].ComputeUnit))
		p.coremask[idx][1] = initCoreUsage(int(p.devices[idx].ComputeUnit))
		idx++
	}

	for _, f := range files {
		pods, err := client.GetClient().CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		found := false
		for _, val := range pods.Items {
			if strings.Contains(f.Name(), string(val.UID)) {
				found = true
				var didx, pid, vdidx int
				tmpstr := strings.Split(f.Name(), "_")
				didx, _ = strconv.Atoi(tmpstr[2])
				pid, _ = strconv.Atoi(tmpstr[3])
				vdidx, _ = strconv.Atoi(tmpstr[4])
				p.coremask[didx][0], _ = addCoreUsage(p.coremask[didx][0], tmpstr[5])
				p.coremask[didx][1], _ = addCoreUsage(p.coremask[didx][1], tmpstr[6])
				p.vidx[vdidx] = true
				p.pipeid[didx][pid] = true
			}
		}
		if !found {
			var didx, pid, vdidx int
			tmpstr := strings.Split(f.Name(), "_")
			didx, _ = strconv.Atoi(tmpstr[2])
			pid, _ = strconv.Atoi(tmpstr[3])
			vdidx, _ = strconv.Atoi(tmpstr[4])
			p.vidx[vdidx] = false
			p.pipeid[didx][pid] = false
			os.RemoveAll("/usr/local/vgpu/dcu/" + f.Name())
			os.Remove(fmt.Sprintf("/etc/vdev/vdev%d.conf", vdidx))
		}
		fmt.Println(f.Name())
	}
	fmt.Println(p.coremask)
	return nil
}

func (p *Plugin) AllocateVidx() (int, error) {
	for idx := range p.vidx {
		if p.vidx[idx] == false {
			p.vidx[idx] = true
			return idx, nil
		}
	}
	return 0, errors.New("vidx out of bound (>200)")
}

func (p *Plugin) AllocatePipeID(devidx int) (int, error) {
	for idx := range p.pipeid[devidx] {
		if p.pipeid[devidx][idx] == false {
			p.pipeid[devidx][idx] = true
			return idx, nil
		}
	}
	return 0, errors.New("pipidx out of bound:" + fmt.Sprint(devidx))
}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disappears, ListAndWatch
// returns the new list
func (p *Plugin) ListAndWatch(e *kubeletdevicepluginv1beta1.Empty, s kubeletdevicepluginv1beta1.DevicePlugin_ListAndWatchServer) error {
	p.AMDGPUs = amdgpu.GetAMDGPUs()

	devs := make([]*kubeletdevicepluginv1beta1.Device, len(p.AMDGPUs))

	// limit scope for hwloc
	func() {
		var hw hwloc.Hwloc
		hw.Init()
		defer hw.Destroy()

		i := 0
		for id := range p.AMDGPUs {
			dev := &kubeletdevicepluginv1beta1.Device{
				ID:     id,
				Health: kubeletdevicepluginv1beta1.Healthy,
			}
			devs[i] = dev
			i++

			numas, err := hw.GetNUMANodes(id)
			glog.Infof("Watching GPU with bus ID: %s NUMA Node: %+v", id, numas)
			if err != nil {
				glog.Error(err)
				continue
			}

			if len(numas) == 0 {
				glog.Errorf("No NUMA for GPU ID: %s", id)
				continue
			}

			numaNodes := make([]*kubeletdevicepluginv1beta1.NUMANode, len(numas))
			for j, v := range numas {
				numaNodes[j] = &kubeletdevicepluginv1beta1.NUMANode{
					ID: int64(v),
				}
			}

			dev.Topology = &kubeletdevicepluginv1beta1.TopologyInfo{
				Nodes: numaNodes,
			}
		}
	}()

	fakedevs := p.apiDevices()
	s.Send(&kubeletdevicepluginv1beta1.ListAndWatchResponse{Devices: p.generateFakeDevs(fakedevs)})

	for {
		select {
		case <-p.Heartbeat:
			var health = kubeletdevicepluginv1beta1.Unhealthy

			// TODO there are no per device health check currently
			// TODO all devices on a node is used together by kfd
			if simpleHealthCheck() {
				health = kubeletdevicepluginv1beta1.Healthy
			}

			for i := 0; i < len(p.AMDGPUs); i++ {
				devs[i].Health = health
			}
			s.Send(&kubeletdevicepluginv1beta1.ListAndWatchResponse{Devices: p.generateFakeDevs(fakedevs)})
		}
	}
	// returning a value with this function will unregister the plugin from k8s
}

func getIndexFromUUID(uid string) int {
	ret, _ := strconv.ParseInt(uid[4:], 10, 64)
	return int(ret)
}

// Create virtual vdev directory and file
func (p *Plugin) createvdevFiles(current *corev1.Pod, ctr *corev1.Container, req util.ContainerDevices) (string, error) {
	var devidx, pipeid, vdevidx int
	var pcibusId string
	var reqcores, mem int32
	var err error
	coremsk1 := initCoreUsage(16)
	coremsk2 := initCoreUsage(16)
	reqtmp := 0
	if len(req) > 1 {
		return "", nil
	}
	for _, val := range req {
		if len(val.UUID) == 0 {
			continue
		}
		idx := getIndexFromUUID(val.UUID)
		pcibusId = p.devices[idx].PciBusNumber
		reqcores = (val.Usedcores * int32(p.devices[idx].ComputeUnit)) / 100
		coremsk1, reqtmp, _ = allocCoreUsage(p.coremask[idx][0], int(reqcores))
		if reqtmp > 0 {
			coremsk2, _, _ = allocCoreUsage(p.coremask[idx][1], reqtmp)
		}
		mem = val.Usedmem
		devidx = idx
		vdevidx, err = p.AllocateVidx()
		if err != nil {
			return "", err
		}
		pipeid, err = p.AllocatePipeID(idx)
		if err != nil {
			return "", err
		}
	}
	dirName := string(current.UID) + "_" + ctr.Name + "_" + fmt.Sprint(devidx) + "_" + fmt.Sprint(pipeid) + "_" + fmt.Sprint(vdevidx) + "_" + fmt.Sprint(coremsk1) + "_" + fmt.Sprint(coremsk2)
	cacheFileHostDirectory := fmt.Sprintf("/usr/local/vgpu/dcu/%s", dirName)
	err = createvdevFile(pcibusId, coremsk1, coremsk2, reqcores, mem, devidx, vdevidx, pipeid, cacheFileHostDirectory, fmt.Sprintf("vdev%d.conf", vdevidx))
	if err != nil {
		return "", err
	}
	// support dcu-exporter
	err = createvdevFile(pcibusId, coremsk1, coremsk2, reqcores, mem, devidx, vdevidx, pipeid, "/etc/vdev/", fmt.Sprintf("vdev%d.conf", vdevidx))
	if err != nil {
		return "", err
	}

	coreUsage1, err := addCoreUsage(p.coremask[devidx][0], coremsk1)
	if err != nil {
		return "", err
	}
	p.coremask[devidx][0] = coreUsage1

	coreUsage2, err := addCoreUsage(p.coremask[devidx][1], coremsk2)
	if err != nil {
		return "", err
	}
	p.coremask[devidx][1] = coreUsage2

	return cacheFileHostDirectory, nil
}

func createvdevFile(pcibusId, coremsk1, coremsk2 string, reqcores, mem int32, deviceid, vdevidx, pipeid int, cacheFileHostDirectory, cacheFileName string) error {
	s := ""
	s = fmt.Sprintf("PciBusId: %s\n", pcibusId)
	s = s + fmt.Sprintf("cu_mask: 0x%s\n", coremsk1)
	s = s + fmt.Sprintf("cu_mask: 0x%s\n", coremsk2)
	s = s + fmt.Sprintf("cu_count: %d\n", reqcores)
	s = s + fmt.Sprintf("mem: %d MiB\n", mem)
	s = s + fmt.Sprintf("device_id: %d\n", deviceid)
	s = s + fmt.Sprintf("vdev_id: %d\n", vdevidx)
	s = s + fmt.Sprintf("pipe_id: %d\n", pipeid)
	s = s + fmt.Sprintln("enable: 1")
	klog.Infoln("s=", s)

	_, err := os.Stat(cacheFileHostDirectory)
	if os.IsNotExist(err) {
		err := os.MkdirAll(cacheFileHostDirectory, 0777)
		if err != nil {
			return err
		}
		err = os.Chmod(cacheFileHostDirectory, 0777)
		if err != nil {
			return err
		}
	}

	err = os.WriteFile(fmt.Sprintf("%s/%s", cacheFileHostDirectory, cacheFileName), []byte(s), os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Allocate(ctx context.Context, reqs *kubeletdevicepluginv1beta1.AllocateRequest) (*kubeletdevicepluginv1beta1.AllocateResponse, error) {
	var car kubeletdevicepluginv1beta1.ContainerAllocateResponse
	var dev *kubeletdevicepluginv1beta1.DeviceSpec
	responses := kubeletdevicepluginv1beta1.AllocateResponse{}
	nodename := util.NodeName
	current, err := util.GetPendingPod(nodename)
	if err != nil {
		nodelock.ReleaseNodeLock(nodename, NodeLockDCU)
		return &kubeletdevicepluginv1beta1.AllocateResponse{}, err
	}
	drmCards, drmRenders, err := util.ListDcuDrmDevices()
	if err != nil {
		util.PodAllocationFailed(nodename, current, NodeLockDCU)
		return &kubeletdevicepluginv1beta1.AllocateResponse{}, err
	}
	for idx := range reqs.ContainerRequests {
		currentCtr, devreq, err := util.GetNextDeviceRequest(util.HygonDCUDevice, *current)
		klog.Infoln("deviceAllocateFromAnnotation=", devreq)
		if err != nil {
			util.PodAllocationFailed(nodename, current, NodeLockDCU)
			return &kubeletdevicepluginv1beta1.AllocateResponse{}, err
		}
		if len(devreq) != len(reqs.ContainerRequests[idx].DevicesIDs) {
			util.PodAllocationFailed(nodename, current, NodeLockDCU)
			return &kubeletdevicepluginv1beta1.AllocateResponse{}, errors.New("device number not matched")
		}

		err = util.EraseNextDeviceTypeFromAnnotation(util.HygonDCUDevice, *current)
		if err != nil {
			util.PodAllocationFailed(nodename, current, NodeLockDCU)
			return &kubeletdevicepluginv1beta1.AllocateResponse{}, err
		}

		car = kubeletdevicepluginv1beta1.ContainerAllocateResponse{}
		// Currently, there are only 1 /dev/kfd per nodes regardless of the # of GPU available
		// for compute/rocm/HSA use cases
		dev = new(kubeletdevicepluginv1beta1.DeviceSpec)
		dev.HostPath = "/dev/kfd"
		dev.ContainerPath = "/dev/kfd"
		dev.Permissions = "rwm"
		car.Devices = append(car.Devices, dev)

		dev = new(kubeletdevicepluginv1beta1.DeviceSpec)
		dev.HostPath = "/dev/mkfd"
		dev.ContainerPath = "/dev/mkfd"
		dev.Permissions = "rwm"
		car.Devices = append(car.Devices, dev)

		for _, val := range devreq {
			var devIdx = -1
			klog.Infof("Allocating device ID: %s", val.UUID)
			succeedCount, err := fmt.Sscanf(val.UUID, "DCU-%d", &devIdx)
			if err != nil || succeedCount == 0 || devIdx == -1 {
				klog.Errorf("Invalid request device uuid: %s", val.UUID)
				util.PodAllocationFailed(nodename, current, NodeLockDCU)
				return &kubeletdevicepluginv1beta1.AllocateResponse{}, fmt.Errorf("invalid request device uuid %s", val.UUID)
			}

			if devIdx > len(drmCards) || devIdx > len(drmRenders) {
				klog.Errorf("Invalid device index: %d, all devices counts is: %d, all renders count is: %d", devIdx, len(drmCards), len(drmRenders))
				util.PodAllocationFailed(nodename, current, NodeLockDCU)
				return &kubeletdevicepluginv1beta1.AllocateResponse{}, fmt.Errorf("can not match dcu dri request %s. cards %d, renders %d", val.UUID, len(drmCards), len(drmRenders))
			}

			drmCardName := drmCards[devIdx]
			klog.Infof("All dcu dri card devs: %v, mapped dri: %s", drmCards, drmCardName)
			devpath := fmt.Sprintf("/dev/dri/%s", drmCardName)
			dev = new(kubeletdevicepluginv1beta1.DeviceSpec)
			dev.HostPath = devpath
			dev.ContainerPath = devpath
			dev.Permissions = "rw"
			car.Devices = append(car.Devices, dev)

			drmRenderName := drmRenders[devIdx]
			klog.Infof("All dcu dri render devs: %v, mapped dri: %s", drmRenders, drmRenderName)
			devpath = fmt.Sprintf("/dev/dri/%s", drmRenderName)
			dev = new(kubeletdevicepluginv1beta1.DeviceSpec)
			dev.HostPath = devpath
			dev.ContainerPath = devpath
			dev.Permissions = "rw"
			car.Devices = append(car.Devices, dev)
		}
		//Create vdev file
		klog.Infoln("devreqs=", len(devreq), "usedmem=", devreq[0].Usedmem, ":", p.devices[0].MemoryTotal/1024/1024)
		if len(devreq) < 2 && devreq[0].Usedmem < int32(p.devices[0].MemoryTotal/1024/1024) {
			filename, err := p.createvdevFiles(current, &currentCtr, devreq)
			if err != nil {
				util.PodAllocationFailed(nodename, current, NodeLockDCU)
				return &responses, err
			}
			if len(filename) > 0 {
				car.Mounts = append(car.Mounts, &kubeletdevicepluginv1beta1.Mount{
					ContainerPath: "/etc/vdev/docker/",
					HostPath:      filename,
					ReadOnly:      false,
				}, &kubeletdevicepluginv1beta1.Mount{
					ContainerPath: "/opt/hygondriver",
					HostPath:      os.Getenv("HYGONPATH"),
					ReadOnly:      false,
				}, &kubeletdevicepluginv1beta1.Mount{
					ContainerPath: "/opt/hyhal",
					HostPath:      "/opt/hyhal",
					ReadOnly:      false,
				})
				car.Mounts = append(car.Mounts)
			}
		}
		responses.ContainerResponses = append(responses.ContainerResponses, &car)
	}
	klog.Infoln("response=", responses)
	util.PodAllocationTrySuccess(nodename, util.HygonDCUDevice, NodeLockDCU, current)
	return &responses, nil
}

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (p *Plugin) AllocateOrigin(ctx context.Context, r *kubeletdevicepluginv1beta1.AllocateRequest) (*kubeletdevicepluginv1beta1.AllocateResponse, error) {
	var response kubeletdevicepluginv1beta1.AllocateResponse
	var car kubeletdevicepluginv1beta1.ContainerAllocateResponse
	var dev *kubeletdevicepluginv1beta1.DeviceSpec

	for _, req := range r.ContainerRequests {
		car = kubeletdevicepluginv1beta1.ContainerAllocateResponse{}

		// Currently, there are only 1 /dev/kfd per nodes regardless of the # of GPU available
		// for compute/rocm/HSA use cases
		dev = new(kubeletdevicepluginv1beta1.DeviceSpec)
		dev.HostPath = "/dev/kfd"
		dev.ContainerPath = "/dev/kfd"
		dev.Permissions = "rw"
		car.Devices = append(car.Devices, dev)

		for _, id := range req.DevicesIDs {
			glog.Infof("Allocating device ID: %s", id)

			for k, v := range p.AMDGPUs[id] {
				devpath := fmt.Sprintf("/dev/dri/%s%d", k, v)
				dev = new(kubeletdevicepluginv1beta1.DeviceSpec)
				dev.HostPath = devpath
				dev.ContainerPath = devpath
				dev.Permissions = "rw"
				car.Devices = append(car.Devices, dev)
			}

		}

		response.ContainerResponses = append(response.ContainerResponses, &car)
	}

	return &response, nil
}

# DCU vGPU device plugin for HAMi
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FProject-HAMi%2Fdcu-vgpu-device-plugin.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FProject-HAMi%2Fdcu-vgpu-device-plugin?ref=badge_shield)


## Introduction
This is a [Kubernetes][k8s] [device plugin][dp] implementation that enables the registration of hygon DCU in a container cluster for compute workload.  With the approrpriate hardware and this plugin deployed in your Kubernetes cluster, you will be able to run jobs that require AMD DCU. It supports DCU-virtualzation by using hy-virtual provided by dtk

## Architect
The flow of vDCU job is as follows:

<img src="./DCU_job_flow.png" width="600px">

## Prerequisites
* dtk >= 24.04
* hy=smi == v1.6.0


## Limitations
* This plugin targets Kubernetes v1.18+.

## Deployment

### Prepare
```bash
# on the dcu node, create these directory:
$ mkdir /etc/vdev
```
```bash
# should change dtk-xx.xx.x to your installed dtk version
$ cp -r /opt/dtk-xx.xx.x /opt/dtk
```

```bash
$ kubectl apply -f k8s-dcu-rbac.yaml
$ kubectl apply -f k8s-dcu-plugin.yaml
# replace NODE_NAME with your dcu node name
$ kubectl label node NODE_NAME dcu=on
```

## Build
```bash
docker build .
```

## Examples

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: alexnet-tf-gpu-pod-mem
  labels:
    purpose: demo-tf-amdgpu
spec:
  containers:
    - name: alexnet-tf-gpu-container
      image: ubuntu:20.04
      workingDir: /root
      command: ["sleep","infinity"]
      resources:
        limits:
          hygon.com/dcunum: 1 # requesting a GPU
          hygon.com/dcumem: 2000 # each dcu require 2000 MiB device memory
          hygon.com/dcucores: 15 # each dcu use 60% of total compute cores
```

## Validation

Inside container, use hy-virtual to validate

```bash
source /opt/hygondriver/env.sh
hy-virtual -show-device-info
```

There will be output like these:
```bash
Device 0:
	Actual Device: 0
	Compute units: 9
	Global memory: 2097152000 bytes
```

## Maintainer

limengxuan@4paradigm.com

## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FProject-HAMi%2Fdcu-vgpu-device-plugin.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2FProject-HAMi%2Fdcu-vgpu-device-plugin?ref=badge_large)
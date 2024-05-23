# DCU vGPU device plugin for HAMi

## Introduction
This is a [Kubernetes][k8s] [device plugin][dp] implementation that enables the registration of hygon DCU in a container cluster for compute workload.  With the approrpriate hardware and this plugin deployed in your Kubernetes cluster, you will be able to run jobs that require AMD DCU. It supports DCU-virtualzation by using hy-virtual provided by dtk


## Prerequisites
* dtk >= 24.04
* hy=smi == v1.6.0


## Limitations
* This plugin targets Kubernetes v1.18+.

## Deployment
```
$ kubectl apply -f k8s-dcu-rbac.yaml
$ kubectl apply -f k8s-dcu-plugin.yaml
```

## Build
```
docker build .
```

## Maintainer

limengxuan@4paradigm.com

module github.com/HAMi/dcu-vgpu-device-plugin

go 1.15

require (
	github.com/go-logr/logr v1.2.4
	github.com/golang/glog v1.0.0
	github.com/kubevirt/device-plugin-manager v1.18.8
	gotest.tools/v3 v3.0.2
	k8s.io/api v0.28.3
	k8s.io/apimachinery v0.28.3
	k8s.io/client-go v0.28.3
	k8s.io/klog/v2 v2.100.1
	k8s.io/kubelet v0.28.3
	sigs.k8s.io/controller-runtime v0.8.1
)

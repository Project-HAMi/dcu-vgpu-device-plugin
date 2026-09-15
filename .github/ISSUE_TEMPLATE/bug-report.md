---
name: Bug Report
about: Report a problem encountered while using DCU vGPU device plugin
labels: bug
---

<!-- Please use this template while reporting a bug and provide as much info as possible. Not doing so may result in your bug not being addressed in a timely manner. Thanks!
-->

**What happened**:

**What you expected to happen**:

**How to reproduce it (as minimally and precisely as possible)**:

**Anything else we need to know?**:

- Relevant `hy-smi` and `hy-virtual -show-device-info` output
- Relevant DCU device-plugin, kubelet, and workload log excerpts
- Relevant node annotations and allocatable `hygon.com/*` resources
- Relevant deployment manifest or Helm values sections
- Relevant, time-bounded DCU driver or kernel log excerpts

Before posting, include only relevant, time-bounded excerpts and remove or mask credentials, tokens, private keys, certificates, device identifiers, node or host names, workload identifiers, and internal image names.

**Environment**:
- dcu-vgpu-device-plugin version or commit:
- Kubernetes version:
- DTK and `hy-smi` versions:
- DCU model and driver version:
- Container runtime and version:
- Deployment image and tag:
- Kernel version from `uname -a`:
- Others:

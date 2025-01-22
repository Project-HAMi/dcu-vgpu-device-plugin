# Copyright 2018 Advanced Micro Devices, Inc.  All rights reserved.
# 
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
# 
#      http://www.apache.org/licenses/LICENSE-2.0
# 
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
FROM golang:1.21-bullseye AS GOBUILD
ADD . /device-plugin
# ARG GOPROXY=https://goproxy.cn,direct
RUN apt-get update && apt-get -y install libhwloc-dev libdrm-dev
RUN cd /device-plugin && go build -o ./k8s-device-plugin cmd/k8s-device-plugin/main.go

FROM ubuntu:20.04
ENV TZ=Asia/Dubai
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone
RUN apt-get update && apt-get -y install libhwloc-dev libdrm-dev pciutils libelf-dev kmod
ENV LD_LIBRARY_PATH=/opt/hygondriver/hip/lib:/opt/hygondriver/llvm/lib:/opt/hygondriver/lib:/opt/hygondriver/lib64:/opt/hyhal/lib:/opt/hyhal/lib64:/opt/hygondriver/.hyhal/lib:/opt/hygondriver/.hyhal/lib64:
ENV PATH=/opt/hygondriver/bin:/opt/hygondriver/llvm/bin:/opt/hygondriver/hip/bin:/opt/hygondriver/hip/bin/hipify:/opt/hyhal/bin:/opt/hygondriver/.hyhal/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV C_INCLUDE_PATH=/opt/hygondriver/include:/opt/hyhal/include:/opt/hygondriver/llvm/include:/opt/hygondriver/.hyhal/include:
ENV ROCM_PATH=/opt/hygondriver
ENV DTKROOT=/opt/hygondriver
ENV CPLUS_INCLUDE_PATH=/opt/hygondriver/include:/opt/hyhal/include:/opt/hygondriver/llvm/include:/opt/hygondriver/.hyhal/include:
ENV HYHAL_PATH=/opt/hyhal
WORKDIR /root/
COPY --from=GOBUILD /device-plugin/k8s-device-plugin .
CMD ["./k8s-device-plugin", "-logtostderr=true", "-stderrthreshold=INFO", "-v=5"]

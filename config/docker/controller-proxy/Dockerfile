# Copyright 2023 Hedgehog
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM ubuntu:24.10 AS build
ARG version="1.11.2"
ENV version_env=$version

RUN apt update && apt install -y build-essential
ADD https://github.com/tinyproxy/tinyproxy/releases/download/$version/tinyproxy-$version.tar.xz /tinyproxy-$version.tar.xz
RUN tar -xaf tinyproxy-$version.tar.xz
WORKDIR /tinyproxy-$version 
RUN ./autogen.sh
RUN ./configure LDFLAGS="-static" --enable-manpage_support=no && make

FROM gcr.io/distroless/static:nonroot
COPY --from=build /tinyproxy-${version_env}/src/tinyproxy /tinyproxy-${version_env}
CMD ["/tinyproxy-${version_env}","-c /tinyproxy.conf"]


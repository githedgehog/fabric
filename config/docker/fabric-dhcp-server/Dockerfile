FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive

RUN yes | unminimize

RUN apt-get -q -y update \
    && apt-get -q -y -o "DPkg::Options::=--force-confold" -o "DPkg::Options::=--force-confdef" install \
    apt-utils dumb-init isc-dhcp-server man \
    && apt-get -q -y autoremove \
    && apt-get -q -y clean \
    && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
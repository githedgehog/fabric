#!/bin/bash

set -e

data_dir="/data"
if [ ! -d "$data_dir" ]; then
    echo "Please ensure '$data_dir' folder is available."
    exit 1
fi

dhcpd_conf="/dhcpd.conf"
if [ ! -r "$dhcpd_conf" ]; then
    echo "Please ensure '$dhcpd_conf' exists and is readable."
    exit 1
fi

[ -e "$data_dir/dhcpd.leases" ] || touch "$data_dir/dhcpd.leases"

exec /usr/bin/dumb-init -- /usr/sbin/dhcpd -4 -f -d --no-pid -cf "$dhcpd_conf" -lf "$data_dir/dhcpd.leases" -user dhcpd -group dhcpd "$@"

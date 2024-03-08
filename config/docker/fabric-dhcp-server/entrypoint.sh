#!/bin/bash
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

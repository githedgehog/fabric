#!/bin/bash

set -e

show vlan config | grep Ethernet | awk '{print $2 " " $3}' | xargs -L1 sudo config vlan member del
show vlan config | grep Vlan | awk '{print $2}' | xargs -L1 sudo config vlan del
show vlan config
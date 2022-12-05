#!/bin/bash

set -e
# set -x

port=$1
vlan=$2
untagged=$3

if ! show vlan config | grep "Vlan$vlan "; then
    sudo config vlan add $vlan || true
fi

ip=$(ip a s $port | grep "inet " | awk '{print $2}')
if [[ ! -z "$ip" ]]; then
    sudo config interface ip remove $port $ip
fi

if ! show vlan config | grep "Vlan$vlan " | grep "$port "; then
    # TODO: handle when vlan exists on the port but with wrong tagged/untagged
    if [[ "$untagged" == "true" ]]; then
        sudo config vlan member add -u $vlan $port
    else
        sudo config vlan member add $vlan $port
    fi
fi

show vlan config | grep "$port "
show interfaces status | grep "$port "
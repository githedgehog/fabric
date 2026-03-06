#!/bin/bash
# Copyright 2025 Hedgehog
# SPDX-License-Identifier: Apache-2.0

#CUMULUS-AUTOPROVISIONING

set -e
set -o pipefail

function error() {
  echo -e "\e[0;33mERROR: The ZTP script failed while running the command $BASH_COMMAND at line $BASH_LINENO.\e[0m" >&2
  exit 1
}

# Log all output from this script
exec >> /var/log/autoprovision 2>&1
date "+%FT%T ztp starting script $0"

trap error ERR

function ping_until_reachable(){
    last_code=1
    max_tries=60
    tries=0
    while [ "0" != "$last_code" ] && [ "$tries" -lt "$max_tries" ]; do
        tries=$((tries+1))
        echo "$(date) INFO: ( Attempt $tries of $max_tries ) Pinging $1 Target Until Reachable."
        ip vrf exec mgmt ping --no-vrf-switch $1 -c2 &> /dev/null
        last_code=$?
        sleep 1
    done
    if [ "$tries" -eq "$max_tries" ] && [ "$last_code" -ne "0" ]; then
        echo "$(date) ERROR: Reached maximum number of attempts to ping the target $1 ."
        exit 1
    fi
}

function do_ztp() {
  cat > config-ztp.yaml <<'EOF'
{{ $.InitialConfig }}
EOF

  nv config replace config-ztp.yaml
  nv config diff || true
  nv config apply -y

  echo "$(date) INFO: Configuration applied successfully"

  # TODO check if it's needed or affected by agent
  mkdir /var/log/ntpsec/
  chown -R ntpsec /var/log/ntpsec/
  systemctl restart ntp@mgmt.service

  # Wait for control VIP to be reachable before proceeding

  ping_until_reachable {{ $.ControlVIP }}
  echo "$(date) INFO: Control VIP is reachable"

  # Install agent config and agent itself as a systemd unit

  mkdir -p /opt/hedgehog/bin
  wget http://{{ $.ControlVIP }}:32000/agent
  mv agent /opt/hedgehog/bin/
  chmod +x /opt/hedgehog/bin/agent

  mkdir -p /etc/hedgehog
  cat > kubeconfig <<'EOF'
{{ $.KubeConfig }}
EOF
  mv kubeconfig /etc/hedgehog/agent-kubeconfig

  cat > agent-config.yaml <<'EOF'
{{ $.AgentConfig }}
EOF
  mv agent-config.yaml /etc/hedgehog/agent-config.yaml

  /opt/hedgehog/bin/agent install --basedir=/etc/hedgehog --vrf=mgmt
  echo "$(date) INFO: Hedgehog Agent installed"
}

do_ztp

#!/usr/bin/env bash
# Copyright 2026 Hedgehog
# SPDX-License-Identifier: Apache-2.0
#
# breakout-test.sh - exercise every breakout mode of every port of a switch, as
# defined by the switch's SwitchProfile, and report which ones actually work.
#
# For each (port, mode) pair the script:
#   1. patches Switch.spec.portBreakouts[port] = mode (webhook validation applies)
#   2. waits for the controller to propagate it to the Agent and for the agent to
#      report the new generation as applied (status.lastAppliedGen)
#   3. verifies agent status.state:
#        - breakouts[port].mode == mode and status is "Completed" (or empty)
#        - every expected sub-port (port/1 .. port/N) shows up in interfaces with
#          the expected speed
#   4. after all modes of a port are done, restores the port's original breakout and
#      waits for the links that were up before to come back up
#
# Link state (oper up/down) is recorded for information only, it depends on cabling
# and transceivers and isn't required for a PASS.
#
# Requires: kubectl (with access to the fabric API), jq.

set -euo pipefail

SWITCH=""
NS="default"
PORTS_FILTER=""
MODES_FILTER=""
APPLY_TIMEOUT=600
VERIFY_TIMEOUT=240
LINK_TIMEOUT=300
POLL=5
OUT_DIR="."
DRY_RUN=false
INCLUDE_CONNECTED=false
PROFILE_OVERRIDE=""
ASSUME_YES=false
MAX_CONSECUTIVE_APPLY_FAILURES=3
KUBECTL="${KUBECTL:-kubectl}"

# Time estimate defaults (seconds), measured on an Edgecore DCS240 (SONiC 4.5.2, agent v0.144.1).
# A test takes one or two ~23s agent apply/report cycles: 1x modes ~24s, 2x modes 23-46s, 4x/8x ~46s.
# Restoring an explicit breakout took ~24s, going back to the profile default (incl. waiting for the
# breakout to complete) ~45s. A dry run takes ~0.6s per test. PASS timings of previous runs for the
# same switch profile found in the output directory are preferred over these per breakout mode.
EST_SECS_1X=25
EST_SECS_2X=35
EST_SECS_4X_PLUS=46
EST_SECS_RESTORE_EXPLICIT=25
EST_SECS_RESTORE_DEFAULT=45
EST_SECS_DRY_RUN=1
EST_SECS_SETUP=5

SW_RES="switches.wiring.githedgehog.com"
SP_RES="switchprofiles.wiring.githedgehog.com"
CONN_RES="connections.wiring.githedgehog.com"
AG_RES="agents.agent.githedgehog.com"

usage() {
	cat <<EOF
Usage: $(basename "$0") -s SWITCH [options]

Walks through every breakout mode, for every breakout-capable port, defined in the
SwitchProfile of SWITCH, applies it through the Fabric API and checks that the switch
actually ends up in that mode. Prints a report and saves it to a file.

Before starting it prints a time estimate and shows a running ETA while testing. Estimates
come from built-in defaults, or from previous results for the same switch profile in the
output directory, so a quick single-port run (-p) calibrates the estimate for a full run.

Options:
  -s, --switch NAME          Switch name (required)
  -n, --namespace NS         Namespace of the Switch/Agent objects (default: ${NS})
  -p, --ports LIST           Comma-separated ports to test, e.g. E1/1,E1/5 (default: all)
  -m, --modes LIST           Comma-separated breakout modes to test, e.g. 4x100G,8x50G (default: all)
  -P, --profile NAME         Test the breakouts of this SwitchProfile from the cluster instead of the
                             switch's assigned one, and skip checking that the switch platform matches
                             the profile; for prototyping new profiles or similar switches. Note the API
                             and agent still validate/apply against the assigned profile, so ports and
                             modes it doesn't have are reported as REJECTED (default: off)
  -o, --out-dir DIR          Directory for the report and raw results (default: ${OUT_DIR})
      --apply-timeout SEC    Max wait for the agent to apply a change (default: ${APPLY_TIMEOUT})
      --verify-timeout SEC   Max wait for state to converge after apply (default: ${VERIFY_TIMEOUT})
      --link-timeout SEC     After restoring a port, max wait for links that were up before
                             testing it to come back up; aborts if they don't, so the next
                             port isn't disrupted while this one is still down
                             (default: ${LINK_TIMEOUT}, 0 disables)
      --poll SEC             Poll interval (default: ${POLL})
      --include-connected    Also test ports used by Connections (disruptive!)
      --dry-run              Only run server-side validation (kubectl --dry-run=server),
                             nothing is changed on the switch
  -y, --yes                  Don't ask for confirmation
  -h, --help                 Show this help

Environment:
  KUBECTL                    kubectl binary to use (default: kubectl)

Exit code: 0 if every tested combination passed, 1 if anything failed, 2 on usage/setup errors.
EOF
}

die() {
	printf 'ERROR: %s\n' "$*" >&2
	exit 2
}

log() {
	printf '%s %s\n' "$(date +%H:%M:%S)" "$*" >&2
}

k() {
	"$KUBECTL" -n "$NS" "$@"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	-s | --switch) SWITCH="$2"; shift 2 ;;
	-n | --namespace) NS="$2"; shift 2 ;;
	-p | --ports) PORTS_FILTER="$2"; shift 2 ;;
	-m | --modes) MODES_FILTER="$2"; shift 2 ;;
	-P | --profile) PROFILE_OVERRIDE="$2"; shift 2 ;;
	-o | --out-dir) OUT_DIR="$2"; shift 2 ;;
	--apply-timeout) APPLY_TIMEOUT="$2"; shift 2 ;;
	--verify-timeout) VERIFY_TIMEOUT="$2"; shift 2 ;;
	--link-timeout) LINK_TIMEOUT="$2"; shift 2 ;;
	--poll) POLL="$2"; shift 2 ;;
	--include-connected) INCLUDE_CONNECTED=true; shift ;;
	--dry-run) DRY_RUN=true; shift ;;
	-y | --yes) ASSUME_YES=true; shift ;;
	-h | --help) usage; exit 0 ;;
	*) usage >&2; die "unknown argument: $1" ;;
	esac
done

[[ -n "$SWITCH" ]] || { usage >&2; die "--switch is required"; }
command -v jq >/dev/null || die "jq is required"
command -v "$KUBECTL" >/dev/null || die "kubectl ($KUBECTL) is required"
mkdir -p "$OUT_DIR"

TS="$(date +%Y%m%d-%H%M%S)"
BASE="${OUT_DIR}/breakout-report-${SWITCH}-${TS}"
REPORT_FILE="${BASE}.txt"
RESULTS_FILE="${BASE}.tsv"
BACKUP_FILE="${BASE}.orig-portBreakouts.json"
STARTED="$(date -Is)"
START_EPOCH="$(date +%s)"

# ---------------------------------------------------------------------------
# Load switch, profile and agent
# ---------------------------------------------------------------------------

SW_JSON="$(k get "$SW_RES" "$SWITCH" -o json)" || die "can't get switch $SWITCH in namespace $NS"
ASSIGNED_PROFILE="$(jq -r '.spec.profile // ""' <<<"$SW_JSON")"
PROFILE="${PROFILE_OVERRIDE:-$ASSIGNED_PROFILE}"
[[ -n "$PROFILE" ]] || die "switch $SWITCH has no spec.profile, pass one with --profile"
SP_JSON="$(k get "$SP_RES" "$PROFILE" -o json)" || die "can't get switch profile $PROFILE (is it registered in the catalog?)"
AG_JSON="$(k get "$AG_RES" "$SWITCH" -o json)" || die "can't get agent $SWITCH"

PROFILE_DISPLAY="$(jq -r '.spec.displayName // ""' <<<"$SP_JSON")"
PLATFORM="$(jq -r '.spec.platform // ""' <<<"$SP_JSON")"
SWITCH_PLATFORM="$(jq -r '.status.state.nos.platformName // ""' <<<"$AG_JSON")"

# Make sure the switch hardware is what the profile describes, unless the user picked the
# profile explicitly (prototyping a new profile, or a switch similar to a profiled one)
if [[ -z "$PROFILE_OVERRIDE" ]]; then
	PROFILE_CHECK="switch platform matches profile"
	if [[ -z "$SWITCH_PLATFORM" ]]; then
		PROFILE_CHECK="not possible, agent doesn't report the switch platform"
		log "WARNING: agent doesn't report the switch platform, can't check it matches profile $PROFILE"
	elif [[ "$SWITCH_PLATFORM" != "$PLATFORM" ]]; then
		die "switch $SWITCH reports platform $SWITCH_PLATFORM, but its profile $PROFILE is for $PLATFORM;" \
			"use --profile to test against a profile anyway (e.g. a new profile or a similar switch)"
	fi
else
	PROFILE_CHECK="skipped (--profile)"
	if [[ "$PROFILE" != "$ASSIGNED_PROFILE" ]]; then
		log "WARNING: testing the breakouts of profile $PROFILE, but $SWITCH is assigned profile $ASSIGNED_PROFILE;"
		log "WARNING: the API and the agent still validate and apply breakouts using $ASSIGNED_PROFILE, so ports"
		log "WARNING: and modes that $ASSIGNED_PROFILE doesn't have will be REJECTED"
	fi
fi
NOS_VERSION="$(jq -r '.status.state.nos.softwareVersion // "unknown"' <<<"$AG_JSON")"
AGENT_VERSION="$(jq -r '.status.version // "unknown"' <<<"$AG_JSON")"
ORIG_BREAKOUTS="$(jq -c '.spec.portBreakouts // {}' <<<"$SW_JSON")"

if jq -e '.spec.roce == true' <<<"$SW_JSON" >/dev/null; then
	die "switch $SWITCH has RoCE enabled, the API rejects any port breakout change while RoCE is on; disable it first"
fi

# Test plan: port <TAB> port profile <TAB> default mode <TAB> mode <TAB> #sub-ports <TAB> sub-port speed
PLAN="$(jq -r '
	.spec.portProfiles as $pp
	| .spec.ports | to_entries[]
	| select((.value.profile // "") != "" and $pp[.value.profile].breakout != null)
	| .key as $port | .value.profile as $prof | $pp[$prof].breakout as $b
	| $b.supported | to_entries[]
	| [$port, $prof, $b.default, .key, (.value.offsets | length), (.key | split("x")[1])]
	| @tsv' <<<"$SP_JSON" | sort -t$'\t' -k1,1V -k5,5n -k6,6V)"

[[ -n "$PLAN" ]] || die "profile $PROFILE has no breakout-capable ports"

in_list() { # in_list item comma,separated,list
	[[ ",$2," == *",$1,"* ]]
}

# Ports used by Connections on this switch (normalized to the parent port, E1/1/2 -> E1/1)
CONNECTED_PORTS="$(k get "$CONN_RES" -o json | jq -r --arg sw "${SWITCH}/" '
	[.. | strings | select(startswith($sw)) | ltrimstr($sw) | split("/")[0:2] | join("/")] | unique | join(",")')"

if [[ -n "$PORTS_FILTER" ]]; then
	PLAN="$(awk -F'\t' -v l=",${PORTS_FILTER}," 'index(l, "," $1 ",")' <<<"$PLAN")"
fi
if [[ -n "$MODES_FILTER" ]]; then
	PLAN="$(awk -F'\t' -v l=",${MODES_FILTER}," 'index(l, "," $4 ",")' <<<"$PLAN")"
fi
[[ -n "$PLAN" ]] || die "nothing to test after applying --ports/--modes filters"

# Ports / modes under test (rows / columns of the report matrix), in display order:
# ports naturally sorted, modes by number of sub-ports and then by speed
TESTED_PORTS="$(cut -f1 <<<"$PLAN" | sort -uV)"
TESTED_MODES="$(cut -f4 <<<"$PLAN" | sort -u | awk -F'x' '{print $1 "\t" $2 "\t" $0}' | sort -k1,1n -k2,2V | cut -f3)"
TOTAL_TESTS="$(wc -l <<<"$PLAN")"


# ---------------------------------------------------------------------------
# Helpers talking to the API
# ---------------------------------------------------------------------------

# patch_breakouts JSON_OBJECT [extra kubectl args...]; JSON null value removes the override
patch_breakouts() {
	local obj="$1"
	shift
	# stderr is captured on purpose (error message), stdout dropped
	# shellcheck disable=SC2069
	k patch "$SW_RES" "$SWITCH" --type=merge "$@" \
		-p "$(jq -nc --argjson o "$obj" '{spec: {portBreakouts: $o}}')" 2>&1 >/dev/null
}

orig_value_json() { # original breakout of a port as a JSON value (null = not set)
	jq -c --arg p "$1" '.[$p] // null' <<<"$ORIG_BREAKOUTS"
}

# wait_applied: waits until the agent got the switch's current portBreakouts and applied it.
# Sets WAIT_REASON on failure. Runs in the main shell (not in $(...)) on purpose: bash only runs
# the INT/TERM trap once the current foreground command returns, so a subshell looping for up to
# APPLY_TIMEOUT would delay the cleanup on interrupt.
# Prints a reason on failure.
wait_applied() {
	local desired deadline ag gen
	WAIT_REASON=""
	desired="$(k get "$SW_RES" "$SWITCH" -o json | jq -c '.spec.portBreakouts // {}')"
	deadline=$(($(date +%s) + APPLY_TIMEOUT))

	gen=""
	while (($(date +%s) < deadline)); do
		ag="$(k get "$AG_RES" "$SWITCH" -o json 2>/dev/null)" || { sleep "$POLL"; continue; }
		if [[ -z "$gen" ]]; then
			if jq -e --argjson d "$desired" '(.spec.switch.portBreakouts // {}) == $d' <<<"$ag" >/dev/null; then
				gen="$(jq -r '.metadata.generation' <<<"$ag")"
			fi
		fi
		if [[ -n "$gen" ]] && jq -e --argjson g "$gen" '(.status.lastAppliedGen // 0) >= $g' <<<"$ag" >/dev/null; then
			return 0
		fi
		sleep "$POLL"
	done

	if [[ -z "$gen" ]]; then
		WAIT_REASON="controller didn't propagate change to agent within ${APPLY_TIMEOUT}s"
	else
		WAIT_REASON="$(printf "agent didn't apply gen %s within %ss (lastAttemptGen=%s, lastAppliedGen=%s)" "$gen" "$APPLY_TIMEOUT" \
			"$(jq -r '.status.lastAttemptGen // 0' <<<"$ag")" "$(jq -r '.status.lastAppliedGen // 0' <<<"$ag")")"
	fi
	return 1
}

# check_state PORT MODE NSUB SPEED -> TSV: verdict <TAB> oper-up-count <TAB> detail
# verdict: PASS | FAIL (definitive) | WAIT (not converged yet)
check_state() {
	k get "$AG_RES" "$SWITCH" -o json | jq -r --arg p "$1" --arg m "$2" --argjson n "$3" --arg s "$4" '
		.status.state as $st
		| ($st.breakouts[$p] // {}) as $b
		| def iface($name): ($st.interfaces[$name] // (if $n == 1 then $st.interfaces[$p] else null end));
		  [range(1; $n + 1) | "\($p)/\(.)"] as $subs
		| [$subs[] | select(iface(.) == null)] as $missing
		| [$subs[] | select(iface(.) != null and (iface(.).speed // "") != $s) | "\(.)=\(iface(.).speed // "none")"] as $badspeed
		| ([$subs[] | select(iface(.).oper == "up")] | length) as $up
		| (if ($b.status // "" | ascii_downcase | test("fail|error")) then
			["FAIL", "breakout status \($b.status) (mode=\($b.mode // "none"))"]
		  elif ($b.mode // "") != $m then
			["WAIT", "breakout mode is \($b.mode // "none"), status \($b.status // "none")"]
		  elif ($b.status // "") as $bs | ($bs != "" and $bs != "Completed") then
			["WAIT", "breakout status \($b.status)"]
		  elif ($missing | length) > 0 then
			["WAIT", "missing interfaces: \($missing | join(" "))"]
		  elif ($badspeed | length) > 0 then
			["WAIT", "speed mismatch (want \($s)): \($badspeed | join(" "))"]
		  else
			["PASS", "\($n) sub-port(s) at \($s)"]
		  end) as $v
		| [$v[0], "\($up)/\($n)", $v[1]] | @tsv'
}

# record PORT PORT-PROFILE MODE RESULT SECONDS OPERUP DETAIL; the switch profile is appended as
# the 8th column so later runs can use the timings for their estimates
record() {
	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$@" "$PROFILE" >>"$RESULTS_FILE"
}

# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------

build_report() {
	local finished duration
	finished="$(date -Is)"
	duration=$(($(date +%s) - START_EPOCH))

	{
		echo "Breakout capability report"
		echo "=========================="
		printf '%-14s %s (namespace %s)\n' "Switch:" "$SWITCH" "$NS"
		printf '%-14s %s (%s), platform %s\n' "Profile:" "$PROFILE" "$PROFILE_DISPLAY" "$PLATFORM"
		[[ "$PROFILE" != "$ASSIGNED_PROFILE" ]] && printf '%-14s %s (used by the API and agent)\n' "Assigned:" "$ASSIGNED_PROFILE"
		printf '%-14s %s\n' "Switch HW:" "${SWITCH_PLATFORM:-unknown}"
		printf '%-14s %s\n' "Profile check:" "$PROFILE_CHECK"
		printf '%-14s %s\n' "NOS version:" "$NOS_VERSION"
		printf '%-14s %s\n' "Agent version:" "$AGENT_VERSION"
		printf '%-14s %s\n' "Run mode:" "$($DRY_RUN && echo "DRY RUN - server-side API validation only, switch not changed" || echo "live")"
		printf '%-14s %s -> %s (%s, estimated ~%s)\n' "Time:" "$STARTED" "$finished" "$(fmt_dur "$duration")" "$(fmt_dur "${EST_TOTAL:-0}")"
		printf '%-14s %s\n' "Raw results:" "$RESULTS_FILE"
		[[ -n "${ABORTED:-}" ]] && printf '%-14s %s\n' "ABORTED:" "$ABORTED"
		echo

		awk -F'\t' -v total="$TOTAL_TESTS" '
			{ n++; c[$4]++ }
			END {
				printf "Summary: %d of %d planned tests run: %d PASS, %d FAIL, %d REJECTED, %d TIMEOUT, %d SKIPPED\n",
					n, total, c["PASS"], c["FAIL"], c["REJECTED"], c["TIMEOUT"], c["SKIPPED"]
			}' "$RESULTS_FILE"
		echo

		echo "Matrix (rows: ports, columns: breakout modes, * marks the profile default)"
		echo
		# shellcheck disable=SC2016
		awk -F'\t' -v ports="$(tr '\n' ' ' <<<"$TESTED_PORTS")" -v modes="$(tr '\n' ' ' <<<"$TESTED_MODES")" \
			-v supported="$(jq -r '.spec.ports as $ps | .spec.portProfiles as $pp
				| $ps | to_entries[] | select((.value.profile // "") != "" and $pp[.value.profile].breakout != null)
				| .key as $k | $pp[.value.profile].breakout as $b
				| ($b.supported | keys[] | "\($k)|\(.)"), "\($k)|\($b.default)|default"' <<<"$SP_JSON" | tr '\n' ' ')" '
			BEGIN {
				np = split(ports, P, " "); nm = split(modes, M, " "); ns = split(supported, S, " ")
				for (i = 1; i <= ns; i++) {
					if (S[i] ~ /\|default$/) { sub(/\|default$/, "", S[i]); def[S[i]] = 1 } else sup[S[i]] = 1
				}
				abbr["PASS"] = "ok"; abbr["FAIL"] = "FAIL"; abbr["REJECTED"] = "REJ"; abbr["TIMEOUT"] = "TMO"; abbr["SKIPPED"] = "skip"
			}
			{ res[$1 "|" $3] = abbr[$4] }
			END {
				pw = 6
				for (i = 1; i <= np; i++) if (length(P[i]) > pw) pw = length(P[i])
				line = sprintf("%-" pw "s |", "Port")
				for (j = 1; j <= nm; j++) { w[j] = length(M[j]) + 1; if (w[j] < 5) w[j] = 5; line = line sprintf(" %-" w[j] "s", M[j]) }
				print line
				sep = ""; for (i = 0; i < length(line); i++) sep = sep "-"; print sep
				for (i = 1; i <= np; i++) {
					line = sprintf("%-" pw "s |", P[i])
					for (j = 1; j <= nm; j++) {
						key = P[i] "|" M[j]
						if (!(key in sup)) cell = "n/a"
						else if (key in res) cell = res[key]
						else cell = "."
						if (key in def) cell = cell "*"
						line = line sprintf(" %-" w[j] "s", cell)
					}
					print line
				}
			}' "$RESULTS_FILE"
		echo
		echo "Legend: ok = passed, FAIL = applied but switch state wrong, REJ = rejected by API validation,"
		echo "        TMO = agent didn't apply in time, skip = port used by a Connection, . = not tested, n/a = not in profile"
		echo

		echo "Per-mode summary"
		echo
		awk -F'\t' -v modes="$(tr '\n' ' ' <<<"$TESTED_MODES")" '
			BEGIN { nm = split(modes, M, " ") }
			$4 != "SKIPPED" { t[$3]++; if ($4 == "PASS") p[$3]++; else bad[$3] = bad[$3] " " $1 }
			END {
				printf "%-10s %-12s %s\n", "Mode", "Pass/Tested", "Failing ports"
				printf "%-10s %-12s %s\n", "----", "-----------", "-------------"
				for (j = 1; j <= nm; j++) {
					m = M[j]; if (!(m in t)) continue
					v = (p[m] == t[m]) ? "-" : substr(bad[m], 2)
					printf "%-10s %-12s %s\n", m, sprintf("%d/%d", p[m], t[m]), v
				}
			}' "$RESULTS_FILE"
		echo

		if awk -F'\t' '$4 != "PASS" && $4 != "SKIPPED" { found = 1 } END { exit !found }' "$RESULTS_FILE"; then
			echo "Failure details"
			echo
			awk -F'\t' '$4 != "PASS" && $4 != "SKIPPED" { printf "%-8s %-8s %-9s %s\n", $1, $3, $4, $7 }' "$RESULTS_FILE" | sort -k1,1V
			echo
		fi

		if awk -F'\t' '$4 == "SKIPPED" { found = 1 } END { exit !found }' "$RESULTS_FILE"; then
			echo "Skipped ports (used by Connections, use --include-connected to test them):"
			awk -F'\t' '$4 == "SKIPPED" { print $1 }' "$RESULTS_FILE" | sort -uV | tr '\n' ' '
			echo
			echo
		fi

		if ! $DRY_RUN; then
			echo "Link state of passed tests (informational, depends on cabling/transceivers): oper up sub-ports"
			awk -F'\t' '$4 == "PASS" { split($6, a, "/"); up += a[1]; all += a[2] } END { printf "  %d of %d sub-ports were oper up\n", up, all }' "$RESULTS_FILE"
		fi
	} >"$REPORT_FILE"
}

print_report() {
	if [[ -t 1 ]]; then
		sed -E -e 's/(^| )ok(\*?)( |$)/\1\x1b[32mok\x1b[0m\2\3/g' \
			-e 's/(^| )(FAIL|REJ|TMO)(\*?)( |$)/\1\x1b[31m\2\x1b[0m\3\4/g' "$REPORT_FILE"
	else
		cat "$REPORT_FILE"
	fi
	echo
	echo "Report saved to ${REPORT_FILE}"
}

# ---------------------------------------------------------------------------
# Cleanup: always try to put the switch back the way we found it
# ---------------------------------------------------------------------------

TOUCHED_PORTS=""
FINISHED=false

restore_all() {
	[[ -z "$TOUCHED_PORTS" ]] && return 0
	local obj='{}' p out
	for p in $TOUCHED_PORTS; do
		obj="$(jq -c --arg p "$p" --argjson v "$(orig_value_json "$p")" '. + {($p): $v}' <<<"$obj")"
	done
	log "Restoring original breakouts for: ${TOUCHED_PORTS}"
	if ! out="$(patch_breakouts "$obj")"; then
		log "WARNING: failed to restore breakouts: $out"
		log "WARNING: original spec.portBreakouts saved in $BACKUP_FILE"
		return 1
	fi
	TOUCHED_PORTS=""
}

on_exit() {
	local rc=$?
	trap - EXIT INT TERM
	if ! $FINISHED; then
		ABORTED="${ABORTED:-interrupted (exit code $rc)}"
		$DRY_RUN || restore_all || true
		build_report
		print_report
	fi
	exit "$rc"
}

# ---------------------------------------------------------------------------
# Time estimate
# ---------------------------------------------------------------------------

fmt_dur() { # fmt_dur SECONDS -> 1h02m / 4m05s / 12s
	local s="$1"
	if ((s >= 3600)); then
		printf '%dh%02dm' $((s / 3600)) $((s % 3600 / 60))
	elif ((s >= 60)); then
		printf '%dm%02ds' $((s / 60)) $((s % 60))
	else
		printf '%ds' "$s"
	fi
}

# Median PASS duration per breakout mode from previous live runs of the same switch profile
# (dry runs record 0s and are ignored): mode <TAB> median seconds <TAB> samples
# shellcheck disable=SC2016 # awk program, not shell expansions
HISTORY="$(find "$OUT_DIR" -maxdepth 1 -name 'breakout-report-*.tsv' ! -name "$(basename "$RESULTS_FILE")" -print0 |
	xargs -0 -r awk -F'\t' -v p="$PROFILE" '$8 == p && $4 == "PASS" && $5 > 0 { print $3 "\t" $5 }' |
	sort -t$'\t' -k1,1 -k2,2n |
	awk -F'\t' '{ v[$1, ++n[$1]] = $2 } END { for (m in n) print m "\t" v[m, int((n[m] + 1) / 2)] "\t" n[m] }')"
declare -A HIST_SECS=()
HIST_SAMPLES=0
while IFS=$'\t' read -r m secs cnt; do
	[[ -n "$m" ]] || continue
	HIST_SECS[$m]="$secs"
	HIST_SAMPLES=$((HIST_SAMPLES + cnt))
done <<<"$HISTORY"

declare -a EST_TEST=()   # test number -> estimated seconds
declare -A EST_RESTORE=() # port -> estimated seconds to restore it
EST_TOTAL=$EST_SECS_SETUP
est_modes_from_history=""
est_modes_default=""
i=0
while IFS=$'\t' read -r port _ _ mode nsub _; do
	i=$((i + 1))
	if ! $INCLUDE_CONNECTED && in_list "$port" "$CONNECTED_PORTS"; then
		secs=0
	elif $DRY_RUN; then
		secs=$EST_SECS_DRY_RUN
	else
		if [[ -n "${HIST_SECS[$mode]:-}" ]]; then
			secs="${HIST_SECS[$mode]}"
			in_list "$mode" "$est_modes_from_history" || est_modes_from_history+="$mode,"
		else
			if ((nsub <= 1)); then secs=$EST_SECS_1X; elif ((nsub <= 2)); then secs=$EST_SECS_2X; else secs=$EST_SECS_4X_PLUS; fi
			in_list "$mode" "$est_modes_default" || est_modes_default+="$mode,"
		fi
		if [[ -z "${EST_RESTORE[$port]:-}" ]]; then
			if [[ "$(orig_value_json "$port")" == "null" ]]; then
				EST_RESTORE[$port]=$EST_SECS_RESTORE_DEFAULT
			else
				EST_RESTORE[$port]=$EST_SECS_RESTORE_EXPLICIT
			fi
			EST_TOTAL=$((EST_TOTAL + EST_RESTORE[$port]))
		fi
	fi
	EST_TEST[i]=$secs
	EST_TOTAL=$((EST_TOTAL + secs))
done <<<"$PLAN"

if $DRY_RUN; then
	EST_SOURCE="~${EST_SECS_DRY_RUN}s per validation request"
elif [[ -z "$est_modes_from_history" ]]; then
	EST_SOURCE="built-in defaults measured on an Edgecore DCS240; run one port first (-p) to calibrate for this switch"
else
	n_hist="$(tr ',' '\n' <<<"${est_modes_from_history%,}" | wc -l)"
	n_all=$((n_hist + $(tr ',' '\n' <<<"${est_modes_default%,}" | grep -c . || true)))
	EST_SOURCE="previous $PROFILE results in $OUT_DIR ($HIST_SAMPLES samples) for $n_hist of $n_all modes"
	[[ -n "$est_modes_default" ]] && EST_SOURCE+=", built-in defaults for the rest"
fi

# Ports whose links are up now: the link recovery wait after restoring them isn't estimated
LINKS_UP_PORTS=""
if ! $DRY_RUN; then
	LINKS_UP_PORTS="$(jq -r --arg ports "$(tr '\n' ' ' <<<"$TESTED_PORTS")" --arg skip ",$CONNECTED_PORTS," --argjson inc "$INCLUDE_CONNECTED" '
		(.status.state.interfaces // {}) as $ifs
		| [$ports | split(" ")[] | select(. != "") | . as $p
			| select($inc or ($skip | contains("," + $p + ",") | not))
			| select([$ifs | to_entries[] | select((.key == $p or (.key | startswith($p + "/"))) and .value.oper == "up")] | length > 0)]
		| join(" ")' <<<"$AG_JSON")"
fi

# eta_info -> "elapsed X, ~Y left, ETA HH:MM"; the remaining estimate is scaled by how fast the run
# has actually been going so far, once there's enough of it to go by
eta_info() {
	local elapsed remaining left
	elapsed=$(($(date +%s) - LOOP_START))
	remaining=$((EST_TOTAL - EST_SECS_SETUP - EST_DONE))
	((remaining < 0)) && remaining=0
	if ((EST_DONE >= 60 && elapsed > 0)); then
		left=$((remaining * elapsed / EST_DONE))
	else
		left=$remaining
	fi
	printf 'elapsed %s, ~%s left, ETA %s' "$(fmt_dur "$elapsed")" "$(fmt_dur "$left")" "$(date -d "+${left} sec" +%H:%M)"
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

cat >&2 <<EOF
Switch:          $SWITCH (namespace $NS)
Profile:         $PROFILE ($PROFILE_DISPLAY), platform $PLATFORM
Assigned:        ${ASSIGNED_PROFILE:-none}$([[ "$PROFILE" != "$ASSIGNED_PROFILE" ]] && echo " (DIFFERENT from the tested profile)")
Switch HW:       ${SWITCH_PLATFORM:-unknown}
Profile check:   $PROFILE_CHECK
NOS:             $NOS_VERSION, agent $AGENT_VERSION
Ports to test:   $(wc -l <<<"$TESTED_PORTS") ($(tr '\n' ' ' <<<"$TESTED_PORTS"))
Modes to test:   $(wc -l <<<"$TESTED_MODES") ($(tr '\n' ' ' <<<"$TESTED_MODES"))
Total tests:     $TOTAL_TESTS
Connected ports: ${CONNECTED_PORTS:-none} $($INCLUDE_CONNECTED && echo "(WILL BE TESTED)" || echo "(will be skipped)")
Mode:            $($DRY_RUN && echo "dry run (server-side validation only)" || echo "LIVE - ports will flap, traffic on tested ports will be disrupted")
Output:          $REPORT_FILE
Estimated time:  ~$(fmt_dur "$EST_TOTAL"), finishing around $(date -d "+${EST_TOTAL} sec" '+%a %H:%M') if started now
                 (${EST_SOURCE})
EOF
if [[ -n "$LINKS_UP_PORTS" ]]; then
	echo "                 + link recovery after restoring ports with links up (not estimated): $LINKS_UP_PORTS" >&2
fi

if ! $DRY_RUN; then
	ag_gen="$(jq -r '.metadata.generation' <<<"$AG_JSON")"
	ag_applied="$(jq -r '.status.lastAppliedGen // 0' <<<"$AG_JSON")"
	if [[ "$ag_gen" != "$ag_applied" ]]; then
		log "Agent is not in sync (generation=$ag_gen, lastAppliedGen=$ag_applied), waiting for it first..."
		wait_applied || die "agent not in sync: $WAIT_REASON"
	fi
fi

if ! $ASSUME_YES; then
	if [[ -t 0 ]]; then
		read -r -p "Proceed? [y/N] " answer
		[[ "$answer" == [yY]* ]] || die "aborted by user"
	else
		die "not running interactively, pass --yes to proceed"
	fi
fi

: >"$RESULTS_FILE"
printf '%s\n' "$ORIG_BREAKOUTS" >"$BACKUP_FILE"

# from here on the switch gets modified: always restore it and write the report on exit
trap on_exit EXIT
trap 'ABORTED="interrupted by signal"; exit 130' INT TERM

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------

consecutive_apply_failures=0
test_no=0
current_port=""

finish_port() { # restore a port to its original breakout once all its modes are tested
	local port="$1" out reason
	$DRY_RUN && return 0
	[[ " $TOUCHED_PORTS " == *" $port "* ]] || return 0
	log "[$port] restoring original breakout ($(orig_value_json "$port" | jq -r '. // "profile default"'))"
	if ! out="$(patch_breakouts "{\"$port\": $(orig_value_json "$port")}")"; then
		ABORTED="failed to restore $port: $out"
		exit 1
	fi
	TOUCHED_PORTS="$(tr ' ' '\n' <<<"$TOUCHED_PORTS" | grep -vxF "$port" | tr '\n' ' ' || true)"
	TOUCHED_PORTS="${TOUCHED_PORTS% }"
	if ! wait_applied; then
		reason="$WAIT_REASON"
		ABORTED="restore of $port wasn't applied: $reason"
		exit 1
	fi

	# the agent reports "applied" once the config is pushed, the breakout itself may still
	# be in progress on the switch (seen taking minutes when going back to the default)
	local orig deadline st
	orig="$(orig_value_json "$port")"
	deadline=$(($(date +%s) + VERIFY_TIMEOUT))
	while :; do
		st="$(k get "$AG_RES" "$SWITCH" -o json | jq -r --arg p "$port" --argjson o "$orig" '
			(.status.state.breakouts // {})[$p] as $b
			| if ($b.status // "") == "InProgress" then "breakout in progress"
			  elif $o != null and ($b.mode // "") != $o then "breakout mode is \($b.mode // "none"), want \($o)"
			  else "" end')" || st="failed to read agent state"
		[[ -z "$st" ]] && break
		if (($(date +%s) >= deadline)); then
			ABORTED="restore of $port didn't complete within ${VERIFY_TIMEOUT}s: $st"
			exit 1
		fi
		sleep "$POLL"
	done

	# don't move on (and possibly disrupt the next port, e.g. the other uplink) until
	# the links that were up before testing this port are up again
	if [[ -n "$PORT_UP_BEFORE" ]] && ((LINK_TIMEOUT > 0)); then
		local now_up missing
		deadline=$(($(date +%s) + LINK_TIMEOUT))
		log "[$port] waiting up to ${LINK_TIMEOUT}s for links to come back up: $PORT_UP_BEFORE"
		while :; do
			now_up=" $(up_subports "$port" || true) "
			missing=""
			for p in $PORT_UP_BEFORE; do
				[[ "$now_up" == *" $p "* ]] || missing="$missing $p"
			done
			[[ -z "$missing" ]] && break
			if (($(date +%s) >= deadline)); then
				ABORTED="links on $port didn't come back up within ${LINK_TIMEOUT}s after restore:$missing"
				exit 1
			fi
			sleep "$POLL"
		done
		log "[$port] links are up again"
	fi
}

# up_subports PORT -> space-separated interfaces of PORT (PORT or PORT/N) that are oper up
up_subports() {
	k get "$AG_RES" "$SWITCH" -o json | jq -r --arg p "$1" '
		[(.status.state.interfaces // {}) | to_entries[]
			| select((.key == $p or (.key | startswith($p + "/"))) and .value.oper == "up") | .key] | sort | join(" ")'
}

PORT_UP_BEFORE=""
LOOP_START="$(date +%s)"
EST_DONE=0 # estimated seconds of the work done so far, used to scale the ETA

while IFS=$'\t' read -r -u 3 port prof _default mode nsub speed; do
	((test_no > 0)) && EST_DONE=$((EST_DONE + EST_TEST[test_no]))
	test_no=$((test_no + 1))

	if [[ "$port" != "$current_port" ]]; then
		if [[ -n "$current_port" ]]; then
			finish_port "$current_port"
			EST_DONE=$((EST_DONE + ${EST_RESTORE[$current_port]:-0}))
		fi
		current_port="$port"
		PORT_UP_BEFORE=""
		if ! $DRY_RUN; then
			PORT_UP_BEFORE="$(up_subports "$port")" || die "can't read agent state"
		fi
	fi

	prefix="[$test_no/$TOTAL_TESTS] $port $mode"

	if ! $INCLUDE_CONNECTED && in_list "$port" "$CONNECTED_PORTS"; then
		log "$prefix: SKIPPED (port is used by a Connection)"
		record "$port" "$prof" "$mode" SKIPPED 0 - "port is used by a Connection"
		continue
	fi

	t0="$(date +%s)"

	if $DRY_RUN; then
		if out="$(patch_breakouts "{\"$port\": \"$mode\"}" --dry-run=server)"; then
			log "$prefix: PASS (validation)"
			record "$port" "$prof" "$mode" PASS 0 - "accepted by API validation"
		else
			out="$(tr '\n\t' '  ' <<<"$out")"
			log "$prefix: REJECTED: $out"
			record "$port" "$prof" "$mode" REJECTED 0 - "$out"
		fi
		continue
	fi

	log "$prefix: applying ($(eta_info))"
	if ! out="$(patch_breakouts "{\"$port\": \"$mode\"}")"; then
		out="$(tr '\n\t' '  ' <<<"$out")"
		log "$prefix: REJECTED: $out"
		record "$port" "$prof" "$mode" REJECTED 0 - "$out"
		continue
	fi
	[[ " $TOUCHED_PORTS " == *" $port "* ]] || TOUCHED_PORTS="${TOUCHED_PORTS:+$TOUCHED_PORTS }$port"

	if ! wait_applied; then
		reason="$WAIT_REASON"
		consecutive_apply_failures=$((consecutive_apply_failures + 1))
		log "$prefix: TIMEOUT: $reason"
		record "$port" "$prof" "$mode" TIMEOUT $(($(date +%s) - t0)) - "$reason"
		if ((consecutive_apply_failures >= MAX_CONSECUTIVE_APPLY_FAILURES)); then
			ABORTED="agent failed to apply $consecutive_apply_failures changes in a row, check the agent/switch"
			exit 1
		fi
		continue
	fi
	consecutive_apply_failures=0
	t_applied="$(date +%s)"

	deadline=$((t_applied + VERIFY_TIMEOUT))
	verdict="WAIT"
	while :; do
		if st="$(check_state "$port" "$mode" "$nsub" "$speed")"; then
			IFS=$'\t' read -r verdict operup detail <<<"$st"
			[[ "$verdict" != "WAIT" ]] && break
		else
			operup="-"
			detail="failed to read agent state"
		fi
		(($(date +%s) >= deadline)) && break
		sleep "$POLL"
	done

	elapsed=$(($(date +%s) - t0))
	if [[ "$verdict" == "PASS" ]]; then
		log "$prefix: PASS in ${elapsed}s ($detail, oper up $operup)"
		record "$port" "$prof" "$mode" PASS "$elapsed" "$operup" "$detail"
	else
		[[ "$verdict" == "WAIT" ]] && detail="not converged after ${VERIFY_TIMEOUT}s: $detail"
		log "$prefix: FAIL in ${elapsed}s: $detail"
		record "$port" "$prof" "$mode" FAIL "$elapsed" "$operup" "$detail"
	fi
done 3<<<"$PLAN"

[[ -n "$current_port" ]] && finish_port "$current_port"

FINISHED=true
build_report
print_report

awk -F'\t' '$4 != "PASS" && $4 != "SKIPPED" { bad = 1 } END { exit bad }' "$RESULTS_FILE"

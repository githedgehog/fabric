# Generated bindings

## Old notes

Preparation to run the generator with new YANG files: (pre 4.4.0)

Comment/remove following sections from file yang/extensions/openconfig-platform-ext.yang in section
"augment /oc-pf:components/oc-pf:component/oc-transceiver:transceiver/oc-transceiver:state", line 148):
cable-length, max-port-power, max-module-power, display-name, vendor-oui, revision-compliance

Comment/remove section "augment /oc-stp:stp/oc-stp:mstp/oc-stp:state" in yang/extensions/openconfig-spanning-tree-ext.yang:446:9

# Switch Catalog

The following is a list of all supported switches with their supported capabilities and configuration. Please, make sure
to use the version of documentation that matches your environment to get an up-to-date list of supported switches, their
features and port naming scheme.


| Switch | Supported Roles | Silicon | Ports |
|--------|-----------------|---------|-------|
| [Celestica DS2000 (Questone 2a)](#celestica-ds2000) | **spine**, **leaf** | Broadcom TD3-X5 2.0T | 48xSFP28-25G, 8xQSFP28-100G |
| [Celestica DS3000 (Seastone2)](#celestica-ds3000) | **spine**, **leaf** | Broadcom TD3-X7 3.2T | 32xQSFP28-100G, 1xSFP28-10G |
| [Celestica DS4000 (Silverstone2)](#celestica-ds4000) | **spine** | Broadcom TH3 | 32xQSFPDD-400G, 1xSFP28-10G |
| [Celestica DS4101 (Greystone)](#celestica-ds4101) | **spine** | Broadcom TH4G | 32xOSFP-2x400G, 2xSFP28-10G |
| [Celestica DS5000 (Moonstone)](#celestica-ds5000) | **spine**, **leaf (l3-only)** | Broadcom TH5 | 64xOSFP-800G, 2xSFP28-25G |
| [Dell S5232F-ON](#dell-s5232f-on) | **spine**, **leaf** | Broadcom TD3-X7 3.2T | 32xQSFP28-100G, 2xSFP28-10G |
| [Dell S5248F-ON](#dell-s5248f-on) | **spine**, **leaf** | Broadcom TD3-X7 3.2T | 48xSFP28-25G, 8xQSFP28-100G |
| [Dell Z9332F-ON](#dell-z9332f-on) | **spine** | Broadcom TH3 | 32xQSFPDD-400G, 2xSFP28-10G |
| [Edgecore DCS203 (AS7326-56X)](#edgecore-dcs203) | **spine**, **leaf** | Broadcom TD3-X7 2.0T | 48xSFP28-25G, 8xQSFP28-100G, 2xSFP28-10G |
| [Edgecore DCS204 (AS7726-32X)](#edgecore-dcs204) | **spine**, **leaf** | Broadcom TD3-X7 3.2T | 32xQSFP28-100G, 2xSFP28-10G |
| [Edgecore DCS501 (AS7712-32X)](#edgecore-dcs501) | **spine** | Broadcom TH | 32xQSFP28-100G |
| [Edgecore EPS203 (AS4630-54NPE)](#edgecore-eps203) | **leaf (limited)** | Broadcom TD3-X3 | 36xRJ45-2.5G, 12xRJ45-10G, 4xSFP28-25G, 2xQSFP28-100G |
| [Supermicro SSE-C4632SB](#supermicro-sse-c4632sb) | **spine**, **leaf** | Broadcom TD3-X7 3.2T | 32xQSFP28-100G, 1xSFP28-10G |

!!! note
    - Switches that support **leaf** role could be used for the collapsed-core topology as well
    - Switches with **leaf (l3-only)** role only support L3 VPC modes
    - Switches with **leaf (limited)** role does not support some leaf features and are not supported in the
      collapsed-core topology


## Celestica DS2000

Profile Name (to use in switch object `.spec.profile`): **celestica-ds2000**

Other names: Celestica Questone 2a

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X5 2.0T**

Ports Summary: **48xSFP28-25G, 8xQSFP28-100G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: false
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Direct |  | 25G | 10G, 25G |
| E1/2 | 2 | Direct |  | 25G | 10G, 25G |
| E1/3 | 3 | Direct |  | 25G | 10G, 25G |
| E1/4 | 4 | Direct |  | 25G | 10G, 25G |
| E1/5 | 5 | Direct |  | 25G | 10G, 25G |
| E1/6 | 6 | Direct |  | 25G | 10G, 25G |
| E1/7 | 7 | Direct |  | 25G | 10G, 25G |
| E1/8 | 8 | Direct |  | 25G | 10G, 25G |
| E1/9 | 9 | Direct |  | 25G | 10G, 25G |
| E1/10 | 10 | Direct |  | 25G | 10G, 25G |
| E1/11 | 11 | Direct |  | 25G | 10G, 25G |
| E1/12 | 12 | Direct |  | 25G | 10G, 25G |
| E1/13 | 13 | Direct |  | 25G | 10G, 25G |
| E1/14 | 14 | Direct |  | 25G | 10G, 25G |
| E1/15 | 15 | Direct |  | 25G | 10G, 25G |
| E1/16 | 16 | Direct |  | 25G | 10G, 25G |
| E1/17 | 17 | Direct |  | 25G | 10G, 25G |
| E1/18 | 18 | Direct |  | 25G | 10G, 25G |
| E1/19 | 19 | Direct |  | 25G | 10G, 25G |
| E1/20 | 20 | Direct |  | 25G | 10G, 25G |
| E1/21 | 21 | Direct |  | 25G | 10G, 25G |
| E1/22 | 22 | Direct |  | 25G | 10G, 25G |
| E1/23 | 23 | Direct |  | 25G | 10G, 25G |
| E1/24 | 24 | Direct |  | 25G | 10G, 25G |
| E1/25 | 25 | Direct |  | 25G | 10G, 25G |
| E1/26 | 26 | Direct |  | 25G | 10G, 25G |
| E1/27 | 27 | Direct |  | 25G | 10G, 25G |
| E1/28 | 28 | Direct |  | 25G | 10G, 25G |
| E1/29 | 29 | Direct |  | 25G | 10G, 25G |
| E1/30 | 30 | Direct |  | 25G | 10G, 25G |
| E1/31 | 31 | Direct |  | 25G | 10G, 25G |
| E1/32 | 32 | Direct |  | 25G | 10G, 25G |
| E1/33 | 33 | Direct |  | 25G | 10G, 25G |
| E1/34 | 34 | Direct |  | 25G | 10G, 25G |
| E1/35 | 35 | Direct |  | 25G | 10G, 25G |
| E1/36 | 36 | Direct |  | 25G | 10G, 25G |
| E1/37 | 37 | Direct |  | 25G | 10G, 25G |
| E1/38 | 38 | Direct |  | 25G | 10G, 25G |
| E1/39 | 39 | Direct |  | 25G | 10G, 25G |
| E1/40 | 40 | Direct |  | 25G | 10G, 25G |
| E1/41 | 41 | Direct |  | 25G | 10G, 25G |
| E1/42 | 42 | Direct |  | 25G | 10G, 25G |
| E1/43 | 43 | Direct |  | 25G | 10G, 25G |
| E1/44 | 44 | Direct |  | 25G | 10G, 25G |
| E1/45 | 45 | Direct |  | 25G | 10G, 25G |
| E1/46 | 46 | Direct |  | 25G | 10G, 25G |
| E1/47 | 47 | Direct |  | 25G | 10G, 25G |
| E1/48 | 48 | Direct |  | 25G | 10G, 25G |
| E1/49 | 49 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/50 | 50 | Direct |  | 100G | 40G, 100G |
| E1/51 | 51 | Direct |  | 100G | 40G, 100G |
| E1/52 | 52 | Direct |  | 100G | 40G, 100G |
| E1/53 | 53 | Direct |  | 100G | 40G, 100G |
| E1/54 | 54 | Direct |  | 100G | 40G, 100G |
| E1/55 | 55 | Direct |  | 100G | 40G, 100G |
| E1/56 | 56 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |


## Celestica DS3000

Profile Name (to use in switch object `.spec.profile`): **celestica-ds3000**

Other names: Celestica Seastone2

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X7 3.2T**

Ports Summary: **32xQSFP28-100G, 1xSFP28-10G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/2 | 2 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/3 | 3 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/4 | 4 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/5 | 5 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/6 | 6 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/7 | 7 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/8 | 8 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/9 | 9 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/10 | 10 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/11 | 11 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/12 | 12 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/13 | 13 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/14 | 14 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/15 | 15 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/16 | 16 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/17 | 17 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/18 | 18 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/19 | 19 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/20 | 20 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/21 | 21 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/22 | 22 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/23 | 23 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/24 | 24 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/25 | 25 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/26 | 26 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/27 | 27 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/28 | 28 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/29 | 29 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/30 | 30 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/31 | 31 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/32 | 32 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/33 | 33 | Direct |  | 10G | 1G, 10G |


## Celestica DS4000

Profile Name (to use in switch object `.spec.profile`): **celestica-ds4000**

Other names: Celestica Silverstone2

**Supported roles**: **spine**

Switch Silicon: **Broadcom TH3**

Ports Summary: **32xQSFPDD-400G, 1xSFP28-10G**

**Supported features:**

- Subinterfaces: false
- ACLs: true
- L2VNI: false
- L3VNI: false
- RoCE: true
- MCLAG: false
- ESLAG: false
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/2 | 2 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/3 | 3 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/4 | 4 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/5 | 5 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/6 | 6 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/7 | 7 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/8 | 8 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/9 | 9 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/10 | 10 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/11 | 11 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/12 | 12 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/13 | 13 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/14 | 14 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/15 | 15 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/16 | 16 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/17 | 17 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/18 | 18 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/19 | 19 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/20 | 20 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/21 | 21 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/22 | 22 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/23 | 23 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/24 | 24 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/25 | 25 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/26 | 26 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/27 | 27 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/28 | 28 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/29 | 29 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/30 | 30 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/31 | 31 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/32 | 32 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x25G, 1x400G, 1x40G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/33 | 33 | Direct |  | 10G | 1G, 10G |


## Celestica DS4101

Profile Name (to use in switch object `.spec.profile`): **celestica-ds4101**

Other names: Celestica Greystone

**Supported roles**: **spine**

Switch Silicon: **Broadcom TH4G**

Ports Summary: **32xOSFP-2x400G, 2xSFP28-10G**

**Supported features:**

- Subinterfaces: false
- ACLs: true
- L2VNI: false
- L3VNI: false
- RoCE: true
- MCLAG: false
- ESLAG: false
- ECMP RoCE QPN hashing: true

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/2 | 2 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/3 | 3 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/4 | 4 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/5 | 5 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/6 | 6 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/7 | 7 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/8 | 8 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/9 | 9 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/10 | 10 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/11 | 11 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/12 | 12 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/13 | 13 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/14 | 14 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/15 | 15 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/16 | 16 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/17 | 17 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/18 | 18 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/19 | 19 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/20 | 20 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/21 | 21 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/22 | 22 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/23 | 23 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/24 | 24 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/25 | 25 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/26 | 26 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/27 | 27 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/28 | 28 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/29 | 29 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/30 | 30 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/31 | 31 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/32 | 32 | Breakout |  | 2x400G | 1x100G, 1x200G, 1x400G, 2x100G, 2x200G, 2x400G, 2x40G, 4x100G, 4x200G, 4x50G, 8x100G, 8x10G, 8x25G, 8x50G |
| E1/33 | M1 | Direct |  | 10G | 1G, 10G |
| E1/34 | M2 | Direct |  | 10G | 1G, 10G |


## Celestica DS5000

Profile Name (to use in switch object `.spec.profile`): **celestica-ds5000**

Other names: Celestica Moonstone

**Supported roles**: **spine**, **leaf (l3-only)**

Switch Silicon: **Broadcom TH5**

Ports Summary: **64xOSFP-800G, 2xSFP28-25G**

Notes: Doesn't support non-L3 VPC modes due to the lack of L2VNI support.

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: false
- L3VNI: true
- RoCE: true
- MCLAG: false
- ESLAG: false
- ECMP RoCE QPN hashing: true

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/2 | 2 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/3 | 3 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/4 | 4 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/5 | 5 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/6 | 6 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/7 | 7 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/8 | 8 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/9 | 9 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/10 | 10 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/11 | 11 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/12 | 12 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/13 | 13 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/14 | 14 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/15 | 15 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/16 | 16 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/17 | 17 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/18 | 18 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/19 | 19 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/20 | 20 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/21 | 21 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/22 | 22 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/23 | 23 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/24 | 24 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/25 | 25 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/26 | 26 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/27 | 27 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/28 | 28 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/29 | 29 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/30 | 30 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/31 | 31 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/32 | 32 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/33 | 33 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/34 | 34 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/35 | 35 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/36 | 36 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/37 | 37 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/38 | 38 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/39 | 39 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/40 | 40 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/41 | 41 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/42 | 42 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/43 | 43 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/44 | 44 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/45 | 45 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/46 | 46 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/47 | 47 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/48 | 48 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/49 | 49 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/50 | 50 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/51 | 51 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/52 | 52 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/53 | 53 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/54 | 54 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/55 | 55 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/56 | 56 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/57 | 57 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/58 | 58 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/59 | 59 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/60 | 60 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/61 | 61 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/62 | 62 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/63 | 63 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/64 | 64 | Breakout |  | 1x800G | 1x100G, 1x200G, 1x400G, 1x50G, 1x800G, 2x100G, 2x200G, 2x400G, 2x50G, 4x100G, 4x200G, 8x100G |
| E1/65 | 65 | Direct |  | 25G | 1G, 10G, 25G |
| E1/66 | 66 | Direct |  | 25G | 1G, 10G, 25G |


## Dell S5232F-ON

Profile Name (to use in switch object `.spec.profile`): **dell-s5232f-on**

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X7 3.2T**

Ports Summary: **32xQSFP28-100G, 2xSFP28-10G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/2 | 2 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/3 | 3 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/4 | 4 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/5 | 5 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/6 | 6 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/7 | 7 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/8 | 8 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/9 | 9 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/10 | 10 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/11 | 11 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/12 | 12 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/13 | 13 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/14 | 14 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/15 | 15 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/16 | 16 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/17 | 17 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/18 | 18 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/19 | 19 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/20 | 20 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/21 | 21 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/22 | 22 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/23 | 23 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/24 | 24 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/25 | 25 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/26 | 26 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/27 | 27 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/28 | 28 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/29 | 29 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/30 | 30 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/31 | 31 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/32 | 32 | Direct |  | 100G | 40G, 100G |
| E1/33 | 33 | Direct |  | 10G | 1G, 10G |
| E1/34 | 34 | Direct |  | 10G | 1G, 10G |


## Dell S5248F-ON

Profile Name (to use in switch object `.spec.profile`): **dell-s5248f-on**

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X7 3.2T**

Ports Summary: **48xSFP28-25G, 8xQSFP28-100G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Port Group | 1 | 25G | 10G, 25G |
| E1/2 | 2 | Port Group | 1 | 25G | 10G, 25G |
| E1/3 | 3 | Port Group | 1 | 25G | 10G, 25G |
| E1/4 | 4 | Port Group | 1 | 25G | 10G, 25G |
| E1/5 | 5 | Port Group | 2 | 25G | 10G, 25G |
| E1/6 | 6 | Port Group | 2 | 25G | 10G, 25G |
| E1/7 | 7 | Port Group | 2 | 25G | 10G, 25G |
| E1/8 | 8 | Port Group | 2 | 25G | 10G, 25G |
| E1/9 | 9 | Port Group | 3 | 25G | 10G, 25G |
| E1/10 | 10 | Port Group | 3 | 25G | 10G, 25G |
| E1/11 | 11 | Port Group | 3 | 25G | 10G, 25G |
| E1/12 | 12 | Port Group | 3 | 25G | 10G, 25G |
| E1/13 | 13 | Port Group | 4 | 25G | 10G, 25G |
| E1/14 | 14 | Port Group | 4 | 25G | 10G, 25G |
| E1/15 | 15 | Port Group | 4 | 25G | 10G, 25G |
| E1/16 | 16 | Port Group | 4 | 25G | 10G, 25G |
| E1/17 | 17 | Port Group | 5 | 25G | 10G, 25G |
| E1/18 | 18 | Port Group | 5 | 25G | 10G, 25G |
| E1/19 | 19 | Port Group | 5 | 25G | 10G, 25G |
| E1/20 | 20 | Port Group | 5 | 25G | 10G, 25G |
| E1/21 | 21 | Port Group | 6 | 25G | 10G, 25G |
| E1/22 | 22 | Port Group | 6 | 25G | 10G, 25G |
| E1/23 | 23 | Port Group | 6 | 25G | 10G, 25G |
| E1/24 | 24 | Port Group | 6 | 25G | 10G, 25G |
| E1/25 | 25 | Port Group | 7 | 25G | 10G, 25G |
| E1/26 | 26 | Port Group | 7 | 25G | 10G, 25G |
| E1/27 | 27 | Port Group | 7 | 25G | 10G, 25G |
| E1/28 | 28 | Port Group | 7 | 25G | 10G, 25G |
| E1/29 | 29 | Port Group | 8 | 25G | 10G, 25G |
| E1/30 | 30 | Port Group | 8 | 25G | 10G, 25G |
| E1/31 | 31 | Port Group | 8 | 25G | 10G, 25G |
| E1/32 | 32 | Port Group | 8 | 25G | 10G, 25G |
| E1/33 | 33 | Port Group | 9 | 25G | 10G, 25G |
| E1/34 | 34 | Port Group | 9 | 25G | 10G, 25G |
| E1/35 | 35 | Port Group | 9 | 25G | 10G, 25G |
| E1/36 | 36 | Port Group | 9 | 25G | 10G, 25G |
| E1/37 | 37 | Port Group | 10 | 25G | 10G, 25G |
| E1/38 | 38 | Port Group | 10 | 25G | 10G, 25G |
| E1/39 | 39 | Port Group | 10 | 25G | 10G, 25G |
| E1/40 | 40 | Port Group | 10 | 25G | 10G, 25G |
| E1/41 | 41 | Port Group | 11 | 25G | 10G, 25G |
| E1/42 | 42 | Port Group | 11 | 25G | 10G, 25G |
| E1/43 | 43 | Port Group | 11 | 25G | 10G, 25G |
| E1/44 | 44 | Port Group | 11 | 25G | 10G, 25G |
| E1/45 | 45 | Port Group | 12 | 25G | 10G, 25G |
| E1/46 | 46 | Port Group | 12 | 25G | 10G, 25G |
| E1/47 | 47 | Port Group | 12 | 25G | 10G, 25G |
| E1/48 | 48 | Port Group | 12 | 25G | 10G, 25G |
| E1/49 | 49 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/50 | 50 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/51 | 51 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/52 | 52 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/53 | 53 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/54 | 54 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/55 | 55 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/56 | 56 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |


## Dell Z9332F-ON

Profile Name (to use in switch object `.spec.profile`): **dell-z9332f-on**

**Supported roles**: **spine**

Switch Silicon: **Broadcom TH3**

Ports Summary: **32xQSFPDD-400G, 2xSFP28-10G**

**Supported features:**

- Subinterfaces: false
- ACLs: true
- L2VNI: false
- L3VNI: false
- RoCE: true
- MCLAG: false
- ESLAG: false
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/2 | 2 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/3 | 3 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/4 | 4 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/5 | 5 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/6 | 6 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/7 | 7 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/8 | 8 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/9 | 9 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/10 | 10 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/11 | 11 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/12 | 12 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/13 | 13 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/14 | 14 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/15 | 15 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/16 | 16 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/17 | 17 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/18 | 18 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/19 | 19 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/20 | 20 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/21 | 21 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/22 | 22 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/23 | 23 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/24 | 24 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/25 | 25 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/26 | 26 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/27 | 27 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/28 | 28 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/29 | 29 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/30 | 30 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/31 | 31 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/32 | 32 | Breakout |  | 1x400G | 1x100G, 1x10G, 1x200G, 1x25G, 1x400G, 1x40G, 1x50G, 2x100G, 2x200G, 2x40G, 4x100G, 4x10G, 4x25G, 8x10G, 8x25G, 8x50G |
| E1/33 | M1 | Direct |  | 10G | 1G, 10G |
| E1/34 | M2 | Direct |  | 10G | 1G, 10G |


## Edgecore DCS203

Profile Name (to use in switch object `.spec.profile`): **edgecore-dcs203**

Other names: Edgecore AS7326-56X

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X7 2.0T**

Ports Summary: **48xSFP28-25G, 8xQSFP28-100G, 2xSFP28-10G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Port Group | 1 | 25G | 10G, 25G |
| E1/2 | 2 | Port Group | 1 | 25G | 10G, 25G |
| E1/3 | 3 | Port Group | 1 | 25G | 10G, 25G |
| E1/4 | 4 | Port Group | 1 | 25G | 10G, 25G |
| E1/5 | 5 | Port Group | 1 | 25G | 10G, 25G |
| E1/6 | 6 | Port Group | 1 | 25G | 10G, 25G |
| E1/7 | 7 | Port Group | 1 | 25G | 10G, 25G |
| E1/8 | 8 | Port Group | 1 | 25G | 10G, 25G |
| E1/9 | 9 | Port Group | 1 | 25G | 10G, 25G |
| E1/10 | 10 | Port Group | 1 | 25G | 10G, 25G |
| E1/11 | 11 | Port Group | 1 | 25G | 10G, 25G |
| E1/12 | 12 | Port Group | 1 | 25G | 10G, 25G |
| E1/13 | 13 | Port Group | 2 | 25G | 10G, 25G |
| E1/14 | 14 | Port Group | 2 | 25G | 10G, 25G |
| E1/15 | 15 | Port Group | 2 | 25G | 10G, 25G |
| E1/16 | 16 | Port Group | 2 | 25G | 10G, 25G |
| E1/17 | 17 | Port Group | 2 | 25G | 10G, 25G |
| E1/18 | 18 | Port Group | 2 | 25G | 10G, 25G |
| E1/19 | 19 | Port Group | 2 | 25G | 10G, 25G |
| E1/20 | 20 | Port Group | 2 | 25G | 10G, 25G |
| E1/21 | 21 | Port Group | 2 | 25G | 10G, 25G |
| E1/22 | 22 | Port Group | 2 | 25G | 10G, 25G |
| E1/23 | 23 | Port Group | 2 | 25G | 10G, 25G |
| E1/24 | 24 | Port Group | 2 | 25G | 10G, 25G |
| E1/25 | 25 | Port Group | 3 | 25G | 10G, 25G |
| E1/26 | 26 | Port Group | 3 | 25G | 10G, 25G |
| E1/27 | 27 | Port Group | 3 | 25G | 10G, 25G |
| E1/28 | 28 | Port Group | 3 | 25G | 10G, 25G |
| E1/29 | 29 | Port Group | 3 | 25G | 10G, 25G |
| E1/30 | 30 | Port Group | 3 | 25G | 10G, 25G |
| E1/31 | 31 | Port Group | 3 | 25G | 10G, 25G |
| E1/32 | 32 | Port Group | 3 | 25G | 10G, 25G |
| E1/33 | 33 | Port Group | 3 | 25G | 10G, 25G |
| E1/34 | 34 | Port Group | 3 | 25G | 10G, 25G |
| E1/35 | 35 | Port Group | 3 | 25G | 10G, 25G |
| E1/36 | 36 | Port Group | 3 | 25G | 10G, 25G |
| E1/37 | 37 | Port Group | 4 | 25G | 10G, 25G |
| E1/38 | 38 | Port Group | 4 | 25G | 10G, 25G |
| E1/39 | 39 | Port Group | 4 | 25G | 10G, 25G |
| E1/40 | 40 | Port Group | 4 | 25G | 10G, 25G |
| E1/41 | 41 | Port Group | 4 | 25G | 10G, 25G |
| E1/42 | 42 | Port Group | 4 | 25G | 10G, 25G |
| E1/43 | 43 | Port Group | 4 | 25G | 10G, 25G |
| E1/44 | 44 | Port Group | 4 | 25G | 10G, 25G |
| E1/45 | 45 | Port Group | 4 | 25G | 10G, 25G |
| E1/46 | 46 | Port Group | 4 | 25G | 10G, 25G |
| E1/47 | 47 | Port Group | 4 | 25G | 10G, 25G |
| E1/48 | 48 | Port Group | 4 | 25G | 10G, 25G |
| E1/49 | 49 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/50 | 50 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/51 | 51 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/52 | 52 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/53 | 53 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/54 | 54 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/55 | 55 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/56 | 56 | Direct |  | 100G | 40G, 100G |
| E1/57 | 57 | Direct |  | 10G | 1G, 10G |
| E1/58 | 58 | Direct |  | 10G | 1G, 10G |


## Edgecore DCS204

Profile Name (to use in switch object `.spec.profile`): **edgecore-dcs204**

Other names: Edgecore AS7726-32X

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X7 3.2T**

Ports Summary: **32xQSFP28-100G, 2xSFP28-10G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/2 | 2 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/3 | 3 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/4 | 4 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/5 | 5 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/6 | 6 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/7 | 7 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/8 | 8 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/9 | 9 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/10 | 10 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/11 | 11 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/12 | 12 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/13 | 13 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/14 | 14 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/15 | 15 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/16 | 16 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/17 | 17 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/18 | 18 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/19 | 19 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/20 | 20 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/21 | 21 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/22 | 22 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/23 | 23 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/24 | 24 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/25 | 25 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/26 | 26 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/27 | 27 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/28 | 28 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/29 | 29 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/30 | 30 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/31 | 31 | Breakout |  | 1x100G | 1x100G, 1x10G, 1x25G, 1x40G, 1x50G, 2x50G, 4x10G, 4x25G |
| E1/32 | 32 | Direct |  | 100G | 40G, 100G |
| E1/33 | 33 | Direct |  | 10G | 1G, 10G |
| E1/34 | 34 | Direct |  | 10G | 1G, 10G |


## Edgecore DCS501

Profile Name (to use in switch object `.spec.profile`): **edgecore-dcs501**

Other names: Edgecore AS7712-32X

**Supported roles**: **spine**

Switch Silicon: **Broadcom TH**

Ports Summary: **32xQSFP28-100G**

**Supported features:**

- Subinterfaces: false
- ACLs: true
- L2VNI: false
- L3VNI: false
- RoCE: false
- MCLAG: false
- ESLAG: false
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/2 | 2 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/3 | 3 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/4 | 4 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/5 | 5 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/6 | 6 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/7 | 7 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/8 | 8 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/9 | 9 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/10 | 10 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/11 | 11 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/12 | 12 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/13 | 13 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/14 | 14 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/15 | 15 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/16 | 16 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/17 | 17 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/18 | 18 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/19 | 19 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/20 | 20 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/21 | 21 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/22 | 22 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/23 | 23 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/24 | 24 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/25 | 25 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/26 | 26 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/27 | 27 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/28 | 28 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/29 | 29 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/30 | 30 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/31 | 31 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/32 | 32 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |


## Edgecore EPS203

Profile Name (to use in switch object `.spec.profile`): **edgecore-eps203**

Other names: Edgecore AS4630-54NPE

**Supported roles**: **leaf (limited)**

Switch Silicon: **Broadcom TD3-X3**

Ports Summary: **36xRJ45-2.5G, 12xRJ45-10G, 4xSFP28-25G, 2xQSFP28-100G**

Notes: Doesn't support StaticExternals and ExternalAttachments with VLANs due to the lack of subinterfaces support.

**Supported features:**

- Subinterfaces: false
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: false
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/2 | 2 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/3 | 3 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/4 | 4 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/5 | 5 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/6 | 6 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/7 | 7 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/8 | 8 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/9 | 9 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/10 | 10 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/11 | 11 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/12 | 12 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/13 | 13 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/14 | 14 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/15 | 15 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/16 | 16 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/17 | 17 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/18 | 18 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/19 | 19 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/20 | 20 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/21 | 21 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/22 | 22 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/23 | 23 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/24 | 24 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/25 | 25 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/26 | 26 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/27 | 27 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/28 | 28 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/29 | 29 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/30 | 30 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/31 | 31 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/32 | 32 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/33 | 33 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/34 | 34 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/35 | 35 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/36 | 36 | Direct |  | 2.5G | 1G, 2.5G, AutoNeg supported (default: true) |
| E1/37 | 37 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/38 | 38 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/39 | 39 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/40 | 40 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/41 | 41 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/42 | 42 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/43 | 43 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/44 | 44 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/45 | 45 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/46 | 46 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/47 | 47 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/48 | 48 | Direct |  | 10G | 1G, 10G, AutoNeg supported (default: true) |
| E1/49 | 49 | Direct |  | 25G | 1G, 10G, 25G |
| E1/50 | 50 | Direct |  | 25G | 1G, 10G, 25G |
| E1/51 | 51 | Direct |  | 25G | 1G, 10G, 25G |
| E1/52 | 52 | Direct |  | 25G | 1G, 10G, 25G |
| E1/53 | 53 | Direct |  | 100G | 40G, 100G |
| E1/54 | 54 | Direct |  | 100G | 40G, 100G |


## Supermicro SSE-C4632SB

Profile Name (to use in switch object `.spec.profile`): **supermicro-sse-c4632sb**

**Supported roles**: **spine**, **leaf**

Switch Silicon: **Broadcom TD3-X7 3.2T**

Ports Summary: **32xQSFP28-100G, 1xSFP28-10G**

**Supported features:**

- Subinterfaces: true
- ACLs: true
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/2 | 2 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/3 | 3 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/4 | 4 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/5 | 5 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/6 | 6 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/7 | 7 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/8 | 8 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/9 | 9 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/10 | 10 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/11 | 11 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/12 | 12 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/13 | 13 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/14 | 14 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/15 | 15 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/16 | 16 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/17 | 17 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/18 | 18 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/19 | 19 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/20 | 20 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/21 | 21 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/22 | 22 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/23 | 23 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/24 | 24 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/25 | 25 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/26 | 26 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/27 | 27 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/28 | 28 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/29 | 29 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/30 | 30 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/31 | 31 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/32 | 32 | Breakout |  | 1x100G | 1x100G, 1x40G, 4x10G, 4x25G |
| E1/33 | 33 | Direct |  | 10G | 1G, 10G |


## Virtual Switch

Profile Name (to use in switch object `.spec.profile`): **vs**

This is a virtual switch profile. It's for testing/demo purpose only with limited features and performance.

**Supported roles**: **spine**, **leaf**

Switch Silicon: **vs**

Ports Summary: **48xSFP28-25G**

**Supported features:**

- Subinterfaces: true
- ACLs: false
- L2VNI: true
- L3VNI: true
- RoCE: true
- MCLAG: true
- ESLAG: true
- ECMP RoCE QPN hashing: false

**Available Ports:**

Label column is a port label on a physical switch.

| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
| M1 |  | Management |  |  |  |
| E1/1 | 1 | Port Group | 1 | 25G | 10G, 25G |
| E1/2 | 2 | Port Group | 1 | 25G | 10G, 25G |
| E1/3 | 3 | Port Group | 1 | 25G | 10G, 25G |
| E1/4 | 4 | Port Group | 1 | 25G | 10G, 25G |
| E1/5 | 5 | Port Group | 2 | 25G | 10G, 25G |
| E1/6 | 6 | Port Group | 2 | 25G | 10G, 25G |
| E1/7 | 7 | Port Group | 2 | 25G | 10G, 25G |
| E1/8 | 8 | Port Group | 2 | 25G | 10G, 25G |
| E1/9 | 9 | Port Group | 3 | 25G | 10G, 25G |
| E1/10 | 10 | Port Group | 3 | 25G | 10G, 25G |
| E1/11 | 11 | Port Group | 3 | 25G | 10G, 25G |
| E1/12 | 12 | Port Group | 3 | 25G | 10G, 25G |
| E1/13 | 13 | Port Group | 4 | 25G | 10G, 25G |
| E1/14 | 14 | Port Group | 4 | 25G | 10G, 25G |
| E1/15 | 15 | Port Group | 4 | 25G | 10G, 25G |
| E1/16 | 16 | Port Group | 4 | 25G | 10G, 25G |
| E1/17 | 17 | Port Group | 5 | 25G | 10G, 25G |
| E1/18 | 18 | Port Group | 5 | 25G | 10G, 25G |
| E1/19 | 19 | Port Group | 5 | 25G | 10G, 25G |
| E1/20 | 20 | Port Group | 5 | 25G | 10G, 25G |
| E1/21 | 21 | Port Group | 6 | 25G | 10G, 25G |
| E1/22 | 22 | Port Group | 6 | 25G | 10G, 25G |
| E1/23 | 23 | Port Group | 6 | 25G | 10G, 25G |
| E1/24 | 24 | Port Group | 6 | 25G | 10G, 25G |
| E1/25 | 25 | Port Group | 7 | 25G | 10G, 25G |
| E1/26 | 26 | Port Group | 7 | 25G | 10G, 25G |
| E1/27 | 27 | Port Group | 7 | 25G | 10G, 25G |
| E1/28 | 28 | Port Group | 7 | 25G | 10G, 25G |
| E1/29 | 29 | Port Group | 8 | 25G | 10G, 25G |
| E1/30 | 30 | Port Group | 8 | 25G | 10G, 25G |
| E1/31 | 31 | Port Group | 8 | 25G | 10G, 25G |
| E1/32 | 32 | Port Group | 8 | 25G | 10G, 25G |
| E1/33 | 33 | Port Group | 9 | 25G | 10G, 25G |
| E1/34 | 34 | Port Group | 9 | 25G | 10G, 25G |
| E1/35 | 35 | Port Group | 9 | 25G | 10G, 25G |
| E1/36 | 36 | Port Group | 9 | 25G | 10G, 25G |
| E1/37 | 37 | Port Group | 10 | 25G | 10G, 25G |
| E1/38 | 38 | Port Group | 10 | 25G | 10G, 25G |
| E1/39 | 39 | Port Group | 10 | 25G | 10G, 25G |
| E1/40 | 40 | Port Group | 10 | 25G | 10G, 25G |
| E1/41 | 41 | Port Group | 11 | 25G | 10G, 25G |
| E1/42 | 42 | Port Group | 11 | 25G | 10G, 25G |
| E1/43 | 43 | Port Group | 11 | 25G | 10G, 25G |
| E1/44 | 44 | Port Group | 11 | 25G | 10G, 25G |
| E1/45 | 45 | Port Group | 12 | 25G | 10G, 25G |
| E1/46 | 46 | Port Group | 12 | 25G | 10G, 25G |
| E1/47 | 47 | Port Group | 12 | 25G | 10G, 25G |
| E1/48 | 48 | Port Group | 12 | 25G | 10G, 25G |



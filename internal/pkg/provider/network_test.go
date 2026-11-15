// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
	"libvirt.org/go/libvirtxml"

	"github.com/siderolabs/omni-infra-provider-libvirt/internal/pkg/provider/cidata"
)

const testUUID = "3f0f5d9e-6c3a-4e7b-9a43-1d2f6b1c0a11"

func parseData(t *testing.T, s string) Data {
	t.Helper()

	var d Data
	if err := yaml.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	return d
}

func TestBuildDomainInterfacesBackCompat(t *testing.T) {
	d := parseData(t, `
network_interfaces:
  - driver: virtio
    network_name: default
`)

	ifaces, err := buildDomainInterfaces(d.NetworkInterfaces, testUUID)
	if err != nil {
		t.Fatal(err)
	}

	if len(ifaces) != 1 || ifaces[0].Source.Network == nil || ifaces[0].Source.Network.Network != "default" {
		t.Fatalf("unexpected iface: %+v", ifaces)
	}

	if ifaces[0].Model.Type != "virtio" {
		t.Fatalf("model = %q", ifaces[0].Model.Type)
	}

	if !strings.HasPrefix(ifaces[0].MAC.Address, "52:54:00:") {
		t.Fatalf("mac = %q", ifaces[0].MAC.Address)
	}
}

func TestBuildDomainInterfacesBridgeAndDirect(t *testing.T) {
	d := parseData(t, `
network_interfaces:
  - type: bridge
    bridge: br0
  - type: bridge
    bridge: br1
    driver: e1000
    mac: "52:54:00:AA:BB:CC"
  - type: direct
    device: enp39s0
`)

	ifaces, err := buildDomainInterfaces(d.NetworkInterfaces, testUUID)
	if err != nil {
		t.Fatal(err)
	}

	if len(ifaces) != 3 {
		t.Fatalf("len = %d", len(ifaces))
	}

	if ifaces[0].Source.Bridge == nil || ifaces[0].Source.Bridge.Bridge != "br0" || ifaces[0].Model.Type != "virtio" {
		t.Fatalf("iface0: %+v", ifaces[0])
	}

	if ifaces[1].Source.Bridge.Bridge != "br1" || ifaces[1].Model.Type != "e1000" || ifaces[1].MAC.Address != "52:54:00:aa:bb:cc" {
		t.Fatalf("iface1: %+v %+v", ifaces[1].Source.Bridge, ifaces[1].MAC)
	}

	if ifaces[2].Source.Direct == nil || ifaces[2].Source.Direct.Dev != "enp39s0" || ifaces[2].Source.Direct.Mode != "bridge" {
		t.Fatalf("iface2: %+v", ifaces[2].Source.Direct)
	}

	if ifaces[0].MAC.Address == ifaces[2].MAC.Address {
		t.Fatal("derived MACs must differ per NIC")
	}

	// rendered XML contains <source bridge="br0">
	dom := libvirtxml.Domain{Type: "kvm", Name: "t", Devices: &libvirtxml.DomainDeviceList{Interfaces: ifaces}}

	x, err := dom.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{`<interface type="bridge">`, `<source bridge="br0">`, `<interface type="direct">`, `<source dev="enp39s0" mode="bridge">`} {
		if !strings.Contains(x, want) {
			t.Errorf("domain XML missing %s:\n%s", want, x)
		}
	}
}

func TestDeriveMACStable(t *testing.T) {
	if deriveMAC(testUUID, 0) != deriveMAC(testUUID, 0) {
		t.Fatal("not stable")
	}

	if deriveMAC(testUUID, 0) == deriveMAC(testUUID, 1) {
		t.Fatal("index not mixed in")
	}

	if deriveMAC(testUUID, 0) == deriveMAC("other", 0) {
		t.Fatal("uuid not mixed in")
	}
}

func TestValidateInterfaces(t *testing.T) {
	for name, tc := range map[string]string{
		"missing network": `[{type: network}]`,
		"missing bridge":  `[{type: bridge}]`,
		"missing device":  `[{type: direct}]`,
		"bad type":        `[{type: vxlan, bridge: br0}]`,
		"bad mac":         `[{type: bridge, bridge: br0, mac: nope}]`,
		"bad addr":        `[{type: bridge, bridge: br0, addresses: [192.168.2.50]}]`,
		"bad gw":          `[{type: bridge, bridge: br0, gateway: x}]`,
		"bad mode":        `[{type: direct, device: eth0, mode: weird}]`,
	} {
		t.Run(name, func(t *testing.T) {
			var ifaces []networkInterface
			if err := yaml.Unmarshal([]byte(tc), &ifaces); err != nil {
				t.Fatal(err)
			}

			if err := validateInterfaces(ifaces); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestBuildNetworkDataDefault(t *testing.T) {
	d := parseData(t, `
network_interfaces:
  - type: bridge
    bridge: br0
`)

	out, err := buildNetworkData(d.NetworkInterfaces, testUUID)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != string(cidata.NetworkData()) {
		t.Fatalf("expected default network data, got:\n%s", out)
	}
}

func TestBuildNetworkDataCustom(t *testing.T) {
	d := parseData(t, `
network_interfaces:
  - type: bridge
    bridge: br0
  - type: bridge
    bridge: br1
    addresses: ["192.168.2.50/24"]
`)

	out, err := buildNetworkData(d.NetworkInterfaces, testUUID)
	if err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		Ethernets map[string]struct {
			Match struct {
				MACAddress string `yaml:"macaddress"`
			} `yaml:"match"`
			Addresses []string `yaml:"addresses"`
			DHCP4     bool     `yaml:"dhcp4"`
			Gateway4  string   `yaml:"gateway4"`
		} `yaml:"ethernets"`
		Version int `yaml:"version"`
	}

	if err = yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Version != 2 || len(cfg.Ethernets) != 2 {
		t.Fatalf("unexpected config:\n%s", out)
	}

	eth0, eth1 := cfg.Ethernets["eth0"], cfg.Ethernets["eth1"]

	if !eth0.DHCP4 || eth0.Match.MACAddress != deriveMAC(testUUID, 0) {
		t.Fatalf("eth0: %+v", eth0)
	}

	if eth1.DHCP4 || len(eth1.Addresses) != 1 || eth1.Addresses[0] != "192.168.2.50/24" || eth1.Gateway4 != "" {
		t.Fatalf("eth1: %+v", eth1)
	}

	// domain MACs must match the network-config MACs
	ifaces, err := buildDomainInterfaces(d.NetworkInterfaces, testUUID)
	if err != nil {
		t.Fatal(err)
	}

	if ifaces[1].MAC.Address != eth1.Match.MACAddress {
		t.Fatalf("mac mismatch: %s vs %s", ifaces[1].MAC.Address, eth1.Match.MACAddress)
	}
}

func TestPhysicalAddressAlias(t *testing.T) {
	d := parseData(t, `
network_interfaces:
  - type: bridge
    bridge: br0
    physical_address: "52:54:00:01:02:03"
`)

	ifaces, err := buildDomainInterfaces(d.NetworkInterfaces, testUUID)
	if err != nil {
		t.Fatal(err)
	}

	if ifaces[0].MAC.Address != "52:54:00:01:02:03" {
		t.Fatalf("mac = %q", ifaces[0].MAC.Address)
	}
}

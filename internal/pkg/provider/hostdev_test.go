// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"strings"
	"testing"

	"libvirt.org/go/libvirtxml"
)

func TestParsePCIAddress(t *testing.T) {
	d, b, s, f, err := parsePCIAddress(" 0000:00:02.0 ")
	if err != nil || d != 0 || b != 0 || s != 2 || f != 0 {
		t.Fatalf("got %d %d %d %d %v", d, b, s, f, err)
	}

	d, b, s, f, err = parsePCIAddress("0001:0a:1f.7")
	if err != nil || d != 1 || b != 0x0a || s != 0x1f || f != 7 {
		t.Fatalf("got %d %d %d %d %v", d, b, s, f, err)
	}

	for _, bad := range []string{"", "00:02.0", "0000:00:02", "0000:zz:02.0", "0000:00:02.x", "0000:100:02.0"} {
		if _, _, _, _, err := parsePCIAddress(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestPCIHostdevsBackCompat(t *testing.T) {
	d := parseData(t, "cores: 2\nmemory: 4096\ndisk_size: 16\n")

	hd, err := buildPCIHostdevs(d.PCIHostdevs)
	if err != nil || hd != nil {
		t.Fatalf("expected no hostdevs, got %v %v", hd, err)
	}
}

func TestPCIHostdevsXML(t *testing.T) {
	d := parseData(t, `cores: 2
memory: 4096
disk_size: 16
pci_hostdevs:
  - address: "0000:00:02.0"
  - address: "0000:01:00.0"
    managed: false
    rombar: false
    romfile: /var/lib/igd.rom
`)

	hd, err := buildPCIHostdevs(d.PCIHostdevs)
	if err != nil {
		t.Fatal(err)
	}

	dom := libvirtxml.Domain{Type: "kvm", Name: "t", Devices: &libvirtxml.DomainDeviceList{Hostdevs: hd}}

	out, err := dom.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`<hostdev mode="subsystem" type="pci" managed="yes">`,
		`<driver name="vfio"></driver>`,
		`<address domain="0x0000" bus="0x00" slot="0x02" function="0x0"></address>`,
		`<hostdev mode="subsystem" type="pci" managed="no">`,
		`<rom bar="off" file="/var/lib/igd.rom"></rom>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}

	if _, err := buildPCIHostdevs([]pciHostdev{{Address: "bogus"}}); err == nil {
		t.Error("expected error for bogus address")
	}
}

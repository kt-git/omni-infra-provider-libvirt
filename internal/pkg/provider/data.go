// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

// Data is the provider custom machine config.
type Data struct {
	StoragePool       string             `yaml:"storage_pool"`
	Graphics          string             `yaml:"graphics,omitempty"`
	NetworkInterfaces []networkInterface `yaml:"network_interfaces,omitempty"`
	AdditionalDisks   []additionalDisk   `yaml:"additional_disks,omitempty"`
	DiskSize          uint64             `yaml:"disk_size"`
	Cores             uint               `yaml:"cores"`
	Memory            uint               `yaml:"memory"`
}

type additionalDisk struct {
	Type string `yaml:"type"`
	Size uint64 `yaml:"size"` // GiB
}

// networkInterface describes a single VM NIC.
//
// Type selects how the NIC is attached on the host:
//   - "network" (default): a libvirt-managed network, selected by NetworkName.
//   - "bridge": an existing host Linux bridge (e.g. br0), selected by Bridge.
//   - "direct": macvtap on a host interface, selected by Device (Mode defaults to "bridge").
//
// MAC is optional; if empty, a stable MAC is derived from the machine UUID and the NIC index.
//
// DHCP4/Addresses/Gateway/Nameservers optionally control the guest network config passed
// via nocloud network-config. If none of the NICs set them, the provider keeps its default
// behavior (DHCP on all interfaces).
type networkInterface struct {
	DHCP4       *bool    `yaml:"dhcp4,omitempty"`
	Type        string   `yaml:"type,omitempty"`
	Driver      string   `yaml:"driver"`
	NetworkName string   `yaml:"network_name,omitempty"`
	Bridge      string   `yaml:"bridge,omitempty"`
	Device      string   `yaml:"device,omitempty"`
	Mode        string   `yaml:"mode,omitempty"`
	MAC         string   `yaml:"mac,omitempty"`
	Gateway     string   `yaml:"gateway,omitempty"`
	Addresses   []string `yaml:"addresses,omitempty"`
	Nameservers []string `yaml:"nameservers,omitempty"`
}

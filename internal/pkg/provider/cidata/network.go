// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cidata

import (
	"fmt"
	"net/netip"

	"go.yaml.in/yaml/v3"
)

// Ethernet is a single interface entry for the nocloud network-config (v2).
type Ethernet struct {
	Name        string
	MAC         string
	Gateway     string
	Addresses   []string
	Nameservers []string
	DHCP4       bool
}

type v2Match struct {
	MACAddress string `yaml:"macaddress"`
}

type v2Nameservers struct {
	Addresses []string `yaml:"addresses"`
}

type v2Ethernet struct {
	Match       v2Match        `yaml:"match"`
	Nameservers *v2Nameservers `yaml:"nameservers,omitempty"`
	Gateway4    string         `yaml:"gateway4,omitempty"`
	Gateway6    string         `yaml:"gateway6,omitempty"`
	Addresses   []string       `yaml:"addresses,omitempty"`
	DHCP4       bool           `yaml:"dhcp4"`
	DHCP6       bool           `yaml:"dhcp6"`
}

type v2Config struct {
	Ethernets map[string]v2Ethernet `yaml:"ethernets"`
	Version   int                   `yaml:"version"`
}

// RenderNetworkData renders a network-config v2 document with interfaces matched by MAC address.
func RenderNetworkData(eths []Ethernet) ([]byte, error) {
	cfg := v2Config{Version: 2, Ethernets: make(map[string]v2Ethernet, len(eths))}

	for _, e := range eths {
		entry := v2Ethernet{
			Match:     v2Match{MACAddress: e.MAC},
			Addresses: e.Addresses,
			DHCP4:     e.DHCP4,
		}

		if e.Gateway != "" {
			gw, err := netip.ParseAddr(e.Gateway)
			if err != nil {
				return nil, fmt.Errorf("invalid gateway %q: %w", e.Gateway, err)
			}

			if gw.Is4() {
				entry.Gateway4 = e.Gateway
			} else {
				entry.Gateway6 = e.Gateway
			}
		}

		if len(e.Nameservers) > 0 {
			entry.Nameservers = &v2Nameservers{Addresses: e.Nameservers}
		}

		cfg.Ethernets[e.Name] = entry
	}

	return yaml.Marshal(cfg)
}

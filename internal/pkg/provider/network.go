// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"libvirt.org/go/libvirtxml"

	"github.com/siderolabs/omni-infra-provider-libvirt/internal/pkg/provider/cidata"
)

const (
	ifaceTypeNetwork = "network"
	ifaceTypeBridge  = "bridge"
	ifaceTypeDirect  = "direct"

	defaultIfaceDriver = "virtio"
	defaultDirectMode  = "bridge"
)

func (n networkInterface) ifaceType() string {
	if n.Type == "" {
		return ifaceTypeNetwork
	}

	return strings.ToLower(n.Type)
}

// macAddr returns the user-provided MAC (mac, or the legacy physical_address).
func (n networkInterface) macAddr() string {
	if n.MAC != "" {
		return n.MAC
	}

	return n.PhysAddr
}

func (n networkInterface) validate(idx int) error {
	switch n.ifaceType() {
	case ifaceTypeNetwork:
		if n.NetworkName == "" {
			return fmt.Errorf("network_interfaces[%d]: network_name is required for type %q", idx, ifaceTypeNetwork)
		}
	case ifaceTypeBridge:
		if n.Bridge == "" {
			return fmt.Errorf("network_interfaces[%d]: bridge is required for type %q", idx, ifaceTypeBridge)
		}
	case ifaceTypeDirect:
		if n.Device == "" {
			return fmt.Errorf("network_interfaces[%d]: device is required for type %q", idx, ifaceTypeDirect)
		}

		switch n.Mode {
		case "", "bridge", "vepa", "private", "passthrough":
		default:
			return fmt.Errorf("network_interfaces[%d]: unsupported direct mode %q", idx, n.Mode)
		}
	default:
		return fmt.Errorf("network_interfaces[%d]: unsupported type %q (want network, bridge or direct)", idx, n.Type)
	}

	if mac := n.macAddr(); mac != "" {
		if _, err := net.ParseMAC(mac); err != nil {
			return fmt.Errorf("network_interfaces[%d]: invalid mac %q: %w", idx, mac, err)
		}
	}

	for _, a := range n.Addresses {
		if _, err := netip.ParsePrefix(a); err != nil {
			return fmt.Errorf("network_interfaces[%d]: invalid address %q (CIDR expected): %w", idx, a, err)
		}
	}

	if n.Gateway != "" {
		if _, err := netip.ParseAddr(n.Gateway); err != nil {
			return fmt.Errorf("network_interfaces[%d]: invalid gateway %q: %w", idx, n.Gateway, err)
		}
	}

	for _, ns := range n.Nameservers {
		if _, err := netip.ParseAddr(ns); err != nil {
			return fmt.Errorf("network_interfaces[%d]: invalid nameserver %q: %w", idx, ns, err)
		}
	}

	return nil
}

// hasGuestConfig reports whether the NIC carries explicit guest IP settings.
func (n networkInterface) hasGuestConfig() bool {
	return n.DHCP4 != nil || len(n.Addresses) > 0 || n.Gateway != "" || len(n.Nameservers) > 0
}

// deriveMAC returns a stable, locally administered MAC (QEMU OUI 52:54:00) for the given machine UUID and NIC index.
func deriveMAC(machineUUID string, idx int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s/%d", machineUUID, idx))

	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", sum[0], sum[1], sum[2])
}

// interfaceMAC returns the configured MAC (normalized) or a derived one.
func interfaceMAC(n networkInterface, machineUUID string, idx int) string {
	if mac := n.macAddr(); mac != "" {
		if hw, err := net.ParseMAC(mac); err == nil {
			return hw.String()
		}
	}

	return deriveMAC(machineUUID, idx)
}

// validateInterfaces validates all NICs.
func validateInterfaces(ifaces []networkInterface) error {
	for i, n := range ifaces {
		if err := n.validate(i); err != nil {
			return err
		}
	}

	return nil
}

// buildDomainInterfaces renders libvirt domain interfaces for the given NICs.
func buildDomainInterfaces(ifaces []networkInterface, machineUUID string) ([]libvirtxml.DomainInterface, error) {
	if err := validateInterfaces(ifaces); err != nil {
		return nil, err
	}

	result := make([]libvirtxml.DomainInterface, 0, len(ifaces))

	for i, n := range ifaces {
		driver := n.Driver
		if driver == "" {
			driver = defaultIfaceDriver
		}

		iface := libvirtxml.DomainInterface{
			MAC:   &libvirtxml.DomainInterfaceMAC{Address: interfaceMAC(n, machineUUID, i)},
			Model: &libvirtxml.DomainInterfaceModel{Type: driver},
		}

		switch n.ifaceType() {
		case ifaceTypeNetwork:
			iface.Source = &libvirtxml.DomainInterfaceSource{
				Network: &libvirtxml.DomainInterfaceSourceNetwork{Network: n.NetworkName},
			}
		case ifaceTypeBridge:
			iface.Source = &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{Bridge: n.Bridge},
			}
		case ifaceTypeDirect:
			mode := n.Mode
			if mode == "" {
				mode = defaultDirectMode
			}

			iface.Source = &libvirtxml.DomainInterfaceSource{
				Direct: &libvirtxml.DomainInterfaceSourceDirect{Dev: n.Device, Mode: mode},
			}
		}

		result = append(result, iface)
	}

	return result, nil
}

// buildNetworkData renders the nocloud network-config for the given NICs.
//
// If no NIC carries explicit guest settings, the default (DHCP on all interfaces) is returned to keep
// backwards compatibility.
func buildNetworkData(ifaces []networkInterface, machineUUID string) ([]byte, error) {
	if err := validateInterfaces(ifaces); err != nil {
		return nil, err
	}

	custom := false

	for _, n := range ifaces {
		if n.hasGuestConfig() {
			custom = true

			break
		}
	}

	if !custom {
		return cidata.NetworkData(), nil
	}

	eths := make([]cidata.Ethernet, 0, len(ifaces))

	for i, n := range ifaces {
		dhcp4 := len(n.Addresses) == 0
		if n.DHCP4 != nil {
			dhcp4 = *n.DHCP4
		}

		eths = append(eths, cidata.Ethernet{
			Name:        fmt.Sprintf("eth%d", i),
			MAC:         interfaceMAC(n, machineUUID, i),
			DHCP4:       dhcp4,
			Addresses:   n.Addresses,
			Gateway:     n.Gateway,
			Nameservers: n.Nameservers,
		})
	}

	return cidata.RenderNetworkData(eths)
}

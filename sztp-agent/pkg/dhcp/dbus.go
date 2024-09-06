/*
SPDX-License-Identifier: Apache-2.0
Copyright (C) 2022-2023 Intel Corporation
Copyright (c) 2022 Dell Inc, or its subsidiaries.
Copyright (C) 2022 Red Hat.
*/

// Package dhcp implements the DHCP client
package dhcp

import (
	"fmt"
	"log"

	"github.com/godbus/dbus"
)

func GetBootstrapURLsViaNetworkManager() ([]string, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		panic(fmt.Errorf("failed to connect to system bus: %v", err))
	}

	// Get NetworkManager object
	nm := conn.Object("org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager")

	// Get PrimaryConnection property from NetworkManager object
	var primaryConnPath dbus.ObjectPath
	err = nm.Call("org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.NetworkManager", "PrimaryConnection").Store(&primaryConnPath)
	if err != nil {
		panic(fmt.Errorf("failed to get PrimaryConnection property: %v", err))
	}

	
	// Get Active Connection object
	connActive := conn.Object("org.freedesktop.NetworkManager", primaryConnPath)
	
	// Get Dhcp4Config property from Active Connection object
	var dhcpPath dbus.ObjectPath
	err = connActive.Call("org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.NetworkManager.Connection.Active", "Dhcp4Config").Store(&dhcpPath)
	if err != nil {
		panic(fmt.Errorf("failed to get Dhcp4Config property: %v", err))
	}
	
	// Get Options property from DHCP4Config object
	dhcp := conn.Object("org.freedesktop.NetworkManager", dhcpPath)
	var options map[string]dbus.Variant
	err = dhcp.Call("org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.NetworkManager.DHCP4Config", "Options").Store(&options)
	if err != nil {
		panic(fmt.Errorf("failed to get Options property: %v", err))
	}
	
	// Print sztp_redirect_urls option
	// sztpRedirectURLs := options["sztp_redirect_urls"].Value().(string)
	if variant, ok := options["sztp_redirect_urls"]; ok {
		sztpRedirectURLs := variant.Value().(string)
		log.Println("lmao")
		fmt.Println(sztpRedirectURLs)
		return []string{sztpRedirectURLs}, nil
	}

	return nil, fmt.Errorf("sztp_redirect_url was not found")
}

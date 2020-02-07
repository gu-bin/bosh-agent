// +build !windows

package net_test

import (
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry/bosh-agent/platform/net"
	fakearp "github.com/cloudfoundry/bosh-agent/platform/net/arp/fakes"
	boship "github.com/cloudfoundry/bosh-agent/platform/net/ip"
	fakeip "github.com/cloudfoundry/bosh-agent/platform/net/ip/fakes"
	"github.com/cloudfoundry/bosh-agent/platform/net/netfakes"
	boshsettings "github.com/cloudfoundry/bosh-agent/settings"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	fakesys "github.com/cloudfoundry/bosh-utils/system/fakes"
)

var _ = Describe("opensuseNetManager", describeOpensuseNetManager)

func describeOpensuseNetManager() {
	var (
		fs                            *fakesys.FakeFileSystem
		cmdRunner                     *fakesys.FakeCmdRunner
		ipResolver                    *fakeip.FakeResolver
		interfaceAddrsProvider        *fakeip.FakeInterfaceAddressesProvider
		addressBroadcaster            *fakearp.FakeAddressBroadcaster
		netManager                    Manager
		interfaceConfigurationCreator InterfaceConfigurationCreator
		fakeMACAddressDetector        *netfakes.FakeMACAddressDetector
	)

	stubInterfaces := func(physicalInterfaces map[string]boshsettings.Network) {
		addresses := map[string]string{}
		for iface, networkSettings := range physicalInterfaces {
			addresses[networkSettings.Mac] = iface
		}

		fakeMACAddressDetector.DetectMacAddressesReturns(addresses, nil)
	}

	BeforeEach(func() {
		fs = fakesys.NewFakeFileSystem()
		cmdRunner = fakesys.NewFakeCmdRunner()
		ipResolver = &fakeip.FakeResolver{}
		logger := boshlog.NewLogger(boshlog.LevelNone)
		fakeMACAddressDetector = &netfakes.FakeMACAddressDetector{}
		interfaceConfigurationCreator = NewInterfaceConfigurationCreator(logger)
		interfaceAddrsProvider = &fakeip.FakeInterfaceAddressesProvider{}
		dnsValidator := NewDNSValidator(fs)
		addressBroadcaster = &fakearp.FakeAddressBroadcaster{}
		netManager = NewOpensuseNetManager(
			fs,
			cmdRunner,
			ipResolver,
			fakeMACAddressDetector,
			interfaceConfigurationCreator,
			interfaceAddrsProvider,
			dnsValidator,
			addressBroadcaster,
			logger,
		)
	})

	Describe("SetupNetworking", func() {
		var (
			dhcpNetwork                           boshsettings.Network
			dhcpNetwork2                          boshsettings.Network
			staticNetwork                         boshsettings.Network
			expectedNetworkConfigurationForStatic string
			expectedNetworkConfigurationForDHCP   string
			expectedDhclientConfiguration         string
		)

		BeforeEach(func() {
			dhcpNetwork = boshsettings.Network{
				Type:    "dynamic",
				Default: []string{"dns"},
				DNS:     []string{"8.8.8.8", "9.9.9.9"},
				Mac:     "fake-dhcp-mac-address",
			}
			dhcpNetwork2 = boshsettings.Network{
				Type:    "dynamic",
				Default: []string{"dns"},
				DNS:     []string{"8.8.8.8", "9.9.9.9"},
				Mac:     "fake-dhcp-mac-address2",
			}
			staticNetwork = boshsettings.Network{
				Type:    "manual",
				IP:      "1.2.3.4",
				Netmask: "255.255.255.0",
				Gateway: "3.4.5.6",
				Mac:     "fake-static-mac-address",
			}
			interfaceAddrsProvider.GetInterfaceAddresses = []boship.InterfaceAddress{
				boship.NewSimpleInterfaceAddress("ethstatic", "1.2.3.4"),
			}
			fs.WriteFileString("/etc/resolv.conf", `
nameserver 8.8.8.8
nameserver 9.9.9.9
`)

			expectedNetworkConfigurationForStatic = `DEVICE=ethstatic
BOOTPROTO=static
STARTMODE='auto'
IPADDR=1.2.3.4
NETMASK=255.255.255.0
BROADCAST=1.2.3.255
GATEWAY=3.4.5.6
DNS1=8.8.8.8
DNS2=9.9.9.9
`

			expectedNetworkConfigurationForDHCP = `DEVICE=ethdhcp
BOOTPROTO=dhcp
STARTMODE='auto'
DHCLIENT_SET_DEFAULT_ROUTE=yes
`

			expectedDhclientConfiguration = `# Generated by bosh-agent
WICKED_DEBUG=""
WICKED_LOG_LEVEL=""
CHECK_DUPLICATE_IP="yes"
SEND_GRATUITOUS_ARP="auto"
DEBUG="no"
WAIT_FOR_INTERFACES="30"
FIREWALL="yes"
NM_ONLINE_TIMEOUT="30"
NETCONFIG_MODULES_ORDER="dns-resolver dns-bind dns-dnsmasq nis ntp-runtime"
NETCONFIG_VERBOSE="no"
NETCONFIG_FORCE_REPLACE="no"
NETCONFIG_DNS_POLICY="auto"
NETCONFIG_DNS_FORWARDER="resolver"
NETCONFIG_DNS_FORWARDER_FALLBACK="yes"
NETCONFIG_DNS_STATIC_SEARCHLIST=""
NETCONFIG_DNS_STATIC_SERVERS="8.8.8.8 9.9.9.9"
NETCONFIG_DNS_RANKING="auto"
NETCONFIG_DNS_RESOLVER_OPTIONS=""
NETCONFIG_DNS_RESOLVER_SORTLIST=""
NETCONFIG_NTP_POLICY="auto"
NETCONFIG_NTP_STATIC_SERVERS=""
NETCONFIG_NIS_POLICY="auto"
NETCONFIG_NIS_SETDOMAINNAME="yes"
NETCONFIG_NIS_STATIC_DOMAIN=""
NETCONFIG_NIS_STATIC_SERVERS=""
WIRELESS_REGULATORY_DOMAIN=''
`
		})

		Context("networks is preconfigured", func() {
			var networks boshsettings.Networks
			BeforeEach(func() {
				dhcpNetwork.Preconfigured = true
				staticNetwork.Preconfigured = true
				networks = boshsettings.Networks{
					"first":  dhcpNetwork,
					"second": staticNetwork,
				}

				Expect(networks.IsPreconfigured()).To(BeTrue())
			})

			Context("when there are configured DNS servers", func() {
				BeforeEach(func() {
					networks = boshsettings.Networks{
						"first": dhcpNetwork,
					}
				})

				It("writes DNS to /etc/resolv.conf", func() {
					err := netManager.SetupNetworking(networks, nil)
					Expect(err).ToNot(HaveOccurred())

					resolvConfBase := fs.GetFileTestStat("/etc/resolv.conf")
					Expect(resolvConfBase).ToNot(BeNil())

					expectedResolvConfBase := `# Generated by bosh-agent
nameserver 8.8.8.8
nameserver 9.9.9.9
`
					Expect(resolvConfBase.StringContents()).To(Equal(expectedResolvConfBase))
				})

				Context("when writing to /etc/resolv.conf", func() {
					It("fails reporting the error", func() {
						fs.WriteFileError = errors.New("fake-write-file-error")

						err := netManager.SetupNetworking(networks, nil)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("Writing to /etc/resolv.conf"))
					})
				})
			})

			It("writes dns servers in /etc/resolv.conf", func() {
				dhcpNetwork.Preconfigured = true
				staticNetwork.Preconfigured = true
				networks := boshsettings.Networks{
					"first":  dhcpNetwork,
					"second": staticNetwork,
				}

				Expect(networks.IsPreconfigured()).To(BeTrue())

				err := netManager.SetupNetworking(networks, nil)
				Expect(err).ToNot(HaveOccurred())

				resolvConf := fs.GetFileTestStat("/etc/resolv.conf")
				Expect(resolvConf).ToNot(BeNil())

				expectedResolvConfHead := `# Generated by bosh-agent
nameserver 8.8.8.8
nameserver 9.9.9.9
`
				Expect(resolvConf.StringContents()).To(Equal(expectedResolvConfHead))
			})
		})

		It("writes a network script for static and dynamic interfaces", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			staticConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethstatic")
			Expect(staticConfig).ToNot(BeNil())
			Expect(staticConfig.StringContents()).To(Equal(expectedNetworkConfigurationForStatic))

			dhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethdhcp")
			Expect(dhcpConfig).ToNot(BeNil())
			Expect(dhcpConfig.StringContents()).To(Equal(expectedNetworkConfigurationForDHCP))
		})

		It("only sets up one default route", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"eth1": dhcpNetwork,
				"eth2": dhcpNetwork2,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"eth1": dhcpNetwork, "eth2": dhcpNetwork2}, nil)
			Expect(err).ToNot(HaveOccurred())

			firstDhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-eth1")
			Expect(firstDhcpConfig).ToNot(BeNil())
			secondDhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-eth2")
			Expect(secondDhcpConfig).ToNot(BeNil())

			if strings.Contains(firstDhcpConfig.StringContents(), "DHCLIENT_SET_DEFAULT_ROUTE=yes") {
				Expect(firstDhcpConfig.StringContents()).To(Equal(`DEVICE=eth1
BOOTPROTO=dhcp
STARTMODE='auto'
DHCLIENT_SET_DEFAULT_ROUTE=yes
`))
				Expect(secondDhcpConfig.StringContents()).To(Equal(`DEVICE=eth2
BOOTPROTO=dhcp
STARTMODE='auto'
DHCLIENT_SET_DEFAULT_ROUTE=no
`))
			} else {
				Expect(firstDhcpConfig.StringContents()).To(Equal(`DEVICE=eth1
BOOTPROTO=dhcp
STARTMODE='auto'
DHCLIENT_SET_DEFAULT_ROUTE=no
`))
				Expect(secondDhcpConfig.StringContents()).To(Equal(`DEVICE=eth2
BOOTPROTO=dhcp
STARTMODE='auto'
DHCLIENT_SET_DEFAULT_ROUTE=yes
`))
			}
		})

		It("returns errors from writing the network configuration", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"dhcp":   dhcpNetwork,
				"static": staticNetwork,
			})
			fs.WriteFileError = errors.New("fs-write-file-error")
			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fs-write-file-error"))
		})

		It("returns errors when it can't create network interface configurations", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethstatic": staticNetwork,
			})

			staticNetwork.Netmask = "not an ip" //will cause InterfaceConfigurationCreator to fail
			err := netManager.SetupNetworking(boshsettings.Networks{"static-network": staticNetwork}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Creating interface configurations"))
		})

		It("writes a dhcp configuration if there are dhcp networks", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			dhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/config")
			Expect(dhcpConfig).ToNot(BeNil())
			Expect(dhcpConfig.StringContents()).To(Equal(expectedDhclientConfiguration))
		})

		It("writes a dhcp configuration without prepended dns servers if there are no dns servers specified", func() {
			dhcpNetworkWithoutDNS := boshsettings.Network{
				Type: "dynamic",
				Mac:  "fake-dhcp-mac-address",
			}

			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp": dhcpNetwork,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetworkWithoutDNS}, nil)
			Expect(err).ToNot(HaveOccurred())

			dhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/config")
			Expect(dhcpConfig).ToNot(BeNil())
			Expect(dhcpConfig.StringContents()).To(Equal(`# Generated by bosh-agent
WICKED_DEBUG=""
WICKED_LOG_LEVEL=""
CHECK_DUPLICATE_IP="yes"
SEND_GRATUITOUS_ARP="auto"
DEBUG="no"
WAIT_FOR_INTERFACES="30"
FIREWALL="yes"
NM_ONLINE_TIMEOUT="30"
NETCONFIG_MODULES_ORDER="dns-resolver dns-bind dns-dnsmasq nis ntp-runtime"
NETCONFIG_VERBOSE="no"
NETCONFIG_FORCE_REPLACE="no"
NETCONFIG_DNS_POLICY="auto"
NETCONFIG_DNS_FORWARDER="resolver"
NETCONFIG_DNS_FORWARDER_FALLBACK="yes"
NETCONFIG_DNS_STATIC_SEARCHLIST=""
NETCONFIG_DNS_RANKING="auto"
NETCONFIG_DNS_RESOLVER_OPTIONS=""
NETCONFIG_DNS_RESOLVER_SORTLIST=""
NETCONFIG_NTP_POLICY="auto"
NETCONFIG_NTP_STATIC_SERVERS=""
NETCONFIG_NIS_POLICY="auto"
NETCONFIG_NIS_SETDOMAINNAME="yes"
NETCONFIG_NIS_STATIC_DOMAIN=""
NETCONFIG_NIS_STATIC_SERVERS=""
WIRELESS_REGULATORY_DOMAIN=''
`))
		})

		It("add DNS servers if the configuration is empty", func() {
			fs.WriteFileString("/etc/sysconfig/network/config", `NETCONFIG_DNS_STATIC_SERVERS=""`)

			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			dhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/config")
			Expect(dhcpConfig).ToNot(BeNil())
			Expect(dhcpConfig.StringContents()).To(ContainSubstring(`NETCONFIG_DNS_STATIC_SERVERS="8.8.8.8 9.9.9.9"`))
		})

		It("preserves existing DNS servers", func() {
			fs.WriteFileString("/etc/sysconfig/network/config", `NETCONFIG_DNS_STATIC_SERVERS="1.2.3.4"`)

			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			dhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/config")
			Expect(dhcpConfig).ToNot(BeNil())
			Expect(dhcpConfig.StringContents()).To(ContainSubstring(`NETCONFIG_DNS_STATIC_SERVERS="1.2.3.4 8.8.8.8 9.9.9.9"`))
		})

		It("returns an error if it can't write a dhcp configuration", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			fs.WriteFileErrors["/etc/sysconfig/network/config"] = errors.New("config-write-error")

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config-write-error"))
		})

		It("doesn't write a dhcp configuration if there are no dhcp networks", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethstatic": staticNetwork,
			})

			err := netManager.SetupNetworking(boshsettings.Networks{"static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			dhcpConfig := fs.GetFileTestStat("/etc/dhcp/dhclient-ethdhcp.conf")
			Expect(dhcpConfig).To(BeNil())
		})

		It("restarts the networks if any ifconfig file changes", func() {
			changingStaticNetwork := boshsettings.Network{
				Type:    "manual",
				IP:      "1.2.3.5",
				Netmask: "255.255.255.0",
				Gateway: "3.4.5.6",
				Mac:     "ethstatict-that-changes",
			}

			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":                dhcpNetwork,
				"ethstatic-that-changes": changingStaticNetwork,
				"ethstatic":              staticNetwork,
			})
			interfaceAddrsProvider.GetInterfaceAddresses = []boship.InterfaceAddress{
				boship.NewSimpleInterfaceAddress("ethstatic", "1.2.3.4"),
				boship.NewSimpleInterfaceAddress("ethstatic-that-changes", "1.2.3.5"),
			}

			fs.WriteFileString("/etc/sysconfig/network/ifcfg-ethstatic", expectedNetworkConfigurationForStatic)
			fs.WriteFileString("/etc/dhcp/dhclient.conf", expectedDhclientConfiguration)

			err := netManager.SetupNetworking(boshsettings.Networks{
				"dhcp-network":            dhcpNetwork,
				"changing-static-network": changingStaticNetwork,
				"static-network":          staticNetwork,
			},
				nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(len(cmdRunner.RunCommands)).To(Equal(2))
			Expect(cmdRunner.RunCommands[1]).To(Equal([]string{"service", "network", "restart"}))
		})

		It("doesn't restart the networks if ifcfg and /etc/dhcp/dhclient.conf don't change", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			fs.WriteFileString("/etc/sysconfig/network/ifcfg-ethstatic", expectedNetworkConfigurationForStatic)
			fs.WriteFileString("/etc/sysconfig/network/ifcfg-ethdhcp", expectedNetworkConfigurationForDHCP)
			fs.WriteFileString("/etc/sysconfig/network/config", expectedDhclientConfiguration)

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			networkConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethstatic")
			Expect(networkConfig).ToNot(BeNil())
			Expect(networkConfig.StringContents()).To(Equal(expectedNetworkConfigurationForStatic))

			dhcpConfig := fs.GetFileTestStat("/etc/sysconfig/network/config")
			Expect(dhcpConfig.StringContents()).To(Equal(expectedDhclientConfiguration))

			Expect(len(cmdRunner.RunCommands)).To(Equal(0))
		})

		It("restarts the networks if /etc/dhcp/dhclient.conf changes", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			fs.WriteFileString("/etc/sysconfig/network/ifcfg-ethstatic", expectedNetworkConfigurationForStatic)

			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, nil)
			Expect(err).ToNot(HaveOccurred())

			networkConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethstatic")
			Expect(networkConfig).ToNot(BeNil())
			Expect(networkConfig.StringContents()).To(Equal(expectedNetworkConfigurationForStatic))

			Expect(len(cmdRunner.RunCommands)).To(Equal(2))
			Expect(cmdRunner.RunCommands[1]).To(Equal([]string{"service", "network", "restart"}))
		})

		Context("when manual networks were not configured with proper IP addresses", func() {
			BeforeEach(func() {
				interfaceAddrsProvider.GetInterfaceAddresses = []boship.InterfaceAddress{
					boship.NewSimpleInterfaceAddress("ethstatic", "1.2.3.5"),
				}
			})

			It("fails", func() {
				stubInterfaces(map[string]boshsettings.Network{
					"ethstatic": staticNetwork,
				})

				errCh := make(chan error)
				err := netManager.SetupNetworking(boshsettings.Networks{"static-network": staticNetwork}, errCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Validating static network configuration"))
			})
		})

		Context("when dns is not properly configured", func() {
			BeforeEach(func() {
				fs.WriteFileString("/etc/resolv.conf", "")
			})

			It("fails", func() {
				staticNetwork = boshsettings.Network{
					Type:    "manual",
					IP:      "1.2.3.4",
					Default: []string{"dns"},
					DNS:     []string{"8.8.8.8"},
					Netmask: "255.255.255.0",
					Gateway: "3.4.5.6",
					Mac:     "fake-static-mac-address",
				}

				stubInterfaces(map[string]boshsettings.Network{
					"ethstatic": staticNetwork,
				})

				errCh := make(chan error)
				err := netManager.SetupNetworking(boshsettings.Networks{"static-network": staticNetwork}, errCh)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Validating dns configuration"))
			})
		})

		It("broadcasts MAC addresses for all interfaces", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			errCh := make(chan error)
			err := netManager.SetupNetworking(boshsettings.Networks{"dhcp-network": dhcpNetwork, "static-network": staticNetwork}, errCh)
			Expect(err).ToNot(HaveOccurred())

			broadcastErr := <-errCh // wait for all arpings
			Expect(broadcastErr).ToNot(HaveOccurred())

			Expect(addressBroadcaster.Value()).To(Equal([]boship.InterfaceAddress{
				boship.NewSimpleInterfaceAddress("ethstatic", "1.2.3.4"),
				boship.NewResolvingInterfaceAddress("ethdhcp", ipResolver),
			}))

		})

		It("skips vip networks", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethdhcp":   dhcpNetwork,
				"ethstatic": staticNetwork,
			})

			vipNetwork := boshsettings.Network{
				Type:    "vip",
				Default: []string{"dns"},
				DNS:     []string{"4.4.4.4", "5.5.5.5"},
				Mac:     "fake-vip-mac-address",
				IP:      "9.8.7.6",
			}

			err := netManager.SetupNetworking(boshsettings.Networks{
				"dhcp-network":   dhcpNetwork,
				"static-network": staticNetwork,
				"vip-network":    vipNetwork,
			}, nil)
			Expect(err).ToNot(HaveOccurred())

			networkConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethstatic")
			Expect(networkConfig).ToNot(BeNil())
			Expect(networkConfig.StringContents()).To(Equal(expectedNetworkConfigurationForStatic))
		})

		It("doesn't use vip networks dns", func() {
			stubInterfaces(map[string]boshsettings.Network{
				"ethstatic": staticNetwork,
			})

			vipNetwork := boshsettings.Network{
				Type:    "vip",
				Default: []string{"dns"},
				DNS:     []string{"4.4.4.4", "5.5.5.5"},
				Mac:     "fake-vip-mac-address",
				IP:      "9.8.7.6",
			}

			err := netManager.SetupNetworking(boshsettings.Networks{
				"vip-network":    vipNetwork,
				"static-network": staticNetwork,
			}, nil)
			Expect(err).ToNot(HaveOccurred())

			networkConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethstatic")
			Expect(networkConfig).ToNot(BeNil())
			Expect(networkConfig.StringContents()).ToNot(ContainSubstring("4.4.4.4"))
			Expect(networkConfig.StringContents()).ToNot(ContainSubstring("5.5.5.5"))
		})

		Context("when no MAC address is provided in the settings", func() {
			var staticNetworkWithoutMAC boshsettings.Network

			BeforeEach(func() {
				staticNetworkWithoutMAC = boshsettings.Network{
					Type:    "manual",
					IP:      "1.2.3.4",
					Netmask: "255.255.255.0",
					Gateway: "3.4.5.6",
					DNS:     []string{"8.8.8.8", "9.9.9.9"},
					Default: []string{"dns"},
				}
			})

			It("configures network for single device", func() {
				stubInterfaces(
					map[string]boshsettings.Network{
						"ethstatic": staticNetwork,
					},
				)

				err := netManager.SetupNetworking(boshsettings.Networks{
					"static-network": staticNetworkWithoutMAC,
				}, nil)
				Expect(err).ToNot(HaveOccurred())

				networkConfig := fs.GetFileTestStat("/etc/sysconfig/network/ifcfg-ethstatic")
				Expect(networkConfig).ToNot(BeNil())
				Expect(networkConfig.StringContents()).To(Equal(expectedNetworkConfigurationForStatic))
			})
		})
	})

	Describe("GetConfiguredNetworkInterfaces", func() {
		Context("when there are network devices", func() {
			BeforeEach(func() {
				stubInterfaces(map[string]boshsettings.Network{
					"fake-eth0": boshsettings.Network{Mac: "aa:bb"},
					"fake-eth1": boshsettings.Network{Mac: "cc:dd"},
					"fake-eth2": boshsettings.Network{Mac: "ee:ff"},
				})
			})

			writeIfcgfFile := func(iface string) {
				fs.WriteFileString(fmt.Sprintf("/etc/sysconfig/network/ifcfg-%s", iface), "fake-config")
			}

			It("returns networks that have ifcfg config present", func() {
				writeIfcgfFile("fake-eth0")
				writeIfcgfFile("fake-eth2")

				interfaces, err := netManager.GetConfiguredNetworkInterfaces()
				Expect(err).ToNot(HaveOccurred())

				Expect(interfaces).To(ConsistOf("fake-eth0", "fake-eth2"))
			})
		})

		Context("when there are no network devices", func() {
			It("returns empty list", func() {
				interfaces, err := netManager.GetConfiguredNetworkInterfaces()
				Expect(err).ToNot(HaveOccurred())
				Expect(interfaces).To(Equal([]string{}))
			})
		})
	})
}

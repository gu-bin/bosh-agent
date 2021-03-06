package settings

import (
	"fmt"

	"github.com/cloudfoundry/bosh-agent/platform/disk"
)

type DiskAssociations struct {
	Associations []DiskAssociation `json:"disk_associations"`
}

type DiskAssociation struct {
	Name    string `json:"name"`
	DiskCID string `json:"cid"`
}

const (
	RootUsername        = "root"
	VCAPUsername        = "vcap"
	AdminGroup          = "admin"
	SudoersGroup        = "bosh_sudoers"
	SshersGroup         = "bosh_sshers"
	EphemeralUserPrefix = "bosh_"
)

type Settings struct {
	AgentID   string    `json:"agent_id"`
	Blobstore Blobstore `json:"blobstore"`
	Disks     Disks     `json:"disks"`
	Env       Env       `json:"env"`
	Networks  Networks  `json:"networks"`
	Ntp       []string  `json:"ntp"`
	Mbus      string    `json:"mbus"`
	VM        VM        `json:"vm"`
}

type UpdateSettings struct {
	DiskAssociations []DiskAssociation `json:"disk_associations"`
	TrustedCerts     string            `json:"trusted_certs"`
}

type Source interface {
	PublicSSHKeyForUsername(string) (string, error)
	Settings() (Settings, error)
}

type Blobstore struct {
	Type    string                 `json:"provider"`
	Options map[string]interface{} `json:"options"`
}

type Disks struct {
	// e.g "/dev/sda", "1"
	System string `json:"system"`

	// Older CPIs returned disk settings as string
	// e.g "/dev/sdb", "2"
	// Newer CPIs will populate it in a hash
	// e.g {"path" => "/dev/sdc", "volume_id" => "3"}
	//     {"lun" => "0", "host_device_id" => "{host-device-id}"}
	Ephemeral interface{} `json:"ephemeral"`

	// Older CPIs returned disk settings as strings
	// e.g {"disk-3845-43758-7243-38754" => "/dev/sdc"}
	//     {"disk-3845-43758-7243-38754" => "3"}
	// Newer CPIs will populate it in a hash:
	// e.g {"disk-3845-43758-7243-38754" => {"path" => "/dev/sdc"}}
	//     {"disk-3845-43758-7243-38754" => {"volume_id" => "3"}}
	//     {"disk-3845-43758-7243-38754" => {"lun" => "0", "host_device_id" => "{host-device-id}"}}
	Persistent map[string]interface{} `json:"persistent"`

	RawEphemeral []DiskSettings `json:"raw_ephemeral"`
}

type DiskSettings struct {
	ID           string
	DeviceID     string
	VolumeID     string
	Lun          string
	HostDeviceID string
	Path         string

	// iscsi related
	InitiatorName string
	Username      string
	Target        string
	Password      string

	FileSystemType disk.FileSystemType
}

type VM struct {
	Name string `json:"name"`
}

func (s Settings) PersistentDiskSettings(diskID string) (DiskSettings, bool) {
	diskSettings := DiskSettings{}

	for key, settings := range s.Disks.Persistent {
		if key == diskID {
			diskSettings.ID = diskID

			if hashSettings, ok := settings.(map[string]interface{}); ok {
				if path, ok := hashSettings["path"]; ok {
					diskSettings.Path = path.(string)
				}
				if volumeID, ok := hashSettings["volume_id"]; ok {
					diskSettings.VolumeID = volumeID.(string)
				}
				if deviceID, ok := hashSettings["id"]; ok {
					diskSettings.DeviceID = deviceID.(string)
				}
				if lun, ok := hashSettings["lun"]; ok {
					diskSettings.Lun = lun.(string)
				}
				if hostDeviceID, ok := hashSettings["host_device_id"]; ok {
					diskSettings.HostDeviceID = hostDeviceID.(string)
				}

				if username, ok := hashSettings["username"]; ok {
					diskSettings.Username = username.(string)
				}
				if password, ok := hashSettings["password"]; ok {
					diskSettings.Password = password.(string)
				}
				if initiator, ok := hashSettings["initiator_name"]; ok {
					diskSettings.InitiatorName = initiator.(string)
				}
				if target, ok := hashSettings["target"]; ok {
					diskSettings.Target = target.(string)
				}

			} else {
				// Old CPIs return disk path (string) or volume id (string) as disk settings
				diskSettings.Path = settings.(string)
				diskSettings.VolumeID = settings.(string)
			}

			diskSettings.FileSystemType = s.Env.PersistentDiskFS
			return diskSettings, true
		}
	}

	return diskSettings, false
}

func (s Settings) EphemeralDiskSettings() DiskSettings {
	diskSettings := DiskSettings{}

	if s.Disks.Ephemeral != nil {
		if hashSettings, ok := s.Disks.Ephemeral.(map[string]interface{}); ok {
			if path, ok := hashSettings["path"]; ok {
				diskSettings.Path = path.(string)
			}
			if volumeID, ok := hashSettings["volume_id"]; ok {
				diskSettings.VolumeID = volumeID.(string)
			}
			if deviceID, ok := hashSettings["id"]; ok {
				diskSettings.DeviceID = deviceID.(string)
			}
			if lun, ok := hashSettings["lun"]; ok {
				diskSettings.Lun = lun.(string)
			}
			if hostDeviceID, ok := hashSettings["host_device_id"]; ok {
				diskSettings.HostDeviceID = hostDeviceID.(string)
			}
		} else {
			// Old CPIs return disk path (string) or volume id (string) as disk settings
			diskSettings.Path = s.Disks.Ephemeral.(string)
			diskSettings.VolumeID = s.Disks.Ephemeral.(string)
		}
	}

	return diskSettings
}

func (s Settings) RawEphemeralDiskSettings() (devices []DiskSettings) {
	return s.Disks.RawEphemeral
}

type Env struct {
	Bosh             BoshEnv             `json:"bosh"`
	PersistentDiskFS disk.FileSystemType `json:"persistent_disk_fs"`
}

func (e Env) GetPassword() string {
	return e.Bosh.Password
}

func (e Env) GetKeepRootPassword() bool {
	return e.Bosh.KeepRootPassword
}

func (e Env) GetRemoveDevTools() bool {
	return e.Bosh.RemoveDevTools
}

func (e Env) GetRemoveStaticLibraries() bool {
	return e.Bosh.RemoveStaticLibraries
}

func (e Env) GetAuthorizedKeys() []string {
	return e.Bosh.AuthorizedKeys
}

func (e Env) GetSwapSizeInBytes() *uint64 {
	if e.Bosh.SwapSizeInMB == nil {
		return nil
	}

	result := uint64(*e.Bosh.SwapSizeInMB * 1024 * 1024)
	return &result
}

type BoshEnv struct {
	Password              string   `json:"password"`
	KeepRootPassword      bool     `json:"keep_root_password"`
	RemoveDevTools        bool     `json:"remove_dev_tools"`
	RemoveStaticLibraries bool     `json:"remove_static_libraries"`
	AuthorizedKeys        []string `json:"authorized_keys"`
	SwapSizeInMB          *uint64  `json:"swap_size"`
	Mbus                  struct {
		Cert CertKeyPair `json:"cert"`
	} `json:"mbus"`

	IPv6 IPv6 `json:"ipv6"`
}

type CertKeyPair struct {
	PrivateKey  string `json:"private_key"`
	Certificate string `json:"certificate"`
}

type IPv6 struct {
	Enable bool `json:"enable"`
}

type DNSRecords struct {
	Version uint64      `json:"Version"`
	Records [][2]string `json:"records"`
}

type NetworkType string

const (
	NetworkTypeDynamic NetworkType = "dynamic"
	NetworkTypeVIP     NetworkType = "vip"
)

type Route struct {
	Destination string
	Gateway     string
	NetMask     string
}

type Routes []Route

type Network struct {
	Type NetworkType `json:"type"`

	IP       string `json:"ip"`
	Netmask  string `json:"netmask"`
	Gateway  string `json:"gateway"`
	Resolved bool   `json:"resolved"` // was resolved via DHCP
	UseDHCP  bool   `json:"use_dhcp"`

	Default []string `json:"default"`
	DNS     []string `json:"dns"`

	Mac string `json:"mac"`

	Preconfigured bool `json:"preconfigured"`

	Alias  string `json:"alias,omitempty"`
	Routes Routes `json:"routes,omitempty"`
}

type Networks map[string]Network

func (n Network) IsDefaultFor(category string) bool {
	return stringArrayContains(n.Default, category)
}

func (n Networks) NetworkForMac(mac string) (Network, bool) {
	for i := range n {
		if n[i].Mac == mac {
			return n[i], true
		}
	}

	return Network{}, false
}

func (n Networks) DefaultNetworkFor(category string) (Network, bool) {
	if len(n) == 1 {
		for _, net := range n {
			return net, true
		}
	}

	for _, net := range n {
		if net.IsDefaultFor(category) {
			return net, true
		}
	}

	return Network{}, false
}

func stringArrayContains(stringArray []string, str string) bool {
	for _, s := range stringArray {
		if s == str {
			return true
		}
	}
	return false
}

func (n Networks) DefaultIP() (ip string, found bool) {
	for _, networkSettings := range n {
		if ip == "" {
			ip = networkSettings.IP
		}
		if len(networkSettings.Default) > 0 {
			ip = networkSettings.IP
		}
	}

	if ip != "" {
		found = true
	}
	return
}

func (n Networks) IPs() (ips []string) {
	for _, net := range n {
		if net.IP != "" {
			ips = append(ips, net.IP)
		}
	}
	return
}

func (n Networks) HasInterfaceAlias() bool {
	for _, network := range n {
		if network.IsVIP() {
			// Skip VIP networks since we do not configure interfaces for them
			continue
		}

		if network.Alias != "" {
			return true
		}
	}

	return false
}

func (n Networks) IsPreconfigured() bool {
	for _, network := range n {
		if network.IsVIP() {
			// Skip VIP networks since we do not configure interfaces for them
			continue
		}

		if !network.Preconfigured {
			return false
		}
	}

	return true
}

func (n Network) String() string {
	return fmt.Sprintf(
		"type: '%s', ip: '%s', netmask: '%s', gateway: '%s', mac: '%s', resolved: '%t', preconfigured: '%t', use_dhcp: '%t', alias: '%s'",
		n.Type, n.IP, n.Netmask, n.Gateway, n.Mac, n.Resolved, n.Preconfigured, n.UseDHCP, n.Alias,
	)
}

func (n Network) IsDHCP() bool {
	if n.IsVIP() {
		return false
	}

	if n.isDynamic() {
		return true
	}

	if n.UseDHCP {
		return true
	}

	// If manual network does not have IP and Netmask it cannot be statically
	// configured. We want to keep track how originally the network was resolved.
	// Otherwise it will be considered as static on subsequent checks.
	isStatic := (n.IP != "" && n.Netmask != "")
	return n.Resolved || !isStatic
}

func (n Network) isDynamic() bool {
	return n.Type == NetworkTypeDynamic
}

func (n Network) IsVIP() bool {
	return n.Type == NetworkTypeVIP
}

//{
//	"agent_id": "bm-xxxxxxxx",
//	"blobstore": {
//		"options": {
//			"blobstore_path": "/var/vcap/micro_bosh/data/cache"
//		},
//		"provider": "local"
//	},
//	"disks": {
//		"ephemeral": "/dev/sdb",
//		"persistent": {
//			"vol-xxxxxx": "/dev/sdf"
//		},
//		"system": "/dev/sda1"
//	},
//	"env": {
//		"bosh": {
//			"password": null
//      },
//      "persistent_disk_fs": "xfs"
//	},
//  "trusted_certs": "very\nlong\nmultiline\nstring"
//	"mbus": "https://vcap:b00tstrap@0.0.0.0:6868",
//	"networks": {
//		"bosh": {
//			"cloud_properties": {
//				"subnet": "subnet-xxxxxx"
//			},
//			"default": [
//				"dns",
//				"gateway"
//			],
//			"dns": [
//				"xx.xx.xx.xx"
//			],
//			"gateway": null,
//			"ip": "xx.xx.xx.xx",
//			"netmask": null,
//			"type": "manual"
//		},
//		"vip": {
//			"cloud_properties": {},
//			"ip": "xx.xx.xx.xx",
//			"type": "vip"
//		}
//	},
//	"ntp": [
//		"0.north-america.pool.ntp.org",
//		"1.north-america.pool.ntp.org",
//		"2.north-america.pool.ntp.org",
//		"3.north-america.pool.ntp.org"
//	],
//	"vm": {
//		"name": "vm-xxxxxxxx"
//	}
//}

package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"

	"github.com/whiteboxvpn/cli/types"
	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Listener int

type Reply struct {
	Data string
}

type VPNDisconnectData struct {
	ServerAddress string
	ServerPort    int
}

var deviceName = "wg0"

const ALL_NETWORK_RANGE = "0.0.0.0/0"

func (l *Listener) VPNDisconnect(data VPNDisconnectData, reply *Reply) error {

	link, err := netlink.LinkByName(deviceName)
	if err != nil {
		log.Fatal("error finding link: ", err)
	}

	// Disable IP routes
	routingNet, err := netlink.ParseIPNet(ALL_NETWORK_RANGE)
	if err != nil {
		log.Fatal("error parsing routing address: ", err)
	}

	configureIpRules(false, routingNet, data.ServerPort)
	configureIpRoutes(false, link.Attrs().Index, data.ServerPort, routingNet)

	err = netlink.LinkDel(link)
	if err != nil {
		log.Fatal("error deleting link: ", err)
	}

	// Disable IPTables
	//configureIptables(false, deviceName, data.ServerAddress)

	rv := "done"
	fmt.Printf("Receive: %v\n", rv)
	*reply = Reply{rv}
	return nil
}

func (l *Listener) ConfigureWgInterface(configData types.ConfigData, reply *Reply) error {

	// Parse the server's wireguard public key
	serverPublicKey, err := wgtypes.ParseKey(string(configData.ServerPublicKeyData))

	// Set the allowed range (which is 0.0.0.0/0)
	_, zeroRange, err := net.ParseCIDR(ALL_NETWORK_RANGE)
	if err != nil {
		log.Fatal(err)
	}
	allowIpsFromServer := []net.IPNet{*zeroRange}

	serverIp := net.ParseIP(configData.ServerAddress)
	serverPort := configData.ServerPort
	peer := wgtypes.PeerConfig{
		PublicKey:  serverPublicKey,
		AllowedIPs: allowIpsFromServer,
		Endpoint: &net.UDPAddr{
			IP:   serverIp,
			Port: serverPort,
		},
	}

	// Parse the client wireguard private key
	clientPrivateKey, err := wgtypes.ParseKey(configData.ClientPrivateKey)
	if err != nil {
		log.Fatal("error parsing private key: ", err)
	}

	// Creating the network's "link" object
	var device netlink.Link
	existingLinks, err := netlink.LinkList()
	if err != nil {
		log.Fatal("error listing links: ", err)
	}
	linkExists := false
	for _, link := range existingLinks {
		if link.Attrs().Name == deviceName {
			linkExists = true
			device = link
		}
	}
	if !linkExists {
		la := netlink.NewLinkAttrs()
		la.Name = deviceName
		la.MTU = 1420
		device = &netlink.Wireguard{LinkAttrs: la}
		err = netlink.LinkAdd(device)
		if err != nil {
			log.Fatal("error adding new link: ", err)
		}
	}
	address, err := netlink.ParseAddr(configData.ClientAddress)
	if err != nil {
		log.Fatal("error parsing client address: ", err)
	}
	err = netlink.AddrReplace(device, address)
	if err != nil {
		log.Fatal("error setting ip address: ", err)
	}

	// Setting the link "up"
	err = netlink.LinkSetUp(device)
	if err != nil {
		log.Fatal("error setting up device: ", err)
	}

	routingNet, err := netlink.ParseIPNet(ALL_NETWORK_RANGE)
	if err != nil {
		msg := fmt.Sprintf("error parsing the address %s: ", ALL_NETWORK_RANGE)
		log.Fatal(msg, err)
	}

	// Configure The Network Interface route through the link
	configureIpRules(true, routingNet, serverPort)
	configureIpRoutes(true, device.Attrs().Index, serverPort, routingNet)

	// Configure device with the wireguard configuration
	cfg := wgtypes.Config{
		PrivateKey:   &clientPrivateKey,
		Peers:        []wgtypes.PeerConfig{peer},
		ReplacePeers: false,
		FirewallMark: &serverPort,
	}
	c, err := wgctrl.New()
	if err != nil {
		log.Fatalf("Error getting new wire guard client: %v", err.Error())
	}
	confDErr := c.ConfigureDevice(deviceName, cfg)
	if confDErr != nil {
		log.Fatalf("Error configuring device %s:\n\n\t%v", deviceName, confDErr.Error())
	}
	closeErr := c.Close()
	if closeErr != nil {
		log.Fatalf("Error closing wireguard client:\n\n\t%v", closeErr.Error())
	}

	rv := string(configData.ServerPublicKeyData)
	fmt.Printf("Finished configuration of device\n")
	*reply = Reply{rv}
	return nil
}

func main() {
	addy, err := net.ResolveTCPAddr("tcp", "0.0.0.0:12345")
	if err != nil {
		log.Fatal(err)
	}
	inbound, err := net.ListenTCP("tcp", addy)
	if err != nil {
		log.Fatal(err)
	}
	listener := new(Listener)
	rpc.Register(listener)
	rpc.Accept(inbound)
}

// configureIpRoutes accepts a boolean. If it is true, the system's IP rules
// will be configured to utilize the VPN. If it's false, those configurations
// will be removed
func configureIpRules(enable bool, routingNet *net.IPNet, tableIndex int) {

	rule1 := netlink.NewRule()
	rule1.Invert = true
	rule1.Src = routingNet
	rule1.Mark = tableIndex
	rule1.Table = tableIndex
	rule1.Family = 4 // IPv4

	rule2 := netlink.NewRule()
	rule2.Src = routingNet
	rule2.SuppressPrefixlen = 0
	rule2.Table = unix.RT_TABLE_MAIN
	rule2.Family = 4 // IPv4

	if enable {
		err := netlink.RuleAdd(rule1)
		if err != nil {
			log.Fatal("error adding network rule: ", err)
		}

		err = netlink.RuleAdd(rule2)
		if err != nil {
			log.Fatal("error adding network rule: ", err)
		}
	} else {
		err := netlink.RuleDel(rule1)
		if err != nil {
			log.Fatal("error deleting network rule: ", err)
		}

		err = netlink.RuleDel(rule2)
		if err != nil {
			log.Fatal("error deleting network rule: ", err)
		}
	}
}

// configureIpRoutes accepts a boolean. If it is true, the system's IP routes
// will be configured to utilize the VPN. If it's false, those configurations
// will be removed.
func configureIpRoutes(enable bool, deviceIndex int, tableIndex int, routingNet *net.IPNet) {

	if enable {
		route := netlink.Route{
			Dst:       routingNet,
			LinkIndex: deviceIndex,
			Table:     tableIndex,
			Scope:     unix.RT_SCOPE_LINK,
		}
		err := netlink.RouteReplace(&route)
		if err != nil {
			log.Fatal("error adding new route: ", err)
		}

	} else {
		link, err := netlink.LinkByIndex(deviceIndex)
		if err != nil {
			log.Fatal("error getting link by index: ", err)
		}

		routeList, err := netlink.RouteList(link, 4)
		if err != nil {
			log.Fatal("error listing routes: ", err)
		}

		for _, route := range routeList {
			if route.Table == tableIndex {
				err := netlink.RouteDel(&route)
				if err != nil {
					log.Fatal("error deleting route: ", err)
				}
			}
		}

	}
}

// configureIptables accepts a boolean. If it is true, the system's IP tables
// will be configured for using the VPN. If it's false, the IP tables
// configuration will be cleared out.
func configureIptables(enable bool, deviceName string, serverIp string) {

	mangleRuleSpecPost := fmt.Sprintf("-m mark --mark 51820 -p udp -j CONNMARK --save-mark --comment \"White Box VPN rule for %s\"", deviceName)
	mangleRuleSpecPre := fmt.Sprintf("-p udp -j CONNMARK --restore-mark --comment \"White Box VPN rule for  %s\"", deviceName)
	rawRuleSpecPost := fmt.Sprintf("! -i %s -d %s -m addrtype ! --src-type LOCAL -j DROP --comment \"White Box VPN rule for %s\"", deviceName, serverIp, deviceName)
	ipt, err := iptables.New()
	if enable {
		// Configure IPTables
		if err != nil {
			log.Fatal("error creating new iptables: ", err)
		}
		ipt.Insert("mangle", "PREROUTING", 1, mangleRuleSpecPre)
		ipt.Insert("mangle", "POSTROUTING", 1, mangleRuleSpecPost)
		ipt.Insert("raw", "PREROUTING", 1, rawRuleSpecPost)
	} else {
		ipt.DeleteIfExists("mangle", "PREROUTING", mangleRuleSpecPre)
		ipt.DeleteIfExists("mangle", "POSTROUTING", mangleRuleSpecPost)
		ipt.DeleteIfExists("raw", "PREROUTING", rawRuleSpecPost)
	}
}

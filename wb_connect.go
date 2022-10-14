package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"

	"github.com/c-robinson/iplib"
	"github.com/go-resty/resty/v2"
	"golang.org/x/crypto/ssh"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Reply struct {
	Data string
}

type ConfigData struct {
	ServerPublicKeyData string
	ClientAddress       string
	ServerAddress       string
	ServerPort          int
	ClientPrivateKey    string
}

func connect(serverName string) {

	accessToken := getToken()

	client := resty.New()

	// Get server IP and server ID whose name is "serverName"
	var servers []VpnServer
	serverQueryStr := fmt.Sprintf("serverName=%s", serverName)
	url := fmt.Sprintf("%s/api/servers", SITE_URL)
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(accessToken).
		SetQueryString(serverQueryStr).
		Get(url)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode() == 401 {
		log.Fatal("please login first")
	} else if resp.StatusCode() != 200 {
		log.Fatal("error: ", resp.Body())
	}
	err = json.Unmarshal([]byte(resp.String()), &servers)
	if err != nil {
		log.Fatal(err)
	}
	if len(servers) <= 0 {
		fmt.Println("no servers found with name", serverName)
		os.Exit(1)
		return
	}
	serverId := servers[0].Id
	serverIp := servers[0].Ipv4[0]

	// Get the server's SSH Public key from the white box API
	var publicKeyData map[string]string
	var signer ssh.Signer
	queryString := fmt.Sprintf("serverId=%s", serverId)
	url = fmt.Sprintf("%s/api/servers/sshPublicKeys", SITE_URL)
	resp, err = client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(accessToken).
		SetQueryString(queryString).
		Get(url)
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal([]byte(resp.String()), &publicKeyData)
	fullStr := []byte("ssh-rsa " + publicKeyData["publicKey"])
	pk, _, _, _, err := ssh.ParseAuthorizedKey(fullStr)
	sshPublicKey, err := ssh.ParsePublicKey(pk.Marshal())
	if err != nil {
		log.Fatal("unable to parse public ssh key: ", err)
	}

	// Prompt user for information regarding the user's private key
	defaultSshPrivateKey := fmt.Sprintf("/home/%s/.ssh/id_rsa", os.Getenv("USER"))
	privateKeyPromptText := fmt.Sprintf("Select the path to your SSH private key. (Leave blank for %s):", defaultSshPrivateKey)
	privateKeyPath := promptForString(privateKeyPromptText, defaultSshPrivateKey)
	key, err := os.ReadFile(privateKeyPath)
	if err != nil {
		log.Fatalf("unable to read private key: %v", err)
	}
	passwordPromptText := fmt.Sprintf("Enter the passphrase for key '%s': ", privateKeyPath)
	passphrase := promptForPassword(passwordPromptText)
	if len(passphrase) <= 0 {
		// Create the Signer for this private key.
		signer, err = ssh.ParsePrivateKey(key)
		if err != nil {
			log.Fatalf("unable to parse private key: %v", err)
		}
	} else {
		passphraseByteArray := []byte(passphrase)
		// Create the Signer for this private key.
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphraseByteArray)
		if err != nil {
			log.Fatalf("unable to parse private key: %v", err)
		}
	}

	// Set up the SSH client configuration
	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(sshPublicKey),
	}
	serverSshEndpoint := fmt.Sprintf("%s:%d", serverIp, 22)

	// Connect to ssh server
	sshClient, err := ssh.Dial("tcp", serverSshEndpoint, config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}
	defer sshClient.Close()

	// Create a new SSH Session
	session, err := sshClient.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	defer session.Close()

	// Run commands to find server-side IP addresses and to find an
	// availble client-side IP address
	var availableAddress net.IP
	var serverInterfaceList []string
	var serverInterfaceName string
	var serverWireguardPort int
	runCommandOnServer("wg show interfaces | xargs -d ' ' -I '{}' echo '{}'", func(line string) {
		if len(line) > 0 {
			serverInterfaceList = append(serverInterfaceList, line)
		}
	}, sshClient)

	for _, serverInterface := range serverInterfaceList {

		serverInterfaceName = serverInterface
		var usedIpAddresses []string
		var networkRange iplib.Net

		// Run command to get the network range this interface uses
		cmd := fmt.Sprintf("ip address show dev %s | awk '/scope global/ { print $2 }'", serverInterface)
		runCommandOnServer(cmd, func(line string) {
			ip, ipNetwork, err := iplib.ParseCIDR(line)
			if err != nil {
				log.Fatal("failed to parse CIDR: ", err)
			}
			usedIpAddresses = append(usedIpAddresses, ip.String())
			networkRange = ipNetwork
		}, sshClient)

		// Run command to get all used client IP addresses
		runCommandOnServer("wg show all allowed-ips | awk '{ print $3 }'", func(line string) {
			ip, _, err := iplib.ParseCIDR(line)
			if err != nil {
				log.Fatal("failed to parse CIDR: ", err)
			}
			usedIpAddresses = append(usedIpAddresses, ip.String())
		}, sshClient)

		// Loop over  to find an available IP address
		var newAddress = networkRange.FirstAddress()
		var lastAddress = networkRange.LastAddress()
		for newAddress.String() != lastAddress.String() {
			if !contains(usedIpAddresses, newAddress.String()) {
				availableAddress = newAddress
				break
			}
			newAddress = iplib.NextIP(newAddress)
		}

		if availableAddress != nil {
			cmd := fmt.Sprintf("wg show %s listen-port", serverInterface)
			runCommandOnServer(cmd, func(line string) {
				serverWireguardPort, err = strconv.Atoi(line)
				if err != nil {
					log.Fatal("error pulling server wireguard port", err)
				}
			}, sshClient)
			break
		}
	}
	clientIp := fmt.Sprintf("%s/32", availableAddress.String())

	// Get the wireguard public key from the white-box API
	var wgPublicKeyData map[string]string
	url = fmt.Sprintf("%s/api/servers/wgPublicKeys", SITE_URL)
	resp, err = client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(accessToken).
		SetQueryString(queryString).
		Get(url)
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal([]byte(resp.String()), &wgPublicKeyData)

	// Generate the client-side wireguard public and private keys
	clientPrivateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		log.Fatalf("Error generating private key: %v", err.Error())
	}
	clientPublicKey := clientPrivateKey.PublicKey()

	// Run command on vpn server to set up the peer
	session, err = sshClient.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	defer session.Close()
	setPeerCommandText := fmt.Sprintf("wg set %s listen-port %d peer %s allowed-ips %s", serverInterfaceName, serverWireguardPort, clientPublicKey, clientIp)
	if err := session.Run(setPeerCommandText); err != nil {
		log.Fatal("failed to run wg set command: ", err)
	}

	// Use white box daemon to set up wireguard tunnel
	var reply Reply
	rpcClient, err := rpc.Dial("tcp", "localhost:12345")
	if err != nil {
		log.Fatal(err)
	}
	configData := ConfigData{
		ServerPublicKeyData: wgPublicKeyData["publicKey"],
		ClientAddress:       clientIp,
		ServerAddress:       serverIp,
		ServerPort:          serverWireguardPort,
		ClientPrivateKey:    clientPrivateKey.String(),
	}
	err = rpcClient.Call("Listener.ConfigureWgInterface", configData, &reply)
	if err != nil {
		log.Fatal(err)
	}
}

// This function takes some bash command to be run on the VPN server. The
// output should be multi-line text. The second parameter (lineCallback) will
// be called for each line of text that comes from the output of the command.
func runCommandOnServer(command string, lineCallback func(line string), sshClient *ssh.Client) {
	session, err := sshClient.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	defer session.Close()
	output, err := session.Output(command)
	if err != nil {
		log.Fatal("failed to run command: ", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		lineCallback(scanner.Text())
	}
}

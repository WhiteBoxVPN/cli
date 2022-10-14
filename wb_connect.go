package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/c-robinson/iplib"
	"github.com/go-resty/resty/v2"
	"golang.org/x/crypto/ssh"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type VpnServer struct {
	IsAdmin bool   `json:"isAdmin"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Id      string `json:"id"`
}

func connect() {

	accessToken := getToken()

	client := resty.New()
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(accessToken).
		Get("http://localhost:3000/api/servers")
	if err != nil {
		log.Fatal(err)
	}

	var servers []VpnServer
	err = json.Unmarshal([]byte(resp.String()), &servers)
	if err != nil {
		log.Fatal(err)
	}
	serverId := servers[0].Id

	queryString := fmt.Sprintf("serverId=%s", serverId)

	resp, err = client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(accessToken).
		SetQueryString(queryString).
		Get("http://localhost:3000/api/servers/sshPublicKeys")
	if err != nil {
		log.Fatal(err)
	}

	var publicKeyData map[string]string
	err = json.Unmarshal([]byte(resp.String()), &publicKeyData)
	fullStr := []byte("ssh-rsa " + publicKeyData["publicKey"])
	pk, _, _, _, err := ssh.ParseAuthorizedKey(fullStr)

	sshPublicKey, err := ssh.ParsePublicKey(pk.Marshal())
	if err != nil {
		log.Fatal("unable to parse public ssh key: ", err)
	}

	key, err := os.ReadFile("/home/wesley/.ssh/id_rsa")
	if err != nil {
		log.Fatalf("unable to read private key: %v", err)
	}

	passphrase := getPassword("Enter the passphrase for key '/home/wesley/.ssh/id_rsa': ")
	passphraseByteArray := []byte(passphrase)

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKeyWithPassphrase(key, passphraseByteArray)
	if err != nil {
		log.Fatalf("unable to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(sshPublicKey),
	}

	// Connect to ssh server
	sshClient, err := ssh.Dial("tcp", "45.33.35.178:22", config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	defer session.Close()

	// Run command to find IP addresses on server
	var usedIpAddresses []string
	var serverVpnNetworks []iplib.Net
	runCommandOnServer("wg show interfaces | xargs -d ' ' -I '{}' echo '{}' | xargs -I '{}' ip address show dev '{}' | awk '/scope global/ { print $2 }'", func(line string) {
		ip, ipNetwork, err := iplib.ParseCIDR(line)
		if err != nil {
			log.Fatal("failed to parse CIDR: ", err)
		}
		usedIpAddresses = append(usedIpAddresses, ip.String())
		serverVpnNetworks = append(serverVpnNetworks, ipNetwork)
	}, sshClient)

	// Run command to get all used client IP addresses
	runCommandOnServer("wg show all allowed-ips | awk '{ print $3 }'", func(line string) {
		ip, _, err := iplib.ParseCIDR(line)
		if err != nil {
			log.Fatal("failed to parse CIDR: ", err)
		}
		usedIpAddresses = append(usedIpAddresses, ip.String())
	}, sshClient)

	// Loop over each network to find an available IP address
	var availableAddress net.IP
	for _, network := range serverVpnNetworks {
		newAddress := network.FirstAddress()
		for availableAddress == nil {
			newAddress = iplib.NextIP(newAddress)
			if !contains(usedIpAddresses, newAddress.String()) {
				availableAddress = newAddress
				break
			}
		}
	}

	clientIp := availableAddress.String()

	resp, err = client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(accessToken).
		SetQueryString(queryString).
		Get("http://localhost:3000/api/servers/wgPublicKeys")
	if err != nil {
		log.Fatal(err)
	}

	var wgPublicKeyData map[string]string
	err = json.Unmarshal([]byte(resp.String()), &wgPublicKeyData)
	serverPublicKey, err := wgtypes.ParseKey(wgPublicKeyData["publicKey"])
	if err != nil {
		log.Fatal("error parsing server public key: ", err)
	}

	clientPrivateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		log.Fatalf("Error generating private key: %v", err.Error())
	}
	clientPublicKey := clientPrivateKey.PublicKey()

	_, zeroRange, err := net.ParseCIDR("0.0.0.0/0")
	if err != nil {
		log.Fatal(err)
	}
	allowIpsFromServer := []net.IPNet{*zeroRange}

	peer := wgtypes.PeerConfig{
		PublicKey:  serverPublicKey,
		AllowedIPs: allowIpsFromServer,
	}

	cfg := wgtypes.Config{
		Peers:        []wgtypes.PeerConfig{peer},
		ReplacePeers: false,
	}

	c, err := wgctrl.New()
	if err != nil {
		log.Fatalf("Error getting new wire guard client: %v", err.Error())
	}
	device := "wg0"
	confDErr := c.ConfigureDevice(device, cfg)
	if confDErr != nil {
		log.Fatalf("Error configuring device %s:\n\n\t%v", device, confDErr.Error())
	}

	closeErr := c.Close()
	if closeErr != nil {
		log.Fatalf("Error closing wireguard client:\n\n\t%v", closeErr.Error())
	}

	// Run command to set up peer on server
	setPeerCommandText := fmt.Sprintf("wg set %s peer %s allowed-ips %s", device, clientPublicKey, clientIp)
	if err := session.Run(setPeerCommandText); err != nil {
		log.Fatal("failed to run command: ", err)
	}

}

// techEcho() - turns terminal echo on or off.
func termEcho(on bool) {
	// Common settings and variables for both stty calls.
	attrs := syscall.ProcAttr{
		Dir:   "",
		Env:   []string{},
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		Sys:   nil}
	var ws syscall.WaitStatus
	cmd := "echo"
	if on == false {
		cmd = "-echo"
	}

	// Enable/disable echoing.
	pid, err := syscall.ForkExec(
		"/bin/stty",
		[]string{"stty", cmd},
		&attrs)
	if err != nil {
		panic(err)
	}

	// Wait for the stty process to complete.
	_, err = syscall.Wait4(pid, &ws, 0, nil)
	if err != nil {
		panic(err)
	}
}

func getPassword(prompt string) string {

	fmt.Print(prompt)

	// Catch a ^C interrupt.
	// Make sure that we reset term echo before exiting.
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)
	go func() {
		for _ = range signalChannel {
			fmt.Println("\n^C interrupt.")
			termEcho(true)
			os.Exit(1)
		}
	}()

	// Echo is disabled, now grab the data.
	termEcho(false) // disable terminal echo
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	termEcho(true) // always re-enable terminal echo
	fmt.Println("")
	if err != nil {
		// The terminal has been reset, go ahead and exit.
		fmt.Println("ERROR:", err.Error())
		os.Exit(1)
	}
	return strings.TrimSpace(text)
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
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

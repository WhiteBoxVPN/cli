package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/c-robinson/iplib"
	"github.com/go-resty/resty/v2"
	"golang.org/x/crypto/ssh"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type VpnServer struct {
	IsAdmin bool   `json:"isAdmin"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Id      string `json:"id"`
}

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

	if resp.StatusCode() == 401 {
		log.Fatal("please login first")
	} else if resp.StatusCode() != 200 {
		log.Fatal("error: ", resp.Body())
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

	serverIp := "45.33.35.178"
	serverSshEndpoint := fmt.Sprintf("%s:%d", serverIp, 22)

	// Connect to ssh server
	sshClient, err := ssh.Dial("tcp", serverSshEndpoint, config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	defer session.Close()

	// Run command to find server-side IP addresses on server
	var availableAddress net.IP
	var serverInterfaceList []string
	var serverInterfaceName string
	var serverWireguardPort int

	// Find all available interfaces on server
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

	rpcClient, err := rpc.Dial("tcp", "localhost:12345")
	if err != nil {
		log.Fatal(err)
	}

	clientPrivateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		log.Fatalf("Error generating private key: %v", err.Error())
	}
	clientPublicKey := clientPrivateKey.PublicKey()

	configData := ConfigData{
		ServerPublicKeyData: wgPublicKeyData["publicKey"],
		ClientAddress:       clientIp,
		ServerAddress:       serverIp,
		ServerPort:          serverWireguardPort,
		ClientPrivateKey:    clientPrivateKey.String(),
	}

	// Run command to set up peer on server
	session, err = sshClient.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	defer session.Close()
	setPeerCommandText := fmt.Sprintf("wg set %s listen-port %d peer %s allowed-ips %s", serverInterfaceName, serverWireguardPort, clientPublicKey, clientIp)
	fmt.Println(setPeerCommandText)
	if err := session.Run(setPeerCommandText); err != nil {
		log.Fatal("failed to run wg set command: ", err)
	}

	var reply Reply
	err = rpcClient.Call("Listener.ConfigureWgInterface", configData, &reply)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Reply: %v, Data: %v", reply, reply.Data)

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
		for range signalChannel {
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

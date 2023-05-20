/* Copyright 2023 White Box VPN

This file is part of White Box VPN CLI.

White Box VPN CLI is free software: you can redistribute it and/or modify it
under the terms of the GNU General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option) any
later version.

White Box VPN CLI  is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for more
details.

You should have received a copy of the GNU General Public License along with
White Box VPN CLI. If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"flag"
	"fmt"
	"os"
)

const SITE_URL = "https://console.whiteboxvpn.com"
const OAUTH_DEVICE_CODE_URL = "https://whiteboxvpn.us.auth0.com/oauth/device/code"
const OAUTH_AUDIENCE_URL = "https://console.whiteboxvpn.com/api/"
const OAUTH_TOKEN_URL = "https://whiteboxvpn.us.auth0.com/oauth/token"
const OAUTH_CLIENT_ID = "tXcyY7reNAvr1zEBt6a7TW2aa4vnOS8N"

func main() {

	loginCommand := flag.NewFlagSet("login", flag.ExitOnError)
	serversCommand := flag.NewFlagSet("servers", flag.ExitOnError)
	serversListCommand := flag.NewFlagSet("list", flag.ExitOnError)
	connectCommand := flag.NewFlagSet("connect", flag.ExitOnError)
	serverName := connectCommand.String("server-name", "", "The name of the server")
	disconnectCommand := flag.NewFlagSet("disconnect", flag.ExitOnError)

	if len(os.Args) == 1 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "login":
		loginCommand.Parse(os.Args[2:])
	case "servers":
		serversCommand.Parse(os.Args[2:])
	case "connect":
		connectCommand.Parse(os.Args[2:])
	case "disconnect":
		disconnectCommand.Parse(os.Args[2:])
	default:
		printHelp()
	}

 	if loginCommand.Parsed() {
		login()
		return
	}

	if serversCommand.Parsed() {

		if len(os.Args) == 2 {
			fmt.Println("usage: wb servers <subcommand> [<args>]")
			fmt.Println("Available sub commands are:")
			fmt.Println(" list   List available servers")
			return
		}

		switch os.Args[2] {
		case "list":
			serversListCommand.Parse(os.Args[3:])
		}

		if serversListCommand.Parsed() {
			servers_list()
		}
	}

	if connectCommand.Parsed() {
		if len(os.Args) == 2 {
			fmt.Println("usage: wb connect <servername>")
		} else {
			connect(*serverName)
		}
	}

	if disconnectCommand.Parsed() {
		disconnect()
	}

}

func printHelp() {
	fmt.Println("usage: wb <command> [<subcommand>] [<args>]")
	fmt.Println("Available commands are:")
	fmt.Println(" login       Log in to your account")
	fmt.Println(" servers     List your VPN Servers")
	fmt.Println(" connect     Connect to a VPN server")
	fmt.Println(" disconnect  Disconnect to a VPN server")
}

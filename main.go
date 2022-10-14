package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {

	loginCommand := flag.NewFlagSet("login", flag.ExitOnError)
	serversCommand := flag.NewFlagSet("servers", flag.ExitOnError)
	serversListCommand := flag.NewFlagSet("list", flag.ExitOnError)
	connectCommand := flag.NewFlagSet("connect", flag.ExitOnError)

	if len(os.Args) == 1 {
		fmt.Println("usage: wb <command> [<subcommand>] [<args>]")
		fmt.Println("Available commands are:")
		fmt.Println(" login   Ask questions")
		fmt.Println(" servers  Send messages to your contacts")
		fmt.Println(" connect  Connect to a VPN server")
		return
	}

	switch os.Args[1] {
	case "login":
		loginCommand.Parse(os.Args[2:])
	case "servers":
		serversCommand.Parse(os.Args[2:])
	case "connect":
		connectCommand.Parse(os.Args[2:])
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
		connect()
	}

}

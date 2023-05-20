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
	"log"
	"net/rpc"
)

type VPNDisconnectData struct {
	ServerAddress string
	ServerPort    int
}

func disconnect() {
	var reply Reply

	rpcClient, err := rpc.Dial("tcp", "localhost:12345")
	if err != nil {
		log.Fatal(err)
	}

	data := VPNDisconnectData{
		ServerAddress: "45.33.35.178",
		ServerPort:    51820,
	}

	err = rpcClient.Call("Listener.VPNDisconnect", data, &reply)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Disconnected")
}

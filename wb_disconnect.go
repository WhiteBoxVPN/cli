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
	log.Printf("Reply: %v, Data: %v", reply, reply.Data)
}

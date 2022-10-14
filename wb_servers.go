package main

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/adrg/xdg"
	"github.com/go-resty/resty/v2"
)

func servers_list() {

	dirPath := fmt.Sprintf("%s/whitebox-vpn-cli", xdg.CacheHome)
	tokenFilePath := fmt.Sprintf("%s/token", dirPath)

	accessToken, err := ioutil.ReadFile(tokenFilePath)
	if err != nil {
		log.Fatal(err)
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(string(accessToken)).
		Get("http://localhost:3000/api/servers")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.String())
}

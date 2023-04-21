package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"text/tabwriter"

	"github.com/adrg/xdg"
	"github.com/go-resty/resty/v2"
)

func servers_list() {

	var serverList []VpnServer
	dirPath := fmt.Sprintf("%s/whitebox-vpn-cli", xdg.CacheHome)
	tokenFilePath := fmt.Sprintf("%s/token", dirPath)

	accessToken, err := ioutil.ReadFile(tokenFilePath)
	if err != nil {
		log.Fatal(err)
	}
	client := resty.New()
	url := fmt.Sprintf("%s/api/servers", SITE_URL)
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetAuthToken(string(accessToken)).
		Get(url)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal([]byte(resp.String()), &serverList)
	const padding = 3
	w := tabwriter.NewWriter(os.Stdout, 0, 0, padding, ' ', tabwriter.DiscardEmptyColumns)
	fmt.Fprintf(w, "Name\tIP\tStatus\tRegion\tIs Admin\n")
	fmt.Fprintf(w, "----\t--\t------\t------\t--------\n")
	for _, s := range serverList {
		var ip string = ""
		if len(s.Ipv4) > 0 {
			ip = s.Ipv4[0]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\n", s.Name, ip, s.Status, s.RegionLabel, s.IsAdmin)
	}
	w.Flush()
}

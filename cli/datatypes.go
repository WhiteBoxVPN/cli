package main

type VpnServer struct {
	IsAdmin     bool     `json:"isAdmin"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Id          string   `json:"id"`
	Ipv4        []string `json:"ipv4"`
	RegionLabel string   `json:"regionLabel"`
}

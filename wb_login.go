package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/adrg/xdg"
)

func login() {
	var accessToken string

	url := "https://dev-xbtylxua.us.auth0.com/oauth/device/code"
	clientId := "bsHSl9e6RCvArdJTumBzhZcceDRfo8au"
	payloadStr := fmt.Sprintf("client_id=%s&scope=profile email openid&audience=https://dev-xbtylxua.us.auth0.com/api/v2/", clientId)
	payload := strings.NewReader(payloadStr)

	req, _ := http.NewRequest("POST", url, payload)

	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	var resProps map[string]interface{}
	json.Unmarshal(body, &resProps)
	resError, containsError := resProps["error"]
	if containsError {
		log.Fatal("error getting device code: ", resError)
	}

	pollingInterval := resProps["interval"].(float64)

	fmt.Printf("Confirm the token matches and log in: %s\n", resProps["user_code"])
	cmd := exec.Command("xdg-open", resProps["verification_uri_complete"].(string))
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	tokenUrl := "https://dev-xbtylxua.us.auth0.com/oauth/token"

	grantType := "urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Adevice_code"
	tokenPayloadStr := fmt.Sprintf("grant_type=%s&device_code=%s&client_id=%s", grantType, resProps["device_code"], clientId)

	var userAuthenticated = false
	for !userAuthenticated {
		tokenPayload := strings.NewReader(tokenPayloadStr)
		tokenReq, _ := http.NewRequest("POST", tokenUrl, tokenPayload)
		tokenReq.Header.Add("content-type", "application/x-www-form-urlencoded")

		tokenRes, err := http.DefaultClient.Do(tokenReq)
		if err != nil {
			log.Fatal(err)
		}
		defer tokenRes.Body.Close()
		tokenResBody, _ := ioutil.ReadAll(tokenRes.Body)
		var tResProps map[string]interface{}
		json.Unmarshal(tokenResBody, &tResProps)

		if tokenRes.StatusCode == 200 {
			userAuthenticated = true
			accessToken = tResProps["access_token"].(string)
		} else {
			apiErr := tResProps["error"].(string)

			switch apiErr {
			case "slow_down":
				pollingInterval = pollingInterval * 2
			case "expired_token":
				fmt.Println("Token expired. Please try to log in again.")
				os.Exit(1)
			case "access_denied":
				fmt.Println("Access denied.")
				os.Exit(1)
			default:
				time.Sleep(time.Duration(pollingInterval) * time.Second)
			}
		}
	}

	dirPath := fmt.Sprintf("%s/whitebox-vpn-cli", xdg.CacheHome)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		log.Fatal(err)
	}

	tokenFilePath := fmt.Sprintf("%s/token", dirPath)
	f, err := os.Create(tokenFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	ioutil.WriteFile(tokenFilePath, []byte(accessToken), 0644)

	fmt.Println("Successfully logged in")
	return
}

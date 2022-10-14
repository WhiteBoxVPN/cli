package main

import (
  "fmt"
  "log"
  "io/ioutil"

  "github.com/adrg/xdg"
)

func getToken() string {
  dirPath := fmt.Sprintf("%s/whitebox-vpn-cli", xdg.CacheHome)
  tokenFilePath := fmt.Sprintf("%s/token", dirPath)

  accessToken, err := ioutil.ReadFile(tokenFilePath)
  if err != nil {
    log.Fatal(err)
  }

  return string(accessToken)
}

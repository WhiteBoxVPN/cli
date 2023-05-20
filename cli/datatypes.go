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

type VpnServer struct {
	IsAdmin     bool     `json:"isAdmin"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Id          string   `json:"id"`
	Ipv4        []string `json:"ipv4"`
	RegionLabel string   `json:"regionLabel"`
}

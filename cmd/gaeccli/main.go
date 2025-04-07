/*
 * Copyright (C) 2025 Lewis Krishnamurti
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/lewiskrishnamurti/gaec"
)

func main() {
	serial := flag.String("serial", "", "Emulator serial number (e.g., emulator-5554)")
	cmdStr := flag.String("cmd", "", "Command to execute AFTER optional authentication (e.g., 'avd', 'gsm', 'network', 'kill', etc.)")
	authToken := flag.String("auth_token", "", "Optional authentication token for the emulator console")

	flag.Parse()

	if *serial == "" {
		log.Println("Error: -serial flag is required")
		flag.Usage()
		os.Exit(1)
	}
	if *cmdStr == "" {
		log.Println("Error: -cmd flag is required")
		flag.Usage()
		os.Exit(1)
	}

	console, err := gaec.GetConsole(*serial)
	if err != nil {
		log.Fatalf("Error getting console for %s: %v", *serial, err)
	}
	defer console.Close()

	log.Printf("Connected to console for %s\n", *serial)

	if *authToken != "" {
		log.Println("Attempting authentication with provided token...")
		err := console.AuthenticateConsole(*authToken)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}
		log.Println("Authentication successful, proceeding with command.")
	} else {
		log.Println("No auth token provided, proceeding without authentication.")
	}

	parts := strings.Fields(*cmdStr)
	if len(parts) == 0 {
		log.Fatalf("Error: Empty command string provided")
	}
	command := strings.ToLower(parts[0])
	args := parts[1:]

	switch command {
	case "avd":
		name, err := console.GetAvdName()
		if err != nil {
			log.Fatalf("Error getting AVD name: %v", err)
		}
		fmt.Printf("AVD Name: %s\n", name)

	case "gsm":
		status, err := console.GetGsmStatus()
		if err != nil {
			log.Fatalf("Error getting GSM status: %v", err)
		}
		fmt.Printf("GSM Status:\n  Voice: %s\n  Data: %s\n", status.Voice, status.Data)

	case "network":
		status, err := console.GetNetworkStatus()
		if err != nil {
			log.Fatalf("Error getting Network status: %v", err)
		}
		fmt.Printf("Network Status:\n  Speed Index: %d\n  Latency Index: %d\n",
			status.Speed, status.Latency)

	case "kill":
		log.Println("Sending KILL command...")
		console.Kill()
		log.Println("KILL command sent.")
		os.Exit(0)

	case "call":
		if len(args) != 1 {
			log.Fatalf("Usage: -cmd 'call <number>'")
		}
		number := args[0]
		log.Printf("Initiating call from %s...", number)
		koMsg, err := console.Call(number)
		handleCommandResult("Call", koMsg, err)

	case "cancel":
		if len(args) != 1 {
			log.Fatalf("Usage: -cmd 'cancel <number>'")
		}
		number := args[0]
		log.Printf("Cancelling call %s...", number)
		koMsg, err := console.CancelCall(number)
		handleCommandResult("Cancel Call", koMsg, err)

	case "sms":
		if len(args) < 2 {
			log.Fatalf("Usage: -cmd 'sms <number> <message>'")
		}
		number := args[0]
		message := strings.Join(args[1:], " ")
		log.Printf("Sending SMS from %s: '%s'...", number, message)
		koMsg, err := console.SendSms(number, message)
		handleCommandResult("Send SMS", koMsg, err)

	case "speed":
		if len(args) != 1 {
			log.Fatalf("Usage: -cmd 'speed <index>'")
		}
		index, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatalf("Invalid speed index '%s': %v", args[0], err)
		}
		log.Printf("Setting network speed to index %d...", index)
		koMsg, err := console.SetNetworkSpeed(index)
		handleCommandResult("Set Network Speed", koMsg, err)

	case "latency":
		if len(args) != 1 {
			log.Fatalf("Usage: -cmd 'latency <index>'")
		}
		index, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatalf("Invalid latency index '%s': %v", args[0], err)
		}
		log.Printf("Setting network latency to index %d...", index)
		koMsg, err := console.SetNetworkLatency(index)
		handleCommandResult("Set Network Latency", koMsg, err)

	case "gps":
		if len(args) != 3 {
			log.Fatalf("Usage: -cmd 'gps <longitude> <latitude> <elevation>'")
		}
		lon, errLon := strconv.ParseFloat(args[0], 64)
		lat, errLat := strconv.ParseFloat(args[1], 64)
		alt, errAlt := strconv.ParseFloat(args[2], 64)
		if errLon != nil || errLat != nil || errAlt != nil {
			log.Fatalf("Invalid GPS coordinates: %v, %v, %v", errLon, errLat, errAlt)
		}
		log.Printf("Sending GPS fix: lon=%f, lat=%f, alt=%f...", lon, lat, alt)
		koMsg, err := console.SendLocation(lon, lat, alt)
		handleCommandResult("Send Location", koMsg, err)

	default:
		log.Fatalf("Error: Unknown command '%s'", command)
	}
}

func handleCommandResult(cmdName string, koMsg string, err error) {
	if err != nil {
		log.Fatalf("Error executing '%s': %v", cmdName, err)
	} else if koMsg != "" {
		fmt.Printf("%s failed: %s\n", cmdName, koMsg)
	} else {
		fmt.Printf("%s successful.\n", cmdName)
	}
}

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

package gaec

import (
	"fmt"
	"log"
	"strings"
)

const (
	commandAuth = "auth %s\r\n"
	commandKill = "kill\r\n"
)

func (ec *EmulatorConsole) AuthenticateConsole(authToken string) error {
	fmt.Printf("Attempting to authenticate with token: %s\n", authToken)

	command := fmt.Sprintf(commandAuth, authToken)
	response, err := ec.processCommand(command)
	if err != nil {
		return fmt.Errorf("failed to send/process auth command: %w", err)
	}

	if response == "" {
		fmt.Println("Authentication successful.")
		return nil
	}

	return fmt.Errorf("authentication failed: %s", strings.TrimSpace(response))
}

func (ec *EmulatorConsole) Kill() {
	_, err := ec.processCommand(commandKill)
	if err != nil {
		log.Printf("%s: Error processing KILL command for port %d: %v\n", LogTag, ec.port, err)
	}

	ec.Close()
}

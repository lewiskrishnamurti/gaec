/*
 * Copyright (C) 2007 The Android Open Source Project
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
 *
 * ------------------------------------------------------------------
 * Modifications to this file made in 2025 by Lewis Krishnamurti.
 * Copyright 2025 Lewis Krishnamurti.
 * ------------------------------------------------------------------
 */

package gaec

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	waitTime              = 5 * time.Millisecond
	stdTimeout            = 5 * time.Second
	host                  = "127.0.0.1"
	commandPing           = "help\r\n"
	commandAvdName        = "avd name\r\n"
	commandGsmStatus      = "gsm status\r\n"
	commandGsmCall        = "gsm call %s\r\n"
	commandGsmCancelCall  = "gsm cancel %s\r\n"
	commandGsmData        = "gsm data %s\r\n"
	commandGsmVoice       = "gsm voice %s\r\n"
	commandSmsSend        = "sms send %s %s\r\n"
	commandNetworkStatus  = "network status\r\n"
	commandNetworkSpeed   = "network speed %s\r\n"
	commandNetworkLatency = "network delay %s\r\n"
	commandGps            = "geo fix %f %f %f\r\n"
	resultOK              = ""
	bufferSize            = 1024
)

var (
	okResultBytes = []byte("OK\r\n")
	koResultBytes = []byte("KO")
	crLfBytes     = []byte("\r\n")
)

var (
	reKO                 = regexp.MustCompile(`KO:\\s+(.*)`)
	sEmulatorRegexp      = regexp.MustCompile(`emulator-(\\d+)`)
	sVoiceStatusRegexp   = regexp.MustCompile(`(?i)gsm\\s+voice\\s+state:\\s*([a-z]+)`)
	sDataStatusRegexp    = regexp.MustCompile(`(?i)gsm\\s+data\\s+state:\\s*([a-z]+)`)
	sDownloadSpeedRegexp = regexp.MustCompile(`(?i)\\s+download\\s+speed:\\s+(\\d+)\\s+bits.*`)
	sMinLatencyRegexp    = regexp.MustCompile(`(?i)\\s+minimum\\s+latency:\\s+(\\d+)\\s+ms`)
)

var (
	minLatencies = []int{
		0,   // No delay
		150, // gprs
		80,  // edge/egprs
		35,  // umts/3g
	}
	downloadSpeeds = []int{
		0,        // full speed
		14400,    // gsm
		43200,    // hscsd
		80000,    // gprs
		236800,   // edge/egprs
		1920000,  // umts/3g
		14400000, // hsdpa
	}
	networkSpeeds = []string{
		"full",
		"gsm",
		"hscsd",
		"gprs",
		"edge",
		"umts",
		"hsdpa",
	}
	networkLatencies = []string{
		"none",
		"gprs",
		"edge",
		"umts",
	}
)

type GsmMode string

const (
	GsmModeUnknown      GsmMode = "unknown"
	GsmModeUnregistered GsmMode = "unregistered"
	GsmModeHome         GsmMode = "home"
	GsmModeRoaming      GsmMode = "roaming"
	GsmModeSearching    GsmMode = "searching"
	GsmModeDenied       GsmMode = "denied"
	gsmModeOff          GsmMode = "off"
	gsmModeOn           GsmMode = "on"
)

var gsmModeTags = map[string]GsmMode{
	"unregistered": GsmModeUnregistered,
	"off":          GsmModeUnregistered,
	"home":         GsmModeHome,
	"on":           GsmModeHome,
	"roaming":      GsmModeRoaming,
	"searching":    GsmModeSearching,
	"denied":       GsmModeDenied,
}

func GetGsmModeEnum(tag string) GsmMode {
	mode, ok := gsmModeTags[strings.ToLower(tag)]
	if !ok {
		return GsmModeUnknown
	}
	return mode
}

func (m GsmMode) Tag() string {
	if m == GsmModeUnknown {
		return ""
	}
	return string(m)
}

type GsmStatus struct {
	Voice GsmMode
	Data  GsmMode
}

type NetworkStatus struct {
	Speed   int // Index in downloadSpeeds, -1 if unknown
	Latency int // Index in minLatencies, -1 if unknown
}

type EmulatorConsole struct {
	port   int
	conn   net.Conn
	reader *bufio.Reader
	buffer []byte
	mu     sync.Mutex
}

var (
	emulatorsMutex sync.Mutex
	emulators      = make(map[int]*EmulatorConsole)
)

const LogTag = "EmulatorConsole"

func getLatencyIndex(value string) int {
	latency, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	for i, l := range minLatencies {
		if l == latency {
			return i
		}
	}
	return -1
}

func getSpeedIndex(value string) int {
	speed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	for i, s := range downloadSpeeds {
		if s == speed {
			return i
		}
	}
	return -1
}

func GetEmulatorPort(serialNumber string) (int, error) {
	match := sEmulatorRegexp.FindStringSubmatch(serialNumber)
	if len(match) == 2 {
		port, err := strconv.Atoi(match[1])
		if err == nil && port > 0 {
			return port, nil
		}
		return 0, fmt.Errorf("failed to parse port number from regex match: %s, error: %w", match[1], err)
	}
	return 0, fmt.Errorf("serial number '%s' does not match expected emulator pattern", serialNumber)
}

func retrieveConsole(port int) *EmulatorConsole {
	emulatorsMutex.Lock()
	defer emulatorsMutex.Unlock()

	console, exists := emulators[port]
	if !exists {
		log.Printf("%s: Creating emulator console for %d\n", LogTag, port)
		console = &EmulatorConsole{
			port:   port,
			buffer: make([]byte, bufferSize),
		}
		emulators[port] = console
	}
	return console
}

func removeConsole(port int) {
	emulatorsMutex.Lock()
	defer emulatorsMutex.Unlock()

	_, exists := emulators[port]
	if exists {
		log.Printf("%s: Removing emulator console for %d\n", LogTag, port)
		delete(emulators, port)
	}
}

func GetConsole(serialNumber string) (*EmulatorConsole, error) {
	port, err := GetEmulatorPort(serialNumber)
	if err != nil {
		log.Printf("%s: Failed to find emulator port from serial: %s, error: %v\n", LogTag, serialNumber, err)
		return nil, fmt.Errorf("failed to get emulator port: %w", err)
	}

	console := retrieveConsole(port)

	if !console.checkConnection() {
		removeConsole(console.port)
		return nil, fmt.Errorf("failed initial connection/ping to emulator console on port %d", port)
	}

	return console, nil
}

func (ec *EmulatorConsole) checkConnection() bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.conn == nil {
		addr := fmt.Sprintf("%s:%d", host, ec.port)
		conn, err := net.DialTimeout("tcp", addr, stdTimeout)
		if err != nil {
			log.Printf("%s: Failed to start Emulator console for %d: %v\n", LogTag, ec.port, err)
			ec.conn = nil
			ec.reader = nil
			return false
		}
		ec.conn = conn
		ec.reader = bufio.NewReader(conn)

		_, err = ec.readLines()
		if err != nil {
			log.Printf("%s: Failed to read initial output from console %d: %v\n", LogTag, ec.port, err)
			if ec.conn != nil {
				ec.conn.Close()
			}
			ec.conn = nil
			ec.reader = nil
			return false
		}
		log.Printf("%s: Connection established to %d\n", LogTag, ec.port)
	}

	return ec.pingInternal()
}

func (ec *EmulatorConsole) pingInternal() bool {
	if ec.sendCommandInternal(commandPing) {
		_, err := ec.readLines()
		if err == nil {
			return true
		}
		log.Printf("%s: Ping failed for port %d: error reading response: %v\n", LogTag, ec.port, err)
	} else {
		log.Printf("%s: Ping failed for port %d: error sending command\n", LogTag, ec.port)
	}

	if ec.conn != nil {
		ec.conn.Close()
	}
	ec.conn = nil
	ec.reader = nil
	return false
}

func (ec *EmulatorConsole) Ping() bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.conn == nil {
		return false
	}
	return ec.pingInternal()
}

func (ec *EmulatorConsole) Close() {
	ec.mu.Lock()
	portToClose := ec.port
	if portToClose == -1 {
		ec.mu.Unlock()
		return
	}

	ec.port = -1
	connToClose := ec.conn
	ec.conn = nil
	ec.reader = nil
	ec.buffer = nil
	ec.mu.Unlock()

	removeConsole(portToClose)

	if connToClose != nil {
		err := connToClose.Close()
		if err != nil {
			log.Printf("%s: Failed to close EmulatorConsole connection for port %d: %v\n", LogTag, portToClose, err)
		}
	}
	log.Printf("%s: Closed console for port %d\n", LogTag, portToClose)
}

func writeFully(w io.Writer, data []byte, timeout time.Duration) error {
	if conn, ok := w.(net.Conn); ok {
		conn.SetWriteDeadline(time.Now().Add(timeout))
		defer conn.SetWriteDeadline(time.Time{})
	}

	totalWritten := 0
	for totalWritten < len(data) {
		n, err := w.Write(data[totalWritten:])
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		totalWritten += n
	}
	return nil
}

func (ec *EmulatorConsole) sendCommandInternal(command string) bool {
	if ec.conn == nil {
		log.Printf("%s: Cannot send command, no connection for port %d\n", LogTag, ec.port)
		return false
	}

	bCommand := []byte(command)

	err := writeFully(ec.conn, bCommand, stdTimeout)
	if err != nil {
		log.Printf("%s: Exception sending command '%s' to %d: %v\n", LogTag, strings.TrimSpace(command), ec.port, err)
		return false
	}
	return true
}

func (ec *EmulatorConsole) readLines() ([]string, error) {
	if ec.conn == nil {
		return nil, errors.New("no connection available")
	}
	if ec.buffer == nil {
		return nil, errors.New("read buffer is nil (console likely closed)")
	}

	bytesRead := 0
	startTime := time.Now()

	for {
		err := ec.conn.SetReadDeadline(time.Now().Add(stdTimeout))
		if err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}

		if bytesRead == len(ec.buffer) {
			return nil, fmt.Errorf("read buffer overflow after %d bytes", bytesRead)
		}

		n, err := ec.conn.Read(ec.buffer[bytesRead:])

		if n > 0 {
			bytesRead += n
			if ec.endsWithOK(bytesRead) || ec.lastLineIsKO(bytesRead) {
				msg := string(ec.buffer[:bytesRead])
				ec.conn.SetReadDeadline(time.Time{})
				lines := strings.Split(msg, "\r\n")
				if len(lines) > 0 && lines[len(lines)-1] == "" {
					return lines[:len(lines)-1], nil
				}
				return lines, nil
			}
			startTime = time.Now()
		}

		if err != nil {
			ec.conn.SetReadDeadline(time.Time{})
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(startTime) > stdTimeout {
					return nil, fmt.Errorf("timeout waiting for OK/KO response after %v", time.Since(startTime))
				}
				time.Sleep(waitTime)
				continue
			}
			return nil, fmt.Errorf("error reading from console: %w", err)
		}

		if time.Since(startTime) > stdTimeout {
			ec.conn.SetReadDeadline(time.Time{})
			return nil, fmt.Errorf("timeout waiting for OK/KO response after %v (no data received)", time.Since(startTime))
		}
		time.Sleep(waitTime)
	}
}

func (ec *EmulatorConsole) endsWithOK(bytesRead int) bool {
	if ec.buffer == nil || bytesRead < len(okResultBytes) {
		return false
	}
	return bytes.Equal(ec.buffer[bytesRead-len(okResultBytes):bytesRead], okResultBytes)
}

func (ec *EmulatorConsole) lastLineIsKO(bytesRead int) bool {
	if ec.buffer == nil || bytesRead < len(crLfBytes) {
		return false
	}
	if !bytes.Equal(ec.buffer[bytesRead-len(crLfBytes):bytesRead], crLfBytes) {
		return false
	}

	lastLineStart := 0
	if bytesRead > len(crLfBytes) {
		prevCrLf := bytes.LastIndex(ec.buffer[:bytesRead-len(crLfBytes)], crLfBytes)
		if prevCrLf != -1 {
			lastLineStart = prevCrLf + len(crLfBytes)
		}
	}

	if lastLineStart+len(koResultBytes) <= bytesRead-len(crLfBytes) {
		return bytes.Equal(ec.buffer[lastLineStart:lastLineStart+len(koResultBytes)], koResultBytes)
	}

	return false
}

func (ec *EmulatorConsole) processCommand(command string) (string, error) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.sendCommandInternal(command) {
		return "", fmt.Errorf("unable to send command '%s' to the emulator", strings.TrimSpace(command))
	}

	result, err := ec.readLines()

	if err != nil {
		return "", fmt.Errorf("unable to read response for command '%s': %w", strings.TrimSpace(command), err)
	}

	if len(result) > 0 {
		lastLine := result[len(result)-1]
		match := reKO.FindStringSubmatch(lastLine)
		if len(match) == 2 {
			return strings.TrimSpace(match[1]), nil
		}
		return resultOK, nil
	}

	return "", errors.New("received empty or invalid response from emulator after command")
}

func (ec *EmulatorConsole) GetAvdName() (string, error) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.sendCommandInternal(commandAvdName) {
		return "", errors.New("failed to send 'avd name' command")
	}

	result, err := ec.readLines()

	if err != nil {
		return "", fmt.Errorf("failed to read response for 'avd name': %w", err)
	}

	if len(result) > 0 {
		lastLine := result[len(result)-1]
		match := reKO.FindStringSubmatch(lastLine)
		if len(match) == 2 {
			return "", fmt.Errorf("emulator returned error for 'avd name': %s", strings.TrimSpace(match[1]))
		}

		if len(result) >= 1 {
			return result[0], nil
		}
	}

	log.Printf("%s: 'avd name' result did not match expected format: %v\n", LogTag, result)
	return "", errors.New("invalid response format for 'avd name' command")
}

func (ec *EmulatorConsole) GetNetworkStatus() (*NetworkStatus, error) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.sendCommandInternal(commandNetworkStatus) {
		return nil, errors.New("failed to send 'network status' command")
	}

	result, err := ec.readLines()

	if err != nil {
		return nil, fmt.Errorf("failed to read response for 'network status': %w", err)
	}

	status := &NetworkStatus{Speed: -1, Latency: -1}

	if len(result) > 0 {
		lastLine := result[len(result)-1]
		matchKO := reKO.FindStringSubmatch(lastLine)
		if len(matchKO) == 2 {
			return nil, fmt.Errorf("emulator returned error for 'network status': %s", strings.TrimSpace(matchKO[1]))
		}
	} else {
		return nil, errors.New("received empty response for 'network status'")
	}

	for _, line := range result {
		matchSpeed := sDownloadSpeedRegexp.FindStringSubmatch(line)
		if len(matchSpeed) == 2 {
			status.Speed = getSpeedIndex(matchSpeed[1])
			continue
		}

		matchLatency := sMinLatencyRegexp.FindStringSubmatch(line)
		if len(matchLatency) == 2 {
			status.Latency = getLatencyIndex(matchLatency[1])
			continue
		}
	}

	return status, nil
}

func (ec *EmulatorConsole) GetGsmStatus() (*GsmStatus, error) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if !ec.sendCommandInternal(commandGsmStatus) {
		return nil, errors.New("failed to send 'gsm status' command")
	}

	result, err := ec.readLines()

	if err != nil {
		return nil, fmt.Errorf("failed to read response for 'gsm status': %w", err)
	}

	status := &GsmStatus{Voice: GsmModeUnknown, Data: GsmModeUnknown}

	if len(result) > 0 {
		lastLine := result[len(result)-1]
		matchKO := reKO.FindStringSubmatch(lastLine)
		if len(matchKO) == 2 {
			return nil, fmt.Errorf("emulator returned error for 'gsm status': %s", strings.TrimSpace(matchKO[1]))
		}
	} else {
		return nil, errors.New("received empty response for 'gsm status'")
	}

	for _, line := range result {
		matchVoice := sVoiceStatusRegexp.FindStringSubmatch(line)
		if len(matchVoice) == 2 {
			status.Voice = GetGsmModeEnum(matchVoice[1])
			continue
		}

		matchData := sDataStatusRegexp.FindStringSubmatch(line)
		if len(matchData) == 2 {
			status.Data = GetGsmModeEnum(matchData[1])
			continue
		}
	}

	return status, nil
}

func (ec *EmulatorConsole) SetGsmVoiceMode(mode GsmMode) (string, error) {
	if mode == GsmModeUnknown {
		return "", fmt.Errorf("invalid parameter: GsmMode cannot be UNKNOWN")
	}
	tag := mode.Tag()
	if tag == "" {
		return "", fmt.Errorf("invalid GsmMode tag for mode: %s", mode)
	}
	command := fmt.Sprintf(commandGsmVoice, tag)
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) SetGsmDataMode(mode GsmMode) (string, error) {
	if mode == GsmModeUnknown {
		return "", fmt.Errorf("invalid parameter: GsmMode cannot be UNKNOWN")
	}
	tag := mode.Tag()
	if tag == "" {
		return "", fmt.Errorf("invalid GsmMode tag for mode: %s", mode)
	}
	command := fmt.Sprintf(commandGsmData, tag)
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) Call(number string) (string, error) {
	command := fmt.Sprintf(commandGsmCall, number)
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) CancelCall(number string) (string, error) {
	command := fmt.Sprintf(commandGsmCancelCall, number)
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) SendSms(senderNumber string, message string) (string, error) {
	command := fmt.Sprintf(commandSmsSend, senderNumber, message)
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) SetNetworkSpeed(selectionIndex int) (string, error) {
	if selectionIndex < 0 || selectionIndex >= len(networkSpeeds) {
		return "", fmt.Errorf("invalid network speed index: %d (must be 0-%d)", selectionIndex, len(networkSpeeds)-1)
	}
	command := fmt.Sprintf(commandNetworkSpeed, networkSpeeds[selectionIndex])
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) SetNetworkLatency(selectionIndex int) (string, error) {
	if selectionIndex < 0 || selectionIndex >= len(networkLatencies) {
		return "", fmt.Errorf("invalid network latency index: %d (must be 0-%d)", selectionIndex, len(networkLatencies)-1)
	}
	command := fmt.Sprintf(commandNetworkLatency, networkLatencies[selectionIndex])
	return ec.processCommand(command)
}

func (ec *EmulatorConsole) SendLocation(longitude, latitude, elevation float64) (string, error) {
	command := fmt.Sprintf(commandGps, longitude, latitude, elevation)
	return ec.processCommand(command)
}

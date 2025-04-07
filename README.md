# gaec
Golang Implementation of ddmlib's Android Emulator Console helper/wrapper.

# Installation
`go get github.com/lewiskrishnamurti/gaec`

# Features
- A near direct port of ddmlib's [EmulatorConsole.java](https://android.googlesource.com/platform/tools/base/+/refs/heads/main/ddmlib/src/main/java/com/android/ddmlib/EmulatorConsole.java)
- Additional authentication support to gain access to auth-locked commands such as `kill`

# Example
A basic CLI example is in [gaccli](./cmd/gaeccli/main.go).

# Acknowledgements
Thank you to The Android Open Source Project for the original open source ddmlib Java library.

---
Inspired by [gadb](https://github.com/electricbubble/gadb/tree/master) - check it out!
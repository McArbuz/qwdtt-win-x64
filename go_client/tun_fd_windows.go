//go:build windows

package main

import (
"fmt"
"os"
)

func recvTunFD(string) (*os.File, error) {
return nil, fmt.Errorf("TUN fd is Android-only")
}
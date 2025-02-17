package test

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"

	gos7logo "github.com/axon-expert/gos7-logo-client"
)

var client gos7logo.Client

func TestMain(m *testing.M) {
	cl, err := gos7logo.NewClient("localhost:102", 0, 1, 0x100, 0x200)
	if err != nil {
		fmt.Printf("failed connect: %s\n", err)
	}
	client = cl

	code := m.Run()
	if err := client.Disconnect(); err != nil {
		fmt.Printf("failed to disconnect: %s\n", err)
	}
	os.Exit(code)
}

func FuzzClientWriteRead(f *testing.F) {
	f.Add("VD3", uint32(rand.Intn(100)))
	f.Add("V2.4", uint32(0))
	f.Add("V94", uint32(rand.Intn(100)))
	f.Add("VW31", uint32(rand.Intn(100)))
	f.Fuzz(writeReadTest)
}

func writeReadTest(t *testing.T, vmAddr string, value uint32) {
	addr, err := gos7logo.NewVmAddrFromString(vmAddr)
	if err != nil {
		t.Errorf("no correct vm address `%s`: %s", vmAddr, err)
	}
	if err := client.Write(addr, value); err != nil {
		t.Errorf("failed write from %s: %s", vmAddr, err)
	}
	v, err := client.Read(addr)
	if err != nil {
		t.Errorf("failed read from %s: %s", vmAddr, err)
	}

	if addr.Type == gos7logo.Bit {
		value &^= 1 << 0
	}

	if value != v {
		t.Errorf("write and read values not equals for %s : %s != %s", vmAddr, strconv.Itoa(int(value)), strconv.Itoa(int(v)))
	}
}

package test

import (
	"fmt"
	gos7logo "gos7-logo"
	"math"
	"math/rand"
	"os"
	"testing"
)

var client gos7logo.Client

func TestMain(m *testing.M) {
	cl, err := gos7logo.NewClient(&gos7logo.ConnectOpt{
		Addr: "localhost:102",
		Rack: 0, Slot: 1,
	})
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
	f.Add("VD3", rand.Intn(math.MaxInt32))
	f.Add("V2.1", rand.Intn(2))
	f.Add("V94", rand.Intn(math.MaxInt8))
	f.Add("VW31", rand.Intn(math.MaxInt16))
	f.Fuzz(writeReadTest)
}

func writeReadTest(t *testing.T, vmAddr string, value int) {
	if err := client.Write(vmAddr, value); err != nil {
		t.Errorf("failed write from %s: %s", vmAddr, err)
	}
	v, err := client.Read(vmAddr)
	if err != nil {
		t.Errorf("failed read from %s: %s", vmAddr, err)
	}

	if value != v {
		t.Errorf("write and reade values not equals: %s", vmAddr)
	}
}

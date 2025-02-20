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

func TestClientWriteManyRead(t *testing.T) {
	vdVmAddr, err := gos7logo.NewVmAddrFromString("VD3")
	if err != nil {
		t.Fatal(err)
	}
	vwVmAddr, err := gos7logo.NewVmAddrFromString("VW31")
	if err != nil {
		t.Fatal(err)
	}
	v1VmAddr, err := gos7logo.NewVmAddrFromString("V2.4")
	if err != nil {
		t.Fatal(err)
	}
	v2VmAddr, err := gos7logo.NewVmAddrFromString("V94")
	if err != nil {
		t.Fatal(err)
	}

	vmAddrVals := []gos7logo.VmAddrValue{
		{VmAddr: vdVmAddr, Value: uint32(rand.Intn(100))},
		{VmAddr: v1VmAddr, Value: uint32(0)},
		{VmAddr: v2VmAddr, Value: uint32(rand.Intn(100))},
		{VmAddr: vwVmAddr, Value: uint32(rand.Intn(100))},
	}

	if err := client.WriteMany(vmAddrVals...); err != nil {
		t.Fatal(err)
	}

	for _, val := range vmAddrVals {
		v, err := client.Read(val.VmAddr)
		if err != nil {
			t.Errorf("failed read: %s", err)
		}

		if val.VmAddr.Type == gos7logo.Bit {
			expectedBit := (val.Value >> uint32(val.VmAddr.Bit)) & 1
			if expectedBit != v {
				t.Errorf("write and read values not equals for bit: expected %d, got %d", expectedBit, v)
			}
			continue
		}

		if val.Value != v {
			t.Errorf("write and read values not equals: %s != %s", strconv.Itoa(int(val.Value)), strconv.Itoa(int(v)))
		}
	}
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
		expectedBit := (0 >> uint32(addr.Bit)) & 1
		if expectedBit != 0 {
			t.Errorf("write and read values not equals for bit: expected %d, got %d", expectedBit, v)
		}
		return
	}

	if value != v {
		t.Errorf("write and read values not equals for %s : %s != %s", vmAddr, strconv.Itoa(int(value)), strconv.Itoa(int(v)))
	}
}

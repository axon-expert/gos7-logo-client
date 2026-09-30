package test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"

	gos7logo "github.com/axon-expert/gos7-logo-client/logo"
	"github.com/stretchr/testify/require"
)

var client gos7logo.Client // nolint:gochecknoglobals
var errConnection error    // nolint:gochecknoglobals

func TestMain(m *testing.M) {
	// TODO: launch snap7 server
	cl := gos7logo.NewClient(gos7logo.NewConfig("localhost"))
	errConnection = cl.Connect(context.Background())
	if errConnection != nil {
		fmt.Printf("controller tests will be skipped: %s\n", errConnection)
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
	if errConnection != nil {
		t.Skip(errConnection)
	}
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
				t.Errorf("write and read values not equals for bit: expected %d, got %d",
					expectedBit, v)
			}
			continue
		}

		if val.Value != v {
			t.Errorf("write and read values not equals: %s != %s",
				strconv.Itoa(int(val.Value)), strconv.Itoa(int(v)))
		}
	}
}

func vmAddr(s string) gos7logo.VmAddr {
	return gos7logo.MustNewVmAddrFromString(s)
}

func TestClientReadMany(t *testing.T) {
	if errConnection != nil {
		t.Skip(errConnection)
	}
	addr1 := vmAddr("V3")
	addr2 := vmAddr("V4.1")
	addr3 := vmAddr("V4.2")

	if err := client.WriteMany(
		gos7logo.VmAddrValue{VmAddr: addr1, Value: 123},
		gos7logo.VmAddrValue{VmAddr: addr2, Value: 1},
		gos7logo.VmAddrValue{VmAddr: addr3, Value: 0},
	); err != nil {
		t.Fatal(err)
	}

	res, err := client.ReadMany(addr1, addr2, addr3)
	require.NoError(t, err)
	require.Len(t, res, 3)
	require.Equal(t, gos7logo.VmAddrValue{VmAddr: addr1, Value: 123}, res[0])
	require.Equal(t, gos7logo.VmAddrValue{VmAddr: addr2, Value: 1}, res[1])
	require.Equal(t, gos7logo.VmAddrValue{VmAddr: addr3, Value: 0}, res[2])
}

func writeReadTest(t *testing.T, vmAddr string, value uint32) {
	if errConnection != nil {
		t.Skip(errConnection)
	}
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
			t.Errorf("write and read values not equals for bit: expected %d, got %d",
				expectedBit, v)
		}
		return
	}

	if value != v {
		t.Errorf("write and read values not equals for %s : %s != %s",
			vmAddr, strconv.Itoa(int(value)), strconv.Itoa(int(v)))
	}
}

type addrs struct {
	Bit   gos7logo.VmAddr
	Byte  gos7logo.VmAddr
	Word  gos7logo.VmAddr
	DWord gos7logo.VmAddr
}

func TestMarshaling(t *testing.T) {
	as := addrs{
		Bit:   gos7logo.MustNewVmAddr(gos7logo.Bit, 1, 2),
		Byte:  gos7logo.MustNewVmAddr(gos7logo.Byte, 3, 0),
		Word:  gos7logo.MustNewVmAddr(gos7logo.Word, 4, 0),
		DWord: gos7logo.MustNewVmAddr(gos7logo.DWord, 5, 0),
	}
	raw, err := json.Marshal(as)
	if err != nil {
		t.Errorf("fail to marshal addrs: %s", err.Error())
	}
	expected := `{"Bit":"V1.2","Byte":"V3","Word":"VW4","DWord":"VD5"}`
	if string(raw) != expected {
		t.Errorf("expect:\n:%s\ngot:\n%s", expected, string(raw))
	}

	as1 := addrs{}
	err = json.Unmarshal(raw, &as1)
	require.NoError(t, err, "fail to unmarshal addrs")
	if as1 != as {
		t.Errorf("expected:\n%#v\ngot:\n%v", as, as1)
	}
}

func TestUnmarshalingMalformedVmAddr(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid address", raw: `{"Bit":"invalid"}`},
		{name: "invalid JSON", raw: `{"Bit":"V1.2"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var as addrs
			err := json.Unmarshal([]byte(tt.raw), &as)
			require.Error(t, err)
		})
	}
}

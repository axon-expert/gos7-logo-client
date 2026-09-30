package gos7logo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewConfig(t *testing.T) {
	require.Equal(t, Config{
		Host:       "192.0.2.1",
		Port:       102,
		LocalTSAP:  0x1000,
		RemoteTSAP: 0x2000,
	}, NewConfig("192.0.2.1"))
}

func TestNewClientJoinsIPv6HostAndPort(t *testing.T) {
	config := NewConfig("2001:db8::1")
	config.Port = 1102

	client := NewClient(config)

	require.Equal(t, "[2001:db8::1]:1102", client.handler.Address)
}

func TestTSAPUnmarshalText(t *testing.T) {
	tests := []struct {
		input string
		want  TSAP
	}{
		{input: "0x2000", want: 0x2000},
		{input: "20.00", want: 0x2000},
		{input: "8192", want: 0x2000},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			var tsap TSAP
			require.NoError(t, tsap.UnmarshalText([]byte(test.input)))
			require.Equal(t, test.want, tsap)
		})
	}
}

func TestTSAPUnmarshalJSON(t *testing.T) {
	var tsap TSAP
	require.NoError(t, json.Unmarshal([]byte(`"20.00"`), &tsap))
	require.Equal(t, TSAP(0x2000), tsap)
	require.NoError(t, json.Unmarshal([]byte(`8196`), &tsap))
	require.Equal(t, TSAP(0x2004), tsap)
}

func TestTSAPRejectsMalformedText(t *testing.T) {
	for _, input := range []string{"", "20.", "20.0", "200.00", "gg.00", "65536"} {
		t.Run(input, func(t *testing.T) {
			var tsap TSAP
			require.Error(t, tsap.UnmarshalText([]byte(input)))
		})
	}
}

func TestConnectHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(NewConfig("192.0.2.1"))
	err := client.Connect(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func TestOperationsRequireConnection(t *testing.T) {
	client := NewClient(NewConfig("192.0.2.1"))
	addr := MustNewVmAddrFromString("V1")
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "read", run: func() error { _, err := client.Read(addr); return err }},
		{name: "read many", run: func() error { _, err := client.ReadMany(addr); return err }},
		{
			name: "read many to",
			run:  func() error { return client.ReadManyTo(make([]byte, 1), addr) },
		},
		{name: "write", run: func() error { return client.Write(addr, 1) }},
		{name: "write many", run: func() error {
			return client.WriteMany(VmAddrValue{VmAddr: addr, Value: 1})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.run(), ErrNotConnected)
		})
	}
}

func TestVmAddrConstructors(t *testing.T) {
	addr, err := NewVmAddr(Bit, 12, 7)
	require.NoError(t, err)
	require.Equal(t, VmAddr{Type: Bit, Byte: 12, Bit: 7}, addr)

	_, err = NewVmAddr(Bit, 12, 8)
	require.Error(t, err)
	_, err = NewVmAddr(Word, 12, 1)
	require.Error(t, err)
	_, err = NewVmAddr(DataType(100), 12, 0)
	require.Error(t, err)

	require.Panics(t, func() {
		MustNewVmAddr(Bit, 12, 8)
	})
	require.Panics(t, func() {
		MustNewVmAddrFromString("V1.8")
	})
}

func TestVmAddrUnmarshalRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{
		"prefixV12",
		"V1.8",
		"V1.2.3",
		"V4294967296",
	} {
		t.Run(input, func(t *testing.T) {
			var addr VmAddr
			require.Error(t, addr.UnmarshalText([]byte(input)))
		})
	}
}

func TestVmAddrRangeUsesAddressEnd(t *testing.T) {
	start, size, err := vmAddrRange([]VmAddr{
		MustNewVmAddr(Byte, 12, 0),
		MustNewVmAddr(DWord, 10, 0),
		MustNewVmAddr(Word, 2, 0),
	})
	require.NoError(t, err)
	require.Equal(t, uint32(2), start)
	require.Equal(t, 12, size)
}

func TestVmAddrComparators(t *testing.T) {
	require.Negative(t, compareVmAddrByte(VmAddr{Byte: 1}, VmAddr{Byte: 2}))
	require.Zero(t, compareVmAddrByte(VmAddr{Byte: 2}, VmAddr{Byte: 2}))
	require.Positive(t, compareVmAddrByte(VmAddr{Byte: 2}, VmAddr{Byte: 1}))
}

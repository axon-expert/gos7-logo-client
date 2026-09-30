package gos7logo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

func TestWriteRejectsOutputAddress(t *testing.T) {
	client := NewClient(NewConfig("192.0.2.1"))
	client.connected.Store(true)
	addr := MustNewVmAddrFromString("Q1")

	err := client.Write(addr, 1)
	require.ErrorIs(t, err, ErrReadOnlyAddress)
	require.EqualError(t, err, "write Q1: address is read-only")

	err = client.WriteMany(VmAddrValue{VmAddr: addr, Value: 1})
	require.ErrorIs(t, err, ErrReadOnlyAddress)
	require.EqualError(t, err, "WriteMany: Q1: address is read-only")
}

func TestStreamValidatesArguments(t *testing.T) {
	client := NewClient(NewConfig("192.0.2.1"))
	addr := MustNewVmAddrFromString("V1")

	_, err := client.Stream(context.Background(), 0, addr)
	require.EqualError(t, err, "stream interval must be greater than zero")
	_, err = client.Stream(context.Background(), time.Second, addr)
	require.ErrorIs(t, err, ErrNotConnected)

	client.connected.Store(true)
	_, err = client.Stream(context.Background(), time.Second)
	require.EqualError(t, err, "stream addresses are empty")
}

func TestStreamClosesWhenContextIsCancelled(t *testing.T) {
	client := NewClient(NewConfig("192.0.2.1"))
	client.connected.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := client.Stream(ctx, time.Second, MustNewVmAddrFromString("V1"))
	require.NoError(t, err)
	_, open := <-results
	require.False(t, open)
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
		"Q0",
		"Q65",
		"Q1.0",
	} {
		t.Run(input, func(t *testing.T) {
			var addr VmAddr
			require.Error(t, addr.UnmarshalText([]byte(input)))
		})
	}
}

func TestOutputAddresses(t *testing.T) {
	tests := []struct {
		input string
		want  VmAddr
	}{
		{input: "Q1", want: VmAddr{Type: Output, Byte: 1064, Bit: 0}},
		{input: "Q8", want: VmAddr{Type: Output, Byte: 1064, Bit: 7}},
		{input: "Q9", want: VmAddr{Type: Output, Byte: 1065, Bit: 0}},
		{input: "Q64", want: VmAddr{Type: Output, Byte: 1071, Bit: 7}},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			addr, err := NewVmAddrFromString(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, addr)
			require.Equal(t, test.input, addr.String())
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

func TestVmAddrValuesAccessors(t *testing.T) {
	addr1 := MustNewVmAddrFromString("V1")
	addr2 := MustNewVmAddrFromString("VW2")
	values := VmAddrValues{
		{VmAddr: addr1, Value: 10},
		{VmAddr: addr2, Value: 20},
	}

	value, ok := values.At(1)
	require.True(t, ok)
	require.Equal(t, VmAddrValue{VmAddr: addr2, Value: 20}, value)
	_, ok = values.At(-1)
	require.False(t, ok)
	_, ok = values.At(len(values))
	require.False(t, ok)

	actual, ok := values.Get(addr1)
	require.True(t, ok)
	require.Equal(t, uint32(10), actual)
	_, ok = values.Get(MustNewVmAddrFromString("V3"))
	require.False(t, ok)
}

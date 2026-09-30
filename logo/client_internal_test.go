package gos7logo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const testHost = "192.0.2.1"

func TestNewConfig(t *testing.T) {
	require.Equal(t, Config{
		Host:       testHost,
		Port:       102,
		LocalTSAP:  0x1000,
		RemoteTSAP: 0x2000,
	}, NewConfig(testHost))
}

func TestConfigUnmarshalJSON(t *testing.T) {
	var config Config
	err := json.Unmarshal([]byte(`{
		"host": "192.0.2.1",
		"port": 1102,
		"local_tsap": "10.00",
		"remote_tsap": 8192
	}`), &config)

	require.NoError(t, err)
	require.Equal(t, Config{
		Host:       testHost,
		Port:       1102,
		LocalTSAP:  0x1000,
		RemoteTSAP: 0x2000,
	}, config)
}

func TestConfigUnmarshalYAML(t *testing.T) {
	var config Config
	err := yaml.Unmarshal([]byte(`
host: 192.0.2.1
port: 1102
local_tsap: "10.00"
remote_tsap: 0x2000
`), &config)

	require.NoError(t, err)
	require.Equal(t, Config{
		Host:       testHost,
		Port:       1102,
		LocalTSAP:  0x1000,
		RemoteTSAP: 0x2000,
	}, config)
}

func TestConfigReadEnvironment(t *testing.T) {
	t.Setenv("HOST", testHost)
	t.Setenv("PORT", "1102")
	t.Setenv("LOCAL_TSAP", "10.00")
	t.Setenv("REMOTE_TSAP", "0x2000")
	var config Config

	err := cleanenv.ReadEnv(&config)

	require.NoError(t, err)
	require.Equal(t, Config{
		Host:       testHost,
		Port:       1102,
		LocalTSAP:  0x1000,
		RemoteTSAP: 0x2000,
	}, config)
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

	client := NewClient(NewConfig(testHost))
	err := client.Connect(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func TestOperationsRequireConnection(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	addr := MustNewVMAddrFromString("V1")
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
			return client.WriteMany(VMAddrValue{VMAddr: addr, Value: 1})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.run(), ErrNotConnected)
		})
	}
}

func TestWriteRejectsOutputAddress(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	client.connected.Store(true)
	addr := MustNewVMAddrFromString("Q1")

	err := client.Write(addr, 1)
	require.ErrorIs(t, err, ErrReadOnlyAddress)
	require.EqualError(t, err, "write Q1: address is read-only")

	err = client.WriteMany(VMAddrValue{VMAddr: addr, Value: 1})
	require.ErrorIs(t, err, ErrReadOnlyAddress)
	require.EqualError(t, err, "WriteMany: Q1: address is read-only")
}

func TestStreamValidatesArguments(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	addr := MustNewVMAddrFromString("V1")

	_, err := client.Stream(context.Background(), 0, addr)
	require.EqualError(t, err, "stream interval must be greater than zero")
	_, err = client.Stream(context.Background(), time.Second, addr)
	require.ErrorIs(t, err, ErrNotConnected)

	client.connected.Store(true)
	_, err = client.Stream(context.Background(), time.Second)
	require.EqualError(t, err, "stream addresses are empty")
}

func TestStreamClosesWhenContextIsCancelled(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	client.connected.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := client.Stream(ctx, time.Second, MustNewVMAddrFromString("V1"))
	require.NoError(t, err)
	_, open := <-results
	require.False(t, open)
}

func TestVMAddrConstructors(t *testing.T) {
	addr, err := NewVMAddr(Bit, 12, 7)
	require.NoError(t, err)
	require.Equal(t, VMAddr{Type: Bit, Byte: 12, Bit: 7}, addr)

	_, err = NewVMAddr(Bit, 12, 8)
	require.Error(t, err)
	_, err = NewVMAddr(Word, 12, 1)
	require.Error(t, err)
	_, err = NewVMAddr(DataType(100), 12, 0)
	require.Error(t, err)

	require.Panics(t, func() {
		MustNewVMAddr(Bit, 12, 8)
	})
	require.Panics(t, func() {
		MustNewVMAddrFromString("V1.8")
	})
}

func TestVMAddrUnmarshalRejectsMalformedInput(t *testing.T) {
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
			var addr VMAddr
			require.Error(t, addr.UnmarshalText([]byte(input)))
		})
	}
}

func TestOutputAddresses(t *testing.T) {
	tests := []struct {
		input string
		want  VMAddr
	}{
		{input: "Q1", want: VMAddr{Type: Output, Byte: 1064, Bit: 0}},
		{input: "Q8", want: VMAddr{Type: Output, Byte: 1064, Bit: 7}},
		{input: "Q9", want: VMAddr{Type: Output, Byte: 1065, Bit: 0}},
		{input: "Q64", want: VMAddr{Type: Output, Byte: 1071, Bit: 7}},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			addr, err := NewVMAddrFromString(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, addr)
			require.Equal(t, test.input, addr.String())
		})
	}
}

func TestVMAddrRangeUsesAddressEnd(t *testing.T) {
	start, size, err := vmAddrRange([]VMAddr{
		MustNewVMAddr(Byte, 12, 0),
		MustNewVMAddr(DWord, 10, 0),
		MustNewVMAddr(Word, 2, 0),
	})
	require.NoError(t, err)
	require.Equal(t, uint32(2), start)
	require.Equal(t, 12, size)
}

func TestVMAddrComparators(t *testing.T) {
	require.Negative(t, compareVMAddrByte(VMAddr{Byte: 1}, VMAddr{Byte: 2}))
	require.Zero(t, compareVMAddrByte(VMAddr{Byte: 2}, VMAddr{Byte: 2}))
	require.Positive(t, compareVMAddrByte(VMAddr{Byte: 2}, VMAddr{Byte: 1}))
}

func TestVMAddrValuesAccessors(t *testing.T) {
	addr1 := MustNewVMAddrFromString("V1")
	addr2 := MustNewVMAddrFromString("VW2")
	values := VMAddrValues{
		{VMAddr: addr1, Value: 10},
		{VMAddr: addr2, Value: 20},
	}

	value, ok := values.At(1)
	require.True(t, ok)
	require.Equal(t, VMAddrValue{VMAddr: addr2, Value: 20}, value)
	_, ok = values.At(-1)
	require.False(t, ok)
	_, ok = values.At(len(values))
	require.False(t, ok)

	actual, ok := values.Get(addr1)
	require.True(t, ok)
	require.Equal(t, uint32(10), actual)
	_, ok = values.Get(MustNewVMAddrFromString("V3"))
	require.False(t, ok)
}

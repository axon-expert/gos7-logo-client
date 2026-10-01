package gos7logo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	gos7patch "github.com/axon-expert/gos7-logo-client/gos7-patch"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const testHost = "192.0.2.1"

func TestNewConfig(t *testing.T) {
	require.Equal(t, Config{
		Host:                  testHost,
		Port:                  102,
		LocalTSAP:             0x1000,
		RemoteTSAP:            0x2000,
		InitialReconnectDelay: defaultInitialReconnectDelay,
		MaxReconnectDelay:     defaultMaxReconnectDelay,
	}, NewConfig(testHost))
}

func TestConfigUnmarshalJSON(t *testing.T) {
	var config Config
	err := json.Unmarshal([]byte(`{
		"host": "192.0.2.1",
		"port": 1102,
		"local_tsap": "10.00",
		"remote_tsap": 8192,
		"reconnect": true,
		"initial_reconnect_delay": 250000000,
		"max_reconnect_delay": 30000000000
	}`), &config)

	require.NoError(t, err)
	require.Equal(t, Config{
		Host:                  testHost,
		Port:                  1102,
		LocalTSAP:             0x1000,
		RemoteTSAP:            0x2000,
		Reconnect:             true,
		InitialReconnectDelay: 250 * time.Millisecond,
		MaxReconnectDelay:     30 * time.Second,
	}, config)
}

func TestConfigUnmarshalYAML(t *testing.T) {
	var config Config
	err := yaml.Unmarshal([]byte(`
host: 192.0.2.1
port: 1102
local_tsap: "10.00"
remote_tsap: 0x2000
reconnect: true
initial_reconnect_delay: 250ms
max_reconnect_delay: 30s
`), &config)

	require.NoError(t, err)
	require.Equal(t, Config{
		Host:                  testHost,
		Port:                  1102,
		LocalTSAP:             0x1000,
		RemoteTSAP:            0x2000,
		Reconnect:             true,
		InitialReconnectDelay: 250 * time.Millisecond,
		MaxReconnectDelay:     30 * time.Second,
	}, config)
}

func TestConfigReadEnvironment(t *testing.T) {
	t.Setenv("HOST", testHost)
	t.Setenv("PORT", "1102")
	t.Setenv("LOCAL_TSAP", "10.00")
	t.Setenv("REMOTE_TSAP", "0x2000")
	t.Setenv("RECONNECT", "true")
	t.Setenv("INITIAL_RECONNECT_DELAY", "250ms")
	t.Setenv("MAX_RECONNECT_DELAY", "30s")
	var config Config

	err := cleanenv.ReadEnv(&config)

	require.NoError(t, err)
	require.Equal(t, Config{
		Host:                  testHost,
		Port:                  1102,
		LocalTSAP:             0x1000,
		RemoteTSAP:            0x2000,
		Reconnect:             true,
		InitialReconnectDelay: 250 * time.Millisecond,
		MaxReconnectDelay:     30 * time.Second,
	}, config)
}

func TestNewClientJoinsIPv6HostAndPort(t *testing.T) {
	config := NewConfig("2001:db8::1")
	config.Port = 1102

	client := NewClient(config)

	require.Equal(t, "[2001:db8::1]:1102", client.handler.Address)
	require.Equal(t, connectionInitial, client.connectionState)
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

func TestLazyConnectHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(NewConfig(testHost))
	_, err := client.Read(ctx, MustNewVMAddrFromString("V1"))

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, connectionInitial, client.connectionState)
}

func TestCancelledInitialConnectCanBeRetried(t *testing.T) {
	config := NewConfig("127.0.0.1")
	config.Port = 0
	client := NewClient(config)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.connect(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, connectionInitial, client.connectionState)

	err = client.connect(context.Background())
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotConnected)
	require.Equal(t, connectionLost, client.connectionState)
}

func TestCancelledReconnectPreservesBackoff(t *testing.T) {
	config := NewConfig(testHost)
	config.Reconnect = true
	client := NewClient(config)
	client.connectionState = connectionLost
	client.reconnectDelay = time.Second
	client.reconnectAt = time.Now().Add(time.Hour)
	reconnectAt := client.reconnectAt
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.connect(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, connectionLost, client.connectionState)
	require.Equal(t, time.Second, client.reconnectDelay)
	require.Equal(t, reconnectAt, client.reconnectAt)
}

func TestOperationsRejectClosedClient(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	require.NoError(t, client.Close())
	require.Equal(t, connectionClosed, client.connectionState)
	addr := MustNewVMAddrFromString("V1")
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "read", run: func() error {
			_, err := client.Read(context.Background(), addr)
			return err
		}},
		{name: "read many", run: func() error {
			_, err := client.ReadMany(context.Background(), addr)
			return err
		}},
		{
			name: "read many to",
			run: func() error {
				return client.ReadManyTo(context.Background(), make([]byte, 1), addr)
			},
		},
		{name: "write", run: func() error {
			return client.Write(context.Background(), addr, 1)
		}},
		{name: "write many", run: func() error {
			return client.WriteMany(
				context.Background(),
				VMAddrValue{VMAddr: addr, Value: 1},
			)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.run(), ErrClientClosed)
		})
	}
}

func TestWriteRejectsOutputAddress(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	addr := MustNewVMAddrFromString("Q1")

	err := client.Write(context.Background(), addr, 1)
	require.ErrorIs(t, err, ErrReadOnlyAddress)
	require.EqualError(t, err, "write Q1: address is read-only")

	err = client.WriteMany(context.Background(), VMAddrValue{VMAddr: addr, Value: 1})
	require.ErrorIs(t, err, ErrReadOnlyAddress)
	require.EqualError(t, err, "WriteMany: Q1: address is read-only")
}

func TestStreamValidatesArguments(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	addr := MustNewVMAddrFromString("V1")

	_, err := client.Stream(context.Background(), 0, addr)
	require.EqualError(t, err, "stream interval must be greater than zero")
	_, err = client.Stream(context.Background(), time.Second)
	require.EqualError(t, err, "stream addresses are empty")
}

func TestStreamClosesWhenContextIsCancelled(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := client.Stream(ctx, time.Second, MustNewVMAddrFromString("V1"))
	require.NoError(t, err)
	_, open := <-results
	require.False(t, open)
}

func TestStreamClosesWithClient(t *testing.T) {
	config := NewConfig(testHost)
	config.Reconnect = true
	client := NewClient(config)
	results, err := client.Stream(
		context.Background(),
		time.Millisecond,
		MustNewVMAddrFromString("V1"),
	)
	require.NoError(t, err)

	require.NoError(t, client.Close())
	for {
		select {
		case _, open := <-results:
			if !open {
				return
			}
		case <-time.After(time.Second):
			t.Fatal("stream did not close with client")
		}
	}
}

func TestStreamReportsReadErrorBeforeReconnectError(t *testing.T) {
	config := NewConfig("127.0.0.1")
	config.Port = 0
	config.Reconnect = true
	client := NewClient(config)
	client.connectionState = connectionConnected
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results, err := client.Stream(ctx, time.Millisecond, MustNewVMAddrFromString("V1"))
	require.NoError(t, err)
	readResult := <-results
	require.Error(t, readResult.Err)
	reconnectResult := <-results
	require.ErrorContains(t, reconnectResult.Err, "reconnect:")
}

func TestReconnectBackoffIsExponential(t *testing.T) {
	config := NewConfig(testHost)
	config.Reconnect = true
	config.InitialReconnectDelay = 10 * time.Millisecond
	config.MaxReconnectDelay = 40 * time.Millisecond
	client := NewClient(config)

	firstStart := time.Now()
	firstDelay := client.connectionFailed()
	require.Equal(t, config.InitialReconnectDelay, firstDelay)
	require.Equal(t, 2*config.InitialReconnectDelay, client.reconnectDelay)
	require.WithinDuration(
		t,
		firstStart.Add(config.InitialReconnectDelay),
		client.reconnectAt,
		time.Second/10,
	)

	secondStart := time.Now()
	secondDelay := client.connectionFailed()
	require.Equal(t, 2*config.InitialReconnectDelay, secondDelay)
	require.Equal(t, 4*config.InitialReconnectDelay, client.reconnectDelay)
	require.WithinDuration(
		t,
		secondStart.Add(2*config.InitialReconnectDelay),
		client.reconnectAt,
		time.Second/10,
	)

	for range 20 {
		client.connectionFailed()
	}
	require.Equal(t, config.MaxReconnectDelay, client.reconnectDelay)
}

func TestDisabledReconnectDoesNotReconnectLostConnection(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	client.connectionState = connectionLost

	err := client.ensureConnected(context.Background())

	require.ErrorIs(t, err, ErrNotConnected)
}

func TestOperationFailureKeepsConnectionForPLCError(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	client.connectionState = connectionConnected
	plcErr := &gos7patch.PLCError{Code: gos7patch.CPUError(5)}

	err := client.operationFailed(plcErr)

	require.Same(t, plcErr, err)
	require.Equal(t, connectionConnected, client.connectionState)
}

func TestOperationFailureClosesBrokenConnection(t *testing.T) {
	config := NewConfig(testHost)
	config.Reconnect = true
	client := NewClient(config)
	client.connectionState = connectionConnected
	transportErr := errors.New("transport failed")

	err := client.operationFailed(transportErr)

	require.ErrorIs(t, err, transportErr)
	require.EqualError(t, err, "transport failed; reconnect in 250ms")
	require.Equal(t, connectionLost, client.connectionState)
}

func TestOperationWaitHonorsContext(t *testing.T) {
	client := NewClient(NewConfig(testHost))
	<-client.operationGate
	defer client.release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Read(ctx, MustNewVMAddrFromString("V1"))

	require.ErrorIs(t, err, context.Canceled)
}

func TestCloseCancelsReconnectBackoff(t *testing.T) {
	config := NewConfig(testHost)
	config.Reconnect = true
	client := NewClient(config)
	client.connectionState = connectionLost
	client.reconnectAt = time.Now().Add(time.Hour)
	errResult := make(chan error, 1)

	go func() {
		_, err := client.Read(context.Background(), MustNewVMAddrFromString("V1"))
		errResult <- err
	}()
	require.Eventually(t, func() bool {
		return len(client.operationGate) == 0
	}, time.Second, time.Millisecond)

	require.NoError(t, client.Close())
	require.ErrorIs(t, <-errResult, context.Canceled)
	require.Equal(t, connectionClosed, client.connectionState)
	require.NoError(t, client.Close())
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

func TestVMAddrRespectsLOGOVariableMemoryLimit(t *testing.T) {
	for _, input := range []string{"V850", "V850.7", "VW849", "VD847"} {
		t.Run("accept "+input, func(t *testing.T) {
			_, err := NewVMAddrFromString(input)
			require.NoError(t, err)
		})
	}
	for _, input := range []string{"V851", "V851.0", "VW850", "VD848"} {
		t.Run("reject "+input, func(t *testing.T) {
			_, err := NewVMAddrFromString(input)
			require.ErrorContains(t, err, "ends after V850")
		})
	}
}

func TestVMAddrMarshalRejectsInvalidAddress(t *testing.T) {
	tests := []struct {
		name string
		addr VMAddr
	}{
		{name: "unknown type", addr: VMAddr{Type: DataType(255)}},
		{name: "invalid bit", addr: VMAddr{Type: Bit, Bit: 8}},
		{name: "invalid output", addr: VMAddr{Type: Output, Byte: 1}},
		{name: "outside variable memory", addr: VMAddr{Type: Byte, Byte: 851}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.addr.MarshalText()
			require.Error(t, err)
			_, err = json.Marshal(test.addr)
			require.Error(t, err)
		})
	}
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
	require.Equal(t, uint16(2), start)
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

package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"

	gos7logo "github.com/axon-expert/gos7-logo-client/logo"
	"github.com/stretchr/testify/require"
)

const (
	commandRead  = "read"
	commandWrite = "write"
	commandWatch = "watch"
	addressVW4   = "VW4"
	optionRetry  = "-retry"
)

type fakeClient struct {
	values        map[gos7logo.VMAddr]uint32
	closed        bool
	streamErr     error
	readFailures  int
	writeFailures int
	readCalls     int
	writeCalls    int
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	output := new(bytes.Buffer)
	previous := logger
	logger = slog.New(slog.NewTextHandler(output, nil))
	t.Cleanup(func() { logger = previous })
	return output
}

func (c *fakeClient) Read(_ context.Context, addr gos7logo.VMAddr) (uint32, error) {
	c.readCalls++
	if c.readFailures > 0 {
		c.readFailures--
		return 0, errors.New("connection lost")
	}
	return c.values[addr], nil
}

func (c *fakeClient) Stream(
	ctx context.Context,
	_ time.Duration,
	addresses ...gos7logo.VMAddr,
) (<-chan gos7logo.StreamResult, error) {
	results := make(chan gos7logo.StreamResult, 2)
	defer close(results)
	select {
	case <-ctx.Done():
		return results, nil
	default:
	}
	if c.streamErr != nil {
		results <- gos7logo.StreamResult{Err: c.streamErr}
	}
	values := make(gos7logo.VMAddrValues, len(addresses))
	for i, addr := range addresses {
		values[i] = gos7logo.VMAddrValue{VMAddr: addr, Value: c.values[addr]}
	}
	results <- gos7logo.StreamResult{Data: values}
	return results, nil
}

func (c *fakeClient) Write(_ context.Context, addr gos7logo.VMAddr, value uint32) error {
	c.writeCalls++
	if c.writeFailures > 0 {
		c.writeFailures--
		return errors.New("connection lost")
	}
	c.values[addr] = value
	return nil
}

func (c *fakeClient) Close() error {
	c.closed = true
	return nil
}

func TestRunRead(t *testing.T) {
	addr := gos7logo.MustNewVMAddrFromString(addressVW4)
	client := &fakeClient{values: map[gos7logo.VMAddr]uint32{addr: 42}}
	logs := captureLogs(t)

	err := run(context.Background(), []string{commandRead, addressVW4}, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Contains(t, logs.String(), "VW4=42")
	require.True(t, client.closed)
}

func TestRunReadRange(t *testing.T) {
	client := &fakeClient{values: map[gos7logo.VMAddr]uint32{
		gos7logo.MustNewVMAddrFromString("V3"): 3,
		gos7logo.MustNewVMAddrFromString("V4"): 4,
		gos7logo.MustNewVMAddrFromString("V5"): 5,
	}}
	logs := captureLogs(t)

	err := run(context.Background(), []string{commandRead, "V3-V5"}, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Contains(t, logs.String(), "V3=3")
	require.Contains(t, logs.String(), "V4=4")
	require.Contains(t, logs.String(), "V5=5")
}

func TestParseAddressRange(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "V3-V5", want: []string{"V3", "V4", "V5"}},
		{input: "VW3-VW5", want: []string{"VW3", "VW4", "VW5"}},
		{input: "VD3-VD4", want: []string{"VD3", "VD4"}},
		{input: "V3.6-V4.2", want: []string{"V3.6", "V3.7", "V4.0", "V4.1", "V4.2"}},
		{input: "Q7-Q10", want: []string{"Q7", "Q8", "Q9", "Q10"}},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			addresses, err := parseAddressOrRange(test.input)
			require.NoError(t, err)
			actual := make([]string, len(addresses))
			for i, addr := range addresses {
				actual[i] = addr.String()
			}
			require.Equal(t, test.want, actual)
		})
	}
}

func TestParseAddressRangeRejectsInvalidEndpoints(t *testing.T) {
	for _, input := range []string{"V5-V3", "V3-VW5", "V3-", "V3-V4-V5"} {
		t.Run(input, func(t *testing.T) {
			_, err := parseAddressOrRange(input)
			require.Error(t, err)
		})
	}
}

func TestRunReadFormatsValue(t *testing.T) {
	addr := gos7logo.MustNewVMAddrFromString(addressVW4)
	tests := []struct {
		format string
		want   string
	}{
		{format: "d", want: "42"},
		{format: "x", want: "002a"},
		{format: "h", want: "002a"},
		{format: "b", want: "0000000000101010"},
		{format: "o", want: "000052"},
		{format: "d_", want: "42"},
		{format: "x_", want: "00_2a"},
		{format: "h_", want: "00_2a"},
		{format: "b_", want: "0000_0000_0010_1010"},
		{format: "o_", want: "000_052"},
	}
	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			client := &fakeClient{values: map[gos7logo.VMAddr]uint32{addr: 42}}
			logs := captureLogs(t)

			err := run(context.Background(), []string{"-f", test.format, commandRead, addressVW4},
				&bytes.Buffer{}, func(gos7logo.Config) logoClient { return client })

			require.NoError(t, err)
			require.Contains(t, logs.String(), "VW4="+test.want)
		})
	}
}

func TestFormatValueGroupsDecimalFromRight(t *testing.T) {
	addr := gos7logo.MustNewVMAddrFromString("VD4")
	require.Equal(t, "123_456", formatValue(addr, 123456, "d_"))
}

func TestRunWrite(t *testing.T) {
	client := &fakeClient{values: make(map[gos7logo.VMAddr]uint32)}
	logs := captureLogs(t)

	err := run(context.Background(), []string{commandWrite, "V3", "0xff", "V4.2", "1"},
		&bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Equal(t, uint32(255), client.values[gos7logo.MustNewVMAddrFromString("V3")])
	require.Equal(t, uint32(1), client.values[gos7logo.MustNewVMAddrFromString("V4.2")])
	require.Contains(t, logs.String(), "V3=255")
	require.Contains(t, logs.String(), "V4.2=1")
}

func TestRunReadRetriesWithReconnect(t *testing.T) {
	addr := gos7logo.MustNewVMAddrFromString("V1")
	client := &fakeClient{
		values:       map[gos7logo.VMAddr]uint32{addr: 7},
		readFailures: 1,
	}
	logs := captureLogs(t)

	err := run(context.Background(), []string{optionRetry, commandRead, "V1"},
		&bytes.Buffer{}, func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Equal(t, 2, client.readCalls)
	require.Contains(t, logs.String(), "V1=7")
	require.Contains(t, logs.String(), "operation failed; retrying")
	require.Contains(t, logs.String(), "error=\"read V1: connection lost\"")
}

func TestRunWriteRetriesWithReconnect(t *testing.T) {
	client := &fakeClient{
		values:        make(map[gos7logo.VMAddr]uint32),
		writeFailures: 1,
	}
	logs := captureLogs(t)

	err := run(context.Background(), []string{optionRetry, commandWrite, "V1", "7"},
		&bytes.Buffer{}, func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Equal(t, 2, client.writeCalls)
	require.Equal(t, uint32(7), client.values[gos7logo.MustNewVMAddrFromString("V1")])
	require.Contains(t, logs.String(), "V1=7")
	require.Contains(t, logs.String(), "operation failed; retrying")
	require.Contains(t, logs.String(), "error=\"write V1: connection lost\"")
}

func TestRunReadReconnectStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &fakeClient{values: make(map[gos7logo.VMAddr]uint32)}
	logs := captureLogs(t)

	err := run(ctx, []string{optionRetry, commandRead, "V1"},
		&bytes.Buffer{}, func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Zero(t, client.readCalls)
	require.Empty(t, logs.String())
}

func TestRunRejectsWritingOutputBeforeConnecting(t *testing.T) {
	connected := false
	err := run(context.Background(), []string{commandWrite, "Q1", "1"},
		&bytes.Buffer{}, func(gos7logo.Config) logoClient {
			connected = true
			return &fakeClient{}
		})

	require.ErrorIs(t, err, gos7logo.ErrReadOnlyAddress)
	require.EqualError(t, err, "cannot write Q1: address is read-only")
	require.False(t, connected)
}

func TestRunRejectsInvalidArgumentsBeforeConnecting(t *testing.T) {
	connected := false
	newClient := func(gos7logo.Config) logoClient {
		connected = true
		return &fakeClient{}
	}

	err := run(context.Background(), []string{commandWrite, "V3", "256"},
		&bytes.Buffer{}, newClient)

	require.EqualError(t, err, "invalid value for V3: byte value must be between 0 and 255")
	require.False(t, connected)
}

func TestRunRejectsInvalidOutputFormatBeforeConnecting(t *testing.T) {
	connected := false
	err := run(context.Background(), []string{"-f", "q", commandRead, "V1"},
		&bytes.Buffer{}, func(gos7logo.Config) logoClient {
			connected = true
			return &fakeClient{}
		})

	require.EqualError(t, err,
		`invalid output format "q": expected d, x, h, b, or o, optionally followed by _`)
	require.False(t, connected)
}

func TestRunWatchStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &fakeClient{values: map[gos7logo.VMAddr]uint32{
		gos7logo.MustNewVMAddrFromString("V1"): 7,
	}}
	logs := captureLogs(t)

	err := run(ctx, []string{commandWatch, "-interval", "1ms", "V1"}, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Empty(t, logs.String())
}

func TestRunWatchUsesStream(t *testing.T) {
	client := &fakeClient{values: map[gos7logo.VMAddr]uint32{
		gos7logo.MustNewVMAddrFromString("V1"): 7,
	}}
	logs := captureLogs(t)

	err := run(context.Background(), []string{commandWatch, "V1"}, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Contains(t, logs.String(), "V1=7")
}

func TestRunWatchEnablesReconnect(t *testing.T) {
	client := &fakeClient{values: map[gos7logo.VMAddr]uint32{
		gos7logo.MustNewVMAddrFromString("V1"): 7,
	}, streamErr: errors.New("connection lost")}
	logs := captureLogs(t)
	var config gos7logo.Config

	err := run(context.Background(), []string{optionRetry, commandWatch, "V1"},
		&bytes.Buffer{}, func(actual gos7logo.Config) logoClient {
			config = actual
			return client
		})

	require.NoError(t, err)
	require.True(t, config.Reconnect)
	require.Contains(t, logs.String(), "V1=7")
	require.Contains(t, logs.String(), "stream failed; retrying")
	require.Contains(t, logs.String(), "error=\"connection lost\"")
}

func TestCancelOnSignalWritesCarriageReturnBeforeCancelling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	var output bytes.Buffer

	go cancelOnSignal(ctx, signals, &output, cancel)
	signals <- syscall.SIGINT
	<-ctx.Done()

	require.Equal(t, "\r", output.String())
}

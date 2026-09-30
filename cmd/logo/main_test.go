package main

import (
	"bytes"
	"context"
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
	addressVW4   = "VW4"
)

type fakeClient struct {
	values map[gos7logo.VmAddr]uint32
	closed bool
}

func (c *fakeClient) Connect(context.Context) error {
	return nil
}

func (c *fakeClient) Read(addr gos7logo.VmAddr) (uint32, error) {
	return c.values[addr], nil
}

func (c *fakeClient) Stream(
	ctx context.Context,
	_ time.Duration,
	addresses ...gos7logo.VmAddr,
) (<-chan gos7logo.StreamResult, error) {
	results := make(chan gos7logo.StreamResult, 1)
	defer close(results)
	select {
	case <-ctx.Done():
		return results, nil
	default:
	}
	values := make(gos7logo.VmAddrValues, len(addresses))
	for i, addr := range addresses {
		values[i] = gos7logo.VmAddrValue{VmAddr: addr, Value: c.values[addr]}
	}
	results <- gos7logo.StreamResult{Data: values}
	return results, nil
}

func (c *fakeClient) Write(addr gos7logo.VmAddr, value uint32) error {
	c.values[addr] = value
	return nil
}

func (c *fakeClient) Disconnect() error {
	c.closed = true
	return nil
}

func TestRunRead(t *testing.T) {
	addr := gos7logo.MustNewVmAddrFromString(addressVW4)
	client := &fakeClient{values: map[gos7logo.VmAddr]uint32{addr: 42}}
	var output bytes.Buffer

	err := run(context.Background(), []string{commandRead, addressVW4}, &output, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Equal(t, "VW4=42\n", output.String())
	require.True(t, client.closed)
}

func TestRunReadRange(t *testing.T) {
	client := &fakeClient{values: map[gos7logo.VmAddr]uint32{
		gos7logo.MustNewVmAddrFromString("V3"): 3,
		gos7logo.MustNewVmAddrFromString("V4"): 4,
		gos7logo.MustNewVmAddrFromString("V5"): 5,
	}}
	var output bytes.Buffer

	err := run(context.Background(), []string{commandRead, "V3-V5"}, &output, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Equal(t, "V3=3\nV4=4\nV5=5\n", output.String())
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
	addr := gos7logo.MustNewVmAddrFromString(addressVW4)
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
			client := &fakeClient{values: map[gos7logo.VmAddr]uint32{addr: 42}}
			var output bytes.Buffer

			err := run(context.Background(), []string{"-f", test.format, commandRead, addressVW4},
				&output, &bytes.Buffer{}, func(gos7logo.Config) logoClient { return client })

			require.NoError(t, err)
			require.Equal(t, "VW4="+test.want+"\n", output.String())
		})
	}
}

func TestFormatValueGroupsDecimalFromRight(t *testing.T) {
	addr := gos7logo.MustNewVmAddrFromString("VD4")
	require.Equal(t, "123_456", formatValue(addr, 123456, "d_"))
}

func TestRunWrite(t *testing.T) {
	client := &fakeClient{values: make(map[gos7logo.VmAddr]uint32)}
	var output bytes.Buffer

	err := run(context.Background(), []string{commandWrite, "V3", "0xff", "V4.2", "1"},
		&output, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Equal(t, uint32(255), client.values[gos7logo.MustNewVmAddrFromString("V3")])
	require.Equal(t, uint32(1), client.values[gos7logo.MustNewVmAddrFromString("V4.2")])
	require.Equal(t, "V3=255\nV4.2=1\n", output.String())
}

func TestRunRejectsWritingOutputBeforeConnecting(t *testing.T) {
	connected := false
	err := run(context.Background(), []string{commandWrite, "Q1", "1"},
		&bytes.Buffer{}, &bytes.Buffer{}, func(gos7logo.Config) logoClient {
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
		&bytes.Buffer{}, &bytes.Buffer{}, newClient)

	require.EqualError(t, err, "invalid value for V3: byte value must be between 0 and 255")
	require.False(t, connected)
}

func TestRunRejectsInvalidOutputFormatBeforeConnecting(t *testing.T) {
	connected := false
	err := run(context.Background(), []string{"-f", "q", commandRead, "V1"},
		&bytes.Buffer{}, &bytes.Buffer{}, func(gos7logo.Config) logoClient {
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
	client := &fakeClient{values: map[gos7logo.VmAddr]uint32{
		gos7logo.MustNewVmAddrFromString("V1"): 7,
	}}
	var output bytes.Buffer

	err := run(ctx, []string{"watch", "-interval", "1ms", "V1"}, &output, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Empty(t, output.String())
}

func TestRunWatchUsesStream(t *testing.T) {
	client := &fakeClient{values: map[gos7logo.VmAddr]uint32{
		gos7logo.MustNewVmAddrFromString("V1"): 7,
	}}
	var output bytes.Buffer

	err := run(context.Background(), []string{"watch", "V1"}, &output, &bytes.Buffer{},
		func(gos7logo.Config) logoClient { return client })

	require.NoError(t, err)
	require.Contains(t, output.String(), " V1=7\n")
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

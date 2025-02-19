package gos7logo

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	gos7patch "github.com/axon-expert/gos7-logo-client/gos7-patch"
)

type DataType int

const (
	Byte DataType = iota
	Bit
	Word
	Counter
	Timer
	DWord
	Real
)

func (t DataType) Size() int {
	switch t {
	case Bit, Byte:
		return 1
	case Word, Counter, Timer:
		return 2
	case DWord, Real:
		return 4
	default:
		return 0
	}
}

func parseTypeByVmAddr(addr string) (DataType, error) {
	switch {
	case regexp.MustCompile(`V[0-9]{1,4}\.[0-7]`).MatchString(addr):
		return Bit, nil
	case regexp.MustCompile(`V[0-9]+`).MatchString(addr):
		return Byte, nil
	case regexp.MustCompile(`VW[0-9]+`).MatchString(addr):
		return Word, nil
	case regexp.MustCompile(`VD[0-9]+`).MatchString(addr):
		return DWord, nil
	}

	return 0, errors.New("unknown address format")
}

type vmAddr struct {
	Type DataType
	Byte uint32
	Bit  uint8
}

func NewVmAddr(t DataType, byteAddr uint32, bit uint8) vmAddr {
	return vmAddr{Type: t, Bit: bit, Byte: byteAddr}
}

func NewVmAddrFromString(addr string) (vmAddr, error) {
	addrType, err := parseTypeByVmAddr(addr)
	if err != nil {
		return vmAddr{}, fmt.Errorf("failed parse data type: %s", err)
	}
	addrSlice := strings.Split(addr, ".")
	var bitAddr uint8
	if len(addrSlice) > 1 {
		bitAddrInt, err := strconv.Atoi(addrSlice[1])
		if err != nil {
			return vmAddr{}, fmt.Errorf("`%s` is not digits", addrSlice[1])
		}
		bitAddr = uint8(bitAddrInt)
	}
	var byteAddr uint32
	for i, ch := range addrSlice[0] {
		if unicode.IsDigit(ch) {
			tempByteAddr, err := strconv.Atoi(addrSlice[0][i:])
			if err != nil {
				return vmAddr{}, fmt.Errorf("`%s` is not digits", addrSlice[0][i:])
			}
			byteAddr = uint32(tempByteAddr)
			break
		}
	}

	return vmAddr{Type: addrType, Byte: byteAddr, Bit: bitAddr}, nil
}

type VmAddrValue struct {
	VmAddr vmAddr
	Value  uint32
}

type Client interface {
	Read(addr vmAddr) (uint32, error)
	Write(addr vmAddr, value uint32) error
	WriteMany(addrs ...VmAddrValue) error
	Disconnect() error
}

type client struct {
	helper   gos7patch.Helper
	client   gos7patch.Client
	handler  *gos7patch.TCPClientHandler
	area     string
	dbNumber int
}

func NewClient(addr string, rack int, slot int, snap7TSAP, logoTSAP uint16) (*client, error) {
	handler := gos7patch.NewTCPClientHandlerWithTSAP(addr, rack, slot, snap7TSAP, logoTSAP)
	if err := handler.Connect(); err != nil {
		return nil, err
	}
	return &client{
		area: "DB", dbNumber: 1,
		client:  gos7patch.NewClient(handler),
		handler: handler}, nil
}

func (c *client) Read(addr vmAddr) (uint32, error) {
	size := addr.Type.Size()
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
		return 0, err
	}
	result, err := c.getIntFromBuffer(addr, buff)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (c *client) Write(addr vmAddr, value uint32) error {
	size := addr.Type.Size()
	buff := make([]byte, size)
	if err := c.writeToBuffer(addr, buff, value); err != nil {
		return err
	}
	if err := c.client.AGWriteDB(c.dbNumber, int(addr.Byte), size, buff); err != nil {
		return err
	}
	return nil
}
func (c *client) WriteMany(args ...VmAddrValue) error {
	if len(args) == 0 {
		return fmt.Errorf("failed `WriteMany`: args is empty")
	}
	minByte := slices.MinFunc(args, compareVmAddrByte)
	maxByte := slices.MaxFunc(args, compareVmAddrByte)
	size := int(maxByte.VmAddr.Byte-minByte.VmAddr.Byte) + 1
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, int(minByte.VmAddr.Byte), size, buff); err != nil {
		return err
	}
	for _, val := range args {
		offset := int(val.VmAddr.Byte - minByte.VmAddr.Byte)
		if err := c.writeToBuffer(val.VmAddr, buff[offset:], val.Value); err != nil {
			return err
		}
	}
	if err := c.client.AGWriteDB(c.dbNumber, int(minByte.VmAddr.Byte), size, buff); err != nil {
		return err
	}
	return nil
}

func (c *client) writeToBuffer(addr vmAddr, buff []byte, value uint32) error {
	switch addr.Type {
	case Bit:
		if err := c.client.AGReadDB(c.dbNumber, int(addr.Byte), addr.Type.Size(), buff); err != nil {
			return err
		}
		if value > 0 {
			buff[0] |= addr.Bit << 0
		} else {
			buff[0] &^= addr.Bit << 0
		}
	case Byte:
		c.helper.SetValueAt(buff, 0, uint8(value))
	case DWord:
		c.helper.SetValueAt(buff, 0, uint32(value))
	case Real:
		c.helper.SetValueAt(buff, 0, float32(value))
	case Word, Counter, Timer:
		c.helper.SetValueAt(buff, 0, uint16(value))
	default:
		return errors.New("write: unknown data type")
	}

	return nil
}
func (c *client) getIntFromBuffer(addr vmAddr, buff []byte) (uint32, error) {
	switch addr.Type {
	case Bit:
		var result uint8
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result >> addr.Bit & 1), nil
	case Byte:
		var result uint8
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case Word, Counter, Timer:
		var result uint16
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case DWord:
		var result uint32
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case Real:
		var result float32
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	}

	return 0, errors.New("write: unknown data type")
}

func (c *client) Disconnect() error {
	return c.handler.Close()
}

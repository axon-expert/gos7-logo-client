package gos7logo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/robinson/gos7"
)

type dataType int

func (t dataType) Size() int {
	switch t {
	case Bit:
		return 2
	case Byte:
		return 1
	case Word:
		return 2
	case DWord:
		return 4
	case Real:
		return 4
	case Counter:
		return 2
	case Timer:
		return 2
	default:
		return 0
	}
}

const (
	Byte dataType = iota
	Bit
	Word
	Counter
	Timer
	DWord
	Real
)

type VmAddr struct {
	Bit    *int
	Prefix string
	Byte   int
}

func NewVmAddr(p string, byteAddr int, bit ...int) VmAddr {
	var bitAddr *int = nil
	if len(bit) > 0 {
		bitAddr = &bit[0]
	}
	return VmAddr{Prefix: p, Bit: bitAddr, Byte: byteAddr}
}

func NewVmAddrFromString(addr string) (VmAddr, error) {
	var builder strings.Builder
	addrSlice := strings.Split(addr, ".")
	var bitAddr *int
	if len(addrSlice) > 1 {
		bitAddrInt, err := strconv.Atoi(addrSlice[1])
		if err != nil {
			return VmAddr{}, fmt.Errorf("`%s` is not digits", addrSlice[1])
		}
		bitAddr = &bitAddrInt
	}
	var byteAddr int
	var prefix string
	for i, ch := range addrSlice[0] {
		if unicode.IsLetter(ch) {
			builder.WriteRune(ch)
		} else if unicode.IsDigit(ch) {
			tempByteAddr, err := strconv.Atoi(addrSlice[0][i:])
			if err != nil {
				return VmAddr{}, fmt.Errorf("`%s` is not digits", addrSlice[0][i:])
			}
			byteAddr = tempByteAddr
			prefix = builder.String()
			break
		}
	}
	return VmAddr{Prefix: prefix, Bit: bitAddr, Byte: byteAddr}, nil
}

func (a *VmAddr) String() string {
	var builder strings.Builder
	builder.WriteString(a.Prefix)
	builder.WriteString(strconv.Itoa(a.Byte))
	if a.Bit != nil {
		builder.WriteString(".")
		builder.WriteString(strconv.Itoa(*a.Bit))
	}
	return builder.String()
}

func parseVmAddr(addr VmAddr) (int, dataType, error) {
	switch addr.Prefix {
	case "V":
		if addr.Bit == nil {
			return addr.Byte, Byte, nil
		}
		return addr.Byte*8 + *addr.Bit, Bit, nil
	case "VW":
		return addr.Byte, Word, nil
	case "VD":
		return addr.Byte, DWord, nil
	}

	return 0, 0, errors.New("unknown address format")
}

type ConnectOpt struct {
	Addr string
	Rack int
	Slot int
}

type Client interface {
	Read(addr VmAddr) (int, error)
	Write(addr VmAddr, value int) error
	Disconnect() error
}

type client struct {
	helper   gos7.Helper
	client   gos7.Client
	handler  *gos7.TCPClientHandler
	area     string
	dbNumber int
}

func NewClient(opt *ConnectOpt) (*client, error) {
	handler := gos7.NewTCPClientHandler(opt.Addr, opt.Rack, opt.Slot)
	if err := handler.Connect(); err != nil {
		return nil, err
	}
	return &client{
		area: "DB", dbNumber: 1,
		client:  gos7.NewClient(handler),
		handler: handler}, nil
}

func (c *client) Read(addr VmAddr) (int, error) {
	start, dataType, err := parseVmAddr(addr)
	if err != nil {
		return 0, err
	}
	size := dataType.Size()
	buff := make([]byte, size)
	if err := c.client.AGReadDB(c.dbNumber, start, size, buff); err != nil {
		return 0, err
	}
	result, err := c.getIntFromBuffer(dataType, buff)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (c *client) Write(addr VmAddr, value int) error {
	start, dataType, err := parseVmAddr(addr)
	if err != nil {
		return err
	}
	size := dataType.Size()
	buff := make([]byte, size)
	if err := c.writeToBuffer(dataType, buff, value); err != nil {
		return err
	}
	if err := c.client.AGWriteDB(c.dbNumber, start, size, buff); err != nil {
		return err
	}
	return nil
}

func (c *client) writeToBuffer(dataType dataType, buff []byte, value int) error {
	switch dataType {
	case Bit:
		c.helper.SetValueAt(buff, 0, int16(value))
	case Byte:
		c.helper.SetValueAt(buff, 0, int8(value))
	case Word:
		c.helper.SetValueAt(buff, 0, int16(value))
	case DWord:
		c.helper.SetValueAt(buff, 0, int32(value))
	case Real:
		c.helper.SetValueAt(buff, 0, float32(value))
	case Counter:
		c.helper.SetValueAt(buff, 0, int16(value))
	case Timer:
		c.helper.SetValueAt(buff, 0, int16(value))
	default:
		return errors.New("write: unknown data type")
	}

	return nil
}
func (c *client) getIntFromBuffer(dataType dataType, buff []byte) (int, error) {
	switch dataType {
	case Bit:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Byte:
		var result int8
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Word:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case DWord:
		var result int32
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Real:
		var result float32
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Counter:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	case Timer:
		var result int16
		c.helper.GetValueAt(buff, 0, &result)
		return int(result), nil
	}

	return 0, errors.New("write: unknown data type")
}

func (c *client) Disconnect() error {
	return c.handler.Close()
}

package gos7logo

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type DataType uint8

const (
	Byte DataType = iota
	Bit
	Word
	DWord
	Output
)

const (
	maxVariableByte = uint16(850)
	outputByteStart = uint16(1064)
	outputByteEnd   = uint16(1071)
)

func (t DataType) Size() int {
	switch t {
	case Bit, Byte, Output:
		return 1
	case Word:
		return 2
	case DWord:
		return 4
	default:
		return 0
	}
}

func (t DataType) String() string {
	switch t {
	case Byte:
		return "V"
	case Bit:
		return "V"
	case Word:
		return "VW"
	case DWord:
		return "VD"
	case Output:
		return "Q"
	}
	return "V"
}

func parseTypeByVMAddr(addr string) (DataType, error) {
	switch {
	case regexp.MustCompile(`^V[0-9]+\.[0-7]$`).MatchString(addr):
		return Bit, nil
	case regexp.MustCompile(`^V[0-9]+$`).MatchString(addr):
		return Byte, nil
	case regexp.MustCompile(`^VW[0-9]+$`).MatchString(addr):
		return Word, nil
	case regexp.MustCompile(`^VD[0-9]+$`).MatchString(addr):
		return DWord, nil
	case regexp.MustCompile(`^Q[0-9]+$`).MatchString(addr):
		return Output, nil
	}

	return 0, errors.New("unknown address format")
}

type VMAddr struct {
	Type DataType
	Byte uint16
	Bit  uint8
}

func NewVMAddr(t DataType, byteAddr uint16, bit uint8) (VMAddr, error) {
	addr := VMAddr{Type: t, Bit: bit, Byte: byteAddr}
	if err := addr.Validate(); err != nil {
		return VMAddr{}, err
	}
	return addr, nil
}

func MustNewVMAddr(t DataType, byteAddr uint16, bit uint8) VMAddr {
	addr, err := NewVMAddr(t, byteAddr, bit)
	if err != nil {
		panic(err)
	}
	return addr
}

func (a VMAddr) Validate() error {
	if a.Type.Size() == 0 {
		return fmt.Errorf("unknown data type: %d", a.Type)
	}
	if a.Type == Bit || a.Type == Output {
		if a.Bit > 7 {
			return fmt.Errorf("bit index must be between 0 and 7: %d", a.Bit)
		}
		if a.Type == Output && (a.Byte < outputByteStart || a.Byte > outputByteEnd) {
			return fmt.Errorf("output address must be between Q1 and Q64")
		}
	} else if a.Bit != 0 {
		return fmt.Errorf("bit index must be zero for data type %v: %d", a.Type, a.Bit)
	}
	if a.Type != Output && uint32(a.Byte)+uint32(a.Type.Size())-1 > uint32(maxVariableByte) {
		return fmt.Errorf(
			"variable address at byte %d with size %d ends after V%d",
			a.Byte,
			a.Type.Size(),
			maxVariableByte,
		)
	}
	return nil
}

func NewVMAddrFromString(addr string) (VMAddr, error) {
	a := VMAddr{}
	if err := a.UnmarshalText([]byte(addr)); err != nil {
		return a, err
	}
	return a, nil
}

func MustNewVMAddrFromString(addr string) VMAddr {
	parsed, err := NewVMAddrFromString(addr)
	if err != nil {
		panic(err)
	}
	return parsed
}

func (addr VMAddr) MarshalText() ([]byte, error) {
	if err := addr.Validate(); err != nil {
		return nil, err
	}
	if addr.Type == Output {
		output := uint32(addr.Byte-outputByteStart)*8 + uint32(addr.Bit) + 1
		return fmt.Appendf(nil, "Q%d", output), nil
	}
	if addr.Type == Bit {
		return fmt.Appendf(nil, "%s%d.%d", addr.Type.String(), addr.Byte, addr.Bit), nil
	}
	return fmt.Appendf(nil, "%s%d", addr.Type.String(), addr.Byte), nil
}

func (addr VMAddr) String() string {
	raw, _ := addr.MarshalText()
	return string(raw)
}

func (a *VMAddr) UnmarshalText(raw []byte) error {
	addr := string(raw)
	addrType, err := parseTypeByVMAddr(addr)
	if err != nil {
		return fmt.Errorf("failed parse data type: %s", err)
	}
	if addrType == Output {
		output, err := strconv.ParseUint(addr[1:], 10, 8)
		if err != nil {
			return fmt.Errorf("invalid output address: %w", err)
		}
		if output < 1 || output > 64 {
			return errors.New("output address must be between Q1 and Q64")
		}
		output--
		*a = VMAddr{
			Type: Output,
			Byte: outputByteStart + uint16(output/8),
			Bit:  uint8(output % 8),
		}
		return nil
	}

	addrSlice := strings.Split(addr, ".")
	prefixLength := 1
	if addrType == Word || addrType == DWord {
		prefixLength = 2
	}
	byteAddr, err := strconv.ParseUint(addrSlice[0][prefixLength:], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid byte address: %w", err)
	}
	parsed := VMAddr{Type: addrType, Byte: uint16(byteAddr)}
	if addrType == Bit {
		bitAddr, err := strconv.ParseUint(addrSlice[1], 10, 8)
		if err != nil {
			return fmt.Errorf("invalid bit address: %w", err)
		}
		parsed.Bit = uint8(bitAddr)
	}
	if err := parsed.Validate(); err != nil {
		return err
	}
	*a = parsed
	return nil
}

type VMAddrValue struct {
	VMAddr VMAddr
	Value  uint32
}

type VMAddrValues []VMAddrValue

func (values VMAddrValues) At(index int) (VMAddrValue, bool) {
	if index < 0 || index >= len(values) {
		return VMAddrValue{}, false
	}
	return values[index], true
}

func (values VMAddrValues) Get(addr VMAddr) (uint32, bool) {
	for _, value := range values {
		if value.VMAddr == addr {
			return value.Value, true
		}
	}
	return 0, false
}

func compareVMAddrByte(lhs, rhs VMAddr) int {
	return cmp.Compare(lhs.Byte, rhs.Byte)
}

func (c *client) getIntFromBuffer(addr VMAddr, buff []byte) (uint32, error) {
	if err := addr.Validate(); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	if len(buff) < addr.Type.Size() {
		return 0, fmt.Errorf("buffer too small for type %v", addr.Type)
	}
	switch addr.Type {
	case Bit, Output:
		var result uint8
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result >> addr.Bit & 1), nil
	case Byte:
		var result uint8
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case Word:
		var result uint16
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	case DWord:
		var result uint32
		c.helper.GetValueAt(buff, 0, &result)
		return uint32(result), nil
	}

	return 0, errors.New("read: unknown data type")
}

func vmAddrRange(addrs []VMAddr) (uint16, int, error) {
	if len(addrs) == 0 {
		return 0, 0, errors.New("addresses are empty")
	}
	start := addrs[0].Byte
	var end uint32
	for _, addr := range addrs {
		if err := addr.Validate(); err != nil {
			return 0, 0, err
		}
		if addr.Byte < start {
			start = addr.Byte
		}
		addrEnd := uint32(addr.Byte) + uint32(addr.Type.Size())
		if addrEnd > end {
			end = addrEnd
		}
	}
	span := end - uint32(start)
	return start, int(span), nil
}

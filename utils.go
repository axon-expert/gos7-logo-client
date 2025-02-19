package gos7logo

func compareVmAddrByte(a, b VmAddrValue) int {
	return int(a.VmAddr.Byte) - int(b.VmAddr.Byte)
}

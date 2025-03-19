package gos7logo

func compareVmAddrByte(lhs, rhs VmAddr) int {
	return int(lhs.Byte) - int(rhs.Byte)
}

func compareVmAddrValueByte(a, b VmAddrValue) int {
	return int(a.VmAddr.Byte) - int(b.VmAddr.Byte)
}

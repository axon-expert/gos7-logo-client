package gos7patch

const (
	// Area ID
	s7areape = 0x81 //process inputs
	s7areapa = 0x82 //process outputs
	s7areamk = 0x83 //Merkers
	s7areadb = 0x84 //DB
	s7areact = 0x1C //counters
	s7areatm = 0x1D //timers

	// Word Length
	s7wlbit     = 0x01 //Bit (inside a word)
	s7wlbyte    = 0x02 //Byte (8 bit)
	s7wlChar    = 0x03
	s7wlword    = 0x04 //Word (16 bit)
	s7wlint     = 0x05
	s7wldword   = 0x06 //Double Word (32 bit)
	s7wldint    = 0x07
	s7wlreal    = 0x08 //Real (32 bit float)
	s7wlcounter = 0x1C //Counter (16 bit)
	s7wltimer   = 0x1D //Timer (16 bit)

	// PLC Status
	s7CpuStatusUnknown = 0
	s7CpuStatusRun     = 8
	s7CpuStatusStop    = 4

	//size header
	sizeHeaderRead  int = 31 // Header Size when Reading
	sizeHeaderWrite int = 35 // Header Size when Writing

	// Result transport size
	tsResBit   = 3
	tsResByte  = 4
	tsResInt   = 5
	tsResReal  = 7
	tsResOctet = 9
)

// Command minos checks that a Mach-O binary is x86_64 and declares a minimum
// macOS version no newer than the one given, e.g. "minos build/awg-hs 10.13".
package main

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	lcVersionMinMacOSX = 0x24
	lcBuildVersion     = 0x32
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: minos BINARY MAJOR.MINOR")
		os.Exit(2)
	}
	var wantMajor, wantMinor uint32
	if _, err := fmt.Sscanf(os.Args[2], "%d.%d", &wantMajor, &wantMinor); err != nil {
		fail("bad version %q", os.Args[2])
	}

	f, err := macho.Open(os.Args[1])
	if err != nil {
		fail("%v", err)
	}
	defer f.Close()
	if f.Cpu != macho.CpuAmd64 {
		fail("%s is %v, want x86_64", os.Args[1], f.Cpu)
	}

	var minos uint32
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) < 16 {
			continue
		}
		switch f.ByteOrder.Uint32(raw) {
		case lcBuildVersion:
			minos = binary.LittleEndian.Uint32(raw[12:16])
		case lcVersionMinMacOSX:
			minos = binary.LittleEndian.Uint32(raw[8:12])
		}
	}
	if minos == 0 {
		fail("%s declares no minimum macOS version", os.Args[1])
	}
	major, minor := minos>>16, (minos>>8)&0xff
	fmt.Printf("%s: x86_64, minimum macOS %d.%d\n", os.Args[1], major, minor)
	if major > wantMajor || (major == wantMajor && minor > wantMinor) {
		fail("needs macOS %d.%d, which is newer than %d.%d", major, minor, wantMajor, wantMinor)
	}
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "minos: "+format+"\n", args...)
	os.Exit(1)
}

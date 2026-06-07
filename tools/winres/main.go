// Command winres generates the Windows version-info resource objects
// (rsrc_windows_amd64.syso / rsrc_windows_arm64.syso) that `go build` links
// into the Windows executables, so the .exe carries publisher metadata
// (CompanyName, ProductName, version...). This is what shows up in the file's
// Properties > Details and helps reduce antivirus false-positives — unsigned Go
// binaries with NO metadata look "anonymous" and trip heuristics more easily.
//
// It is intentionally self-contained (standard library only): no external
// tooling to install or trust. To bump the version, edit the consts below and
// run, from the repo root:
//
//	go run ./tools/winres
//
// then commit the regenerated *.syso files.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"unicode/utf16"
)

// ---- Edit these on each release -------------------------------------------
const (
	companyName      = "Atines"
	productName      = "Atines Smart Stream"
	fileDescription  = "Atines Smart Stream - ponte RTSP para RTMP com interface web"
	internalName     = "atines-smart-stream"
	originalFilename = "atines-smart-stream.exe"
	legalCopyright   = "Atines"

	verMajor = 1
	verMinor = 0
	verPatch = 2
	verBuild = 0
)

// ---------------------------------------------------------------------------

func versionString() string {
	return fmt.Sprintf("%d.%d.%d.%d", verMajor, verMinor, verPatch, verBuild)
}

// utf16z encodes s as UTF-16LE with a terminating NUL code unit.
func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2+2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	// last 2 bytes already zero = NUL terminator
	return b
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// vsNode builds one node of the VS_VERSIONINFO tree:
//
//	WORD wLength; WORD wValueLength; WORD wType; WCHAR szKey[]; pad;
//	[Value; pad]; [Children...]
//
// valueLen is what goes into wValueLength (bytes for binary nodes, UTF-16 code
// units for string nodes, 0 for containers), per the Win32 spec.
func vsNode(key string, valueLen, typ int, value []byte, children ...[]byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0}) // wLength placeholder
	writeU16(&b, uint16(valueLen))
	writeU16(&b, uint16(typ))
	b.Write(utf16z(key))
	pad4(&b)
	if len(value) > 0 {
		b.Write(value)
		pad4(&b)
	}
	for _, c := range children {
		b.Write(c)
		pad4(&b)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out[0:2], uint16(len(out)))
	return out
}

func stringNode(key, val string) []byte {
	v := utf16z(val)
	return vsNode(key, len(v)/2, 1, v) // wValueLength counts UTF-16 units incl. NUL
}

// versionInfo builds the full VS_VERSIONINFO resource payload.
func versionInfo() []byte {
	var ffi bytes.Buffer
	writeU32(&ffi, 0xFEEF04BD)            // dwSignature
	writeU32(&ffi, 0x00010000)            // dwStrucVersion
	writeU32(&ffi, verMajor<<16|verMinor) // dwFileVersionMS
	writeU32(&ffi, verPatch<<16|verBuild) // dwFileVersionLS
	writeU32(&ffi, verMajor<<16|verMinor) // dwProductVersionMS
	writeU32(&ffi, verPatch<<16|verBuild) // dwProductVersionLS
	writeU32(&ffi, 0x3F)                  // dwFileFlagsMask
	writeU32(&ffi, 0)                     // dwFileFlags
	writeU32(&ffi, 0x00040004)            // dwFileOS = VOS_NT_WINDOWS32
	writeU32(&ffi, 0x00000001)            // dwFileType = VFT_APP
	writeU32(&ffi, 0)                     // dwFileSubtype
	writeU32(&ffi, 0)                     // dwFileDateMS
	writeU32(&ffi, 0)                     // dwFileDateLS

	ver := versionString()
	strTable := vsNode("040904B0", 0, 1, nil, // langID 0x0409 (en-US), CP 0x04B0 (Unicode)
		stringNode("CompanyName", companyName),
		stringNode("FileDescription", fileDescription),
		stringNode("FileVersion", ver),
		stringNode("InternalName", internalName),
		stringNode("LegalCopyright", legalCopyright),
		stringNode("OriginalFilename", originalFilename),
		stringNode("ProductName", productName),
		stringNode("ProductVersion", ver),
	)
	stringFileInfo := vsNode("StringFileInfo", 0, 1, nil, strTable)

	var trans bytes.Buffer
	writeU32(&trans, 0x04B00409) // codepage<<16 | langID
	varNode := vsNode("Translation", 4, 0, trans.Bytes())
	varFileInfo := vsNode("VarFileInfo", 0, 1, nil, varNode)

	return vsNode("VS_VERSION_INFO", ffi.Len(), 0, ffi.Bytes(), stringFileInfo, varFileInfo)
}

// COFF / .rsrc constants.
const (
	imageMachineAMD64 = 0x8664
	imageMachineARM64 = 0xAA64

	relAMD64Addr32NB = 0x0003 // IMAGE_REL_AMD64_ADDR32NB
	relARM64Addr32NB = 0x0002 // IMAGE_REL_ARM64_ADDR32NB

	rtVersion = 16 // RT_VERSION
	resID     = 1
	langID    = 0x0409

	scnInitializedData = 0x00000040
	scnMemRead         = 0x40000000
	symClassStatic     = 3
)

// buildResourceSection lays out the .rsrc section bytes for a single
// RT_VERSION resource, and returns (sectionBytes, relocFieldOffset) where
// relocFieldOffset is the offset of the IMAGE_RESOURCE_DATA_ENTRY.OffsetToData
// field that needs a section-relative relocation.
func buildResourceSection(data []byte) ([]byte, uint32) {
	// Three nested directories (root->id->lang), each: 16-byte header + 8-byte
	// entry. Then a 16-byte data entry, then the resource bytes.
	const (
		dirSize   = 16 + 8
		dataEntry = 16
	)
	rootOff := uint32(0)
	idOff := rootOff + dirSize
	langOff := idOff + dirSize
	deOff := langOff + dirSize
	resOff := deOff + dataEntry // resource payload offset within section

	var b bytes.Buffer
	dir := func(id, child uint32, leaf bool) {
		writeU32(&b, 0) // Characteristics
		writeU32(&b, 0) // TimeDateStamp
		writeU16(&b, 0) // MajorVersion
		writeU16(&b, 0) // MinorVersion
		writeU16(&b, 0) // NumberOfNamedEntries
		writeU16(&b, 1) // NumberOfIdEntries
		writeU32(&b, id)
		if leaf {
			writeU32(&b, child) // points to data entry
		} else {
			writeU32(&b, 0x80000000|child) // high bit = subdirectory
		}
	}
	dir(rtVersion, idOff, false)
	dir(resID, langOff, false)
	dir(langID, deOff, true)

	// IMAGE_RESOURCE_DATA_ENTRY
	writeU32(&b, resOff) // OffsetToData (section-relative addend; fixed up by reloc)
	writeU32(&b, uint32(len(data)))
	writeU32(&b, 0) // CodePage
	writeU32(&b, 0) // Reserved

	b.Write(data)
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
	return b.Bytes(), deOff // reloc targets the DATA_ENTRY's first field
}

func buildSyso(machine, relocType uint16) []byte {
	section, relocOff := buildResourceSection(versionInfo())

	const (
		fileHeaderSize = 20
		sectionHdrSize = 40
		relocSize      = 10
	)
	ptrRawData := uint32(fileHeaderSize + sectionHdrSize)
	ptrReloc := ptrRawData + uint32(len(section))
	ptrSymbols := ptrReloc + relocSize
	numSymbols := uint32(1)

	var b bytes.Buffer
	// COFF file header
	writeU16(&b, machine)
	writeU16(&b, 1) // NumberOfSections
	writeU32(&b, 0) // TimeDateStamp
	writeU32(&b, ptrSymbols)
	writeU32(&b, numSymbols)
	writeU16(&b, 0) // SizeOfOptionalHeader
	writeU16(&b, 0) // Characteristics

	// Section header: ".rsrc"
	b.Write([]byte(".rsrc\x00\x00\x00"))
	writeU32(&b, 0) // VirtualSize
	writeU32(&b, 0) // VirtualAddress
	writeU32(&b, uint32(len(section)))
	writeU32(&b, ptrRawData)
	writeU32(&b, ptrReloc)
	writeU32(&b, 0) // PointerToLinenumbers
	writeU16(&b, 1) // NumberOfRelocations
	writeU16(&b, 0) // NumberOfLinenumbers
	writeU32(&b, scnInitializedData|scnMemRead)

	// Section raw data
	b.Write(section)

	// Relocation: make DATA_ENTRY.OffsetToData a real RVA (section base + addend)
	writeU32(&b, relocOff) // VirtualAddress
	writeU32(&b, 0)        // SymbolTableIndex -> symbol 0 (.rsrc section)
	writeU16(&b, relocType)

	// Symbol table: one static symbol for the .rsrc section (value 0 = base)
	b.Write([]byte(".rsrc\x00\x00\x00"))
	writeU32(&b, 0) // Value
	writeU16(&b, 1) // SectionNumber (1-based)
	writeU16(&b, 0) // Type
	b.WriteByte(symClassStatic)
	b.WriteByte(0) // NumberOfAuxSymbols

	// String table: just the mandatory 4-byte size field (no long names).
	writeU32(&b, 4)

	return b.Bytes()
}

func main() {
	targets := []struct {
		file      string
		machine   uint16
		relocType uint16
	}{
		{"rsrc_windows_amd64.syso", imageMachineAMD64, relAMD64Addr32NB},
		{"rsrc_windows_arm64.syso", imageMachineARM64, relARM64Addr32NB},
	}
	for _, t := range targets {
		if err := os.WriteFile(t.file, buildSyso(t.machine, t.relocType), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (v%s)\n", t.file, versionString())
	}
}

func writeU16(b *bytes.Buffer, v uint16) {
	var x [2]byte
	binary.LittleEndian.PutUint16(x[:], v)
	b.Write(x[:])
}

func writeU32(b *bytes.Buffer, v uint32) {
	var x [4]byte
	binary.LittleEndian.PutUint32(x[:], v)
	b.Write(x[:])
}

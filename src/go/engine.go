package main

import (
	"fmt"
	"strings"
)

// Error codes matching JS engine
const (
	HeapFull    = 0x4001
	AuxFull     = 0x4002
	ExpectObj   = 0x4003
	ExpectBound = 0x4004
	LTSFull     = 0x4006
	IOState     = 0x4007
)

// Spacing states
const (
	SPAuto    = 0
	SPNoSpace = 1
	SPNBSP    = 2
	SPPending = 3
	SPSpace   = 4
	SPLine    = 5
	SPPar     = 6
)

const (
	VerMajor = 1
	VerMinor = 0
)

// Keys
var Keys = map[string]int{
	"KEY_BACKSPACE": 8,
	"KEY_RETURN":    13,
	"KEY_UP":        16,
	"KEY_DOWN":      17,
	"KEY_LEFT":      18,
	"KEY_RIGHT":     19,
}

// Status codes
const (
	StatusQuit    = 0
	StatusGetInput = 1
	StatusGetKey   = 2
	StatusRestore  = 3
)

// IO interface for frontends
type IO interface {
	Reset()
	Print(string)
	Nbsp()
	Space()
	SpaceN(int)
	Line()
	Par()
	Flush()
	SetBody(int)
	EnterDiv(int)
	LeaveDiv(int)
	EnterSpan(int)
	LeaveSpan()
	EnterStatus(int, int)
	LeaveStatus()
	SetStyle(int)
	ResetStyle(int)
	Unstyle()
	Clear()
	ClearAll()
	ClearLinks()
	ClearOld()
	ClearDiv()
	ClearStatus()
	HaveLinks() bool
	EnterLink(string)
	LeaveLink()
	EnterLinkRes(Resource)
	LeaveLinkRes()
	EnterSelfLink()
	LeaveSelfLink()
	LeaveAll()
	EmbedRes(Resource)
	CanEmbedRes(Resource) bool
	ProgressBar(int, int)
	Trace(string)
	MeasureDims(int) int
	ScriptOn() bool
	ScriptOff()
	ScriptActive() bool
	Save([]byte) bool
	Restore()
	HaveStyles() bool
	HaveColor() bool
	HaveAlign() bool
}

type Resource struct {
	URL     string
	Alt     string
	Options map[string]interface{}
}

// Encoded state for save/restore
type EncodedState struct {
	Data []byte
	Regs []byte
}

// Engine is the VM engine state
type Engine struct {
	// Story file chunks
	Head []byte
	Code []byte
	Dict []byte
	Init []byte
	Lang []byte
	Maps []byte
	Tags []byte
	Writ []byte
	Look []byte
	Meta []byte
	Urls []byte
	Files map[string][]byte

	// String decoding state
	StrShift   int
	ExtChars   int
	EscBits    int
	EscBoundary int

	// Punctuation tables
	StopChars  []byte
	NospBefore []byte
	NospAfter  []byte

	// VM registers
	Reg [64]uint16

	// VM pointers (all int for clean indexing)
	Inst int
	Cont int
	Top  int
	Env  int
	Cho  int
	Sim  int
	Aux  int
	Trl  int
	Sta  int
	Stc  int
	Cwl  int
	Spc  int

	// Object/heap limits
	Nob int
	Ltb int
	Ltt int

	// Memory arrays
	HeapData []uint16
	AuxData  []uint16
	RamData  []uint16

	// UI state
	Upper    bool
	Trace    bool
	Divs     []int
	InStatus bool
	NSpan    int
	NLink    int

	// Undo
	UndoData   []EncodedState
	PrunedUndo bool

	// Config
	HaveQuit    bool
	HaveTop     bool
	HaveInline  bool
	RandomSeed  uint32
	RandomState uint32

	// Initial state for restart/undo
	InitState *EncodedState

	// IO frontend
	IO IO

	// Temporary variable for LTS operations
	Tmp int
}

// --- Low-level byte operations ---

func get4(data []byte, off int) string {
	return string(data[off : off+4])
}

func get16(data []byte, off int) int {
	return int(data[off])<<8 | int(data[off+1])
}

func get32(data []byte, off int) int {
	return int(data[off])<<24 | int(data[off+1])<<16 | int(data[off+2])<<8 | int(data[off+3])
}

func put4(data []byte, off int, s string) {
	copy(data[off:], s)
}

func put16(data []byte, off int, v int) int {
	data[off] = byte(v >> 8)
	data[off+1] = byte(v)
	return off + 2
}

func put32(data []byte, off int, v int) int {
	data[off] = byte(v >> 24)
	data[off+1] = byte(v >> 16)
	data[off+2] = byte(v >> 8)
	data[off+3] = byte(v)
	return off + 4
}

// --- IFF chunk finding ---

func findChunk(data []byte, name string) []byte {
	size := get32(data, 4) + 8
	pos := 12
	for pos < size {
		chname := get4(data, pos)
		chsize := get32(data, pos+4)
		if chname == name {
			start := pos + 8
			return data[start : start+chsize]
		}
		pos += 8 + ((chsize + 1) & ^1)
	}
	return nil
}

// findChunkIn searches for a chunk across multiple data sources
func findChunkIn(name string, sources ...[]byte) []byte {
	for _, src := range sources {
		if src == nil {
			continue
		}
		if chunk := findChunk(src, name); chunk != nil {
			return chunk
		}
	}
	return nil
}

func findFiles(fileArray []byte) map[string][]byte {
	size := get32(fileArray, 4) + 8
	list := make(map[string][]byte)
	pos := 12
	for pos < size {
		chname := get4(fileArray, pos)
		chsize := get32(fileArray, pos+4)
		if chname == "FILE" {
			i := 0
			for fileArray[pos+8+i] != 0 {
				i++
			}
			fname := string(fileArray[pos+8 : pos+8+i])
			fdata := make([]byte, chsize-i-1)
			copy(fdata, fileArray[pos+8+i+1:pos+8+chsize])
			list[fname] = fdata
		}
		pos += 8 + ((chsize + 1) & ^1)
	}
	return list
}

// --- String decoding ---

func (e *Engine) DecodeChar(aach int) string {
	if e.Upper {
		if aach >= 0x61 && aach <= 0x7a {
			aach ^= 0x20
		} else if aach >= 0x80 {
			aach = int(e.Lang[e.ExtChars+1+(aach&0x7f)*5+1])
		}
		e.Upper = false
	}
	if aach < 0x80 {
		return string(rune(aach))
	}
	aach &= 0x7f
	if aach >= int(e.Lang[e.ExtChars]) {
		e.Upper = false
		return "??"
	}
	entry := e.ExtChars + 1 + aach*5
	uchar := int(e.Lang[entry+2])<<16 | int(e.Lang[entry+3])<<8 | int(e.Lang[entry+4])
	return string(rune(uchar))
}

func (e *Engine) DecodeStr(addr int) string {
	decoder := get16(e.Lang, 0)
	state := 0
	bits := 0
	nbit := 0
	str := ""

	for {
		if nbit == 0 {
			bits = int(e.Writ[addr])
			addr++
			nbit = 8
		}
		code := int(e.Lang[decoder+state*2])
		if bits&0x80 != 0 {
			code = int(e.Lang[decoder+state*2+1])
		}
		bits <<= 1
		nbit--
		if code >= 0x81 {
			state = code & 0x7f
		} else if code == 0x80 {
			break
		} else if code == 0x5f {
			code = 0
			for i := 0; i < e.EscBits; i++ {
				if nbit == 0 {
					bits = int(e.Writ[addr])
					addr++
					nbit = 8
				}
				code <<= 1
				if bits&0x80 != 0 {
					code |= 1
				}
				bits <<= 1
				nbit--
			}
			if e.Head[0] == 0 && e.Head[1] < 4 {
				str += e.DecodeChar(0x80 + code)
			} else if code < e.EscBoundary {
				str += e.DecodeChar(0xa0 + code)
	} else {
			str += " "
				entry := 2 + (code-e.EscBoundary)*3
				l := int(e.Dict[entry])
				charAddr := get16(e.Dict, entry+1)
				for i := 0; i < l; i++ {
					str += e.DecodeChar(int(e.Dict[charAddr+i]))
				}
			}
			state = 0
		} else {
			str += e.DecodeChar(code + 0x20)
			state = 0
		}
	}
	return str
}

// --- Styles and metadata ---

func GetStyles(e *Engine) []map[string]string {
	var styles []map[string]string
	n := get16(e.Look, 0)
	for i := 0; i < n; i++ {
		offs := get16(e.Look, 2+i*2)
		m := make(map[string]string)
		for e.Look[offs] != 0 {
			str := ""
			for e.Look[offs] != 0 {
				str += string(rune(e.Look[offs]))
				offs++
			}
			colon := strings.Index(str, ":")
			if colon > 0 {
				key := str[:colon]
				val := strings.TrimSpace(str[colon+1:])
				m[key] = val
			}
		}
		styles = append(styles, m)
	}
	return styles
}

func GetMetadata(e *Engine) map[string]interface{} {
	result := map[string]interface{}{
		"title":   "Untitled story",
		"release": get16(e.Head, 4),
	}
	keynames := []string{"title", "author", "noun", "blurb", "date", "compiler"}
	if e.Meta != nil {
		offs := 1
		for i := 0; i < int(e.Meta[0]); i++ {
			key := int(e.Meta[offs])
			offs++
			value := ""
			for e.Meta[offs] != 0 {
				value += e.DecodeChar(int(e.Meta[offs]))
				offs++
			}
			if key >= 1 && key <= len(keynames) {
				result[keynames[key-1]] = value
			}
		}
	}
	return result
}

// --- Resource handling ---

func (e *Engine) GetRes(id int) Resource {
	obj := Resource{URL: "", Alt: "", Options: make(map[string]interface{})}
	if e.Urls == nil {
		return obj
	}
	n := get16(e.Urls, 0)
	if id < n {
		offs := get16(e.Urls, 2+id*2)
		altAddr := int(e.Urls[offs])<<16 | int(e.Urls[offs+1])<<8 | int(e.Urls[offs+2])
		obj.Alt = e.DecodeStr(altAddr << e.StrShift)
		i := 3
		for e.Urls[offs+i] != 0 {
			obj.URL += string(rune(e.Urls[offs+i]))
			i++
		}
		i++
		opts := ""
		for e.Urls[offs+i] != 0 {
			opts += string(rune(e.Urls[offs+i]))
			i++
		}
		for _, opt := range strings.Split(opts, ",") {
			opt = strings.TrimSpace(opt)
			if opt == "" {
				continue
			}
			if idx := strings.Index(opt, ":"); idx > 0 {
				k := strings.TrimSpace(opt[:idx])
				v := strings.TrimSpace(opt[idx+1:])
				if existing, ok := obj.Options[k]; ok {
					if arr, ok := existing.([]string); ok {
						obj.Options[k] = append(arr, v)
					} else {
						obj.Options[k] = []string{existing.(string), v}
					}
				} else {
					obj.Options[k] = v
				}
			} else {
				if _, ok := obj.Options[opt]; !ok {
					obj.Options[opt] = true
				}
			}
		}
	}
	return obj
}

func (e *Engine) GetResources() []Resource {
	var ress []Resource
	if e.Urls == nil {
		return ress
	}
	n := get16(e.Urls, 0)
	for i := 0; i < n; i++ {
		ress = append(ress, e.GetRes(i))
	}
	return ress
}

func (e *Engine) GetStoryKey() string {
	i := 0
	meta := GetMetadata(e)
	title, _ := meta["title"].(string)
	str := strings.ReplaceAll(title, " ", "-")
	str = strings.ReplaceAll(str, "'", "-")
	str = strings.ReplaceAll(str, ",", "")
	str += "-"
	for i = 0; i < 6; i++ {
		str += e.DecodeChar(int(e.Head[6+i]))
	}
	str += "-"
	for i = 0; i < 4; i++ {
		hex := fmt.Sprintf("%02x", e.Head[12+i])
		str += hex
	}
	return str
}

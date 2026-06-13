package main

import (
	"testing"
)

// MockIO for testing - captures all output
type MockIO struct {
	output   string
	newlines int
	xpos     int
	width    int
}

func NewMockIO() *MockIO {
	return &MockIO{width: 80}
}

func (io *MockIO) Reset()                                  { io.output = ""; io.newlines = 1; io.xpos = 0 }
func (io *MockIO) Print(s string)                          { io.output += s; io.xpos += len(s) }
func (io *MockIO) Nbsp()                                   { io.output += " "; io.xpos++ }
func (io *MockIO) Space()                                  { io.Print(" ") }
func (io *MockIO) SpaceN(n int)                            {}
func (io *MockIO) Line()                                   { io.output += "\n"; io.newlines++; io.xpos = 0 }
func (io *MockIO) Par()                                    { io.output += "\n\n"; io.newlines += 2; io.xpos = 0 }
func (io *MockIO) Flush()                                  {}
func (io *MockIO) SetBody(id int)                          {}
func (io *MockIO) EnterDiv(id int)                         {}
func (io *MockIO) LeaveDiv(id int)                         {}
func (io *MockIO) EnterSpan(id int)                        {}
func (io *MockIO) LeaveSpan()                              {}
func (io *MockIO) EnterStatus(area, id int)                {}
func (io *MockIO) LeaveStatus()                            {}
func (io *MockIO) SetStyle(s int)                          {}
func (io *MockIO) ResetStyle(s int)                        {}
func (io *MockIO) Unstyle()                                {}
func (io *MockIO) Clear()                                  {}
func (io *MockIO) ClearAll()                               {}
func (io *MockIO) ClearLinks()                             {}
func (io *MockIO) ClearOld()                               {}
func (io *MockIO) ClearDiv()                               {}
func (io *MockIO) ClearStatus()                            {}
func (io *MockIO) HaveLinks() bool                         { return false }
func (io *MockIO) EnterLink(s string)                      {}
func (io *MockIO) LeaveLink()                              {}
func (io *MockIO) EnterLinkRes(r Resource)                 {}
func (io *MockIO) LeaveLinkRes()                           {}
func (io *MockIO) EnterSelfLink()                          {}
func (io *MockIO) LeaveSelfLink()                          {}
func (io *MockIO) LeaveAll()                               { io.Line() }
func (io *MockIO) EmbedRes(r Resource)                     { io.Print("["); io.Print(r.Alt); io.Print("]") }
func (io *MockIO) CanEmbedRes(r Resource) bool             { return false }
func (io *MockIO) ProgressBar(p, total int)                {}
func (io *MockIO) Trace(s string)                          {}
func (io *MockIO) MeasureDims(which int) int               { return 0 }
func (io *MockIO) ScriptOn() bool                          { return false }
func (io *MockIO) ScriptOff()                              {}
func (io *MockIO) ScriptActive() bool                      { return false }
func (io *MockIO) Save(data []byte) bool                   { return true }
func (io *MockIO) Restore()                                {}
func (io *MockIO) HaveStyles() bool                        { return false }
func (io *MockIO) HaveColor() bool                         { return false }
func (io *MockIO) HaveAlign() bool                         { return false }

func TestEngineInitBodyNotStatus(t *testing.T) {
	data := loadStory(t, "../../test/body_not_status/body_not_status.aastory")
	io := NewMockIO()

	e := NewEngine(data, io, 1234, true, false, false)

	// Verify memory allocation
	if len(e.HeapData) != 1000+128 {
		t.Errorf("HeapData size: got %d, want %d", len(e.HeapData), 1000+128)
	}
	if len(e.AuxData) != 500+128 {
		t.Errorf("AuxData size: got %d, want %d", len(e.AuxData), 500+128)
	}
	if len(e.RamData) != 501+128 {
		t.Errorf("RamData size: got %d, want %d", len(e.RamData), 501+128)
	}

	// Verify VM state after reset
	if e.Inst != 1 {
		t.Errorf("Inst: got %d, want 1", e.Inst)
	}
	if e.Cont != 0 {
		t.Errorf("Cont: got %d, want 0", e.Cont)
	}
	if e.Top != 0 {
		t.Errorf("Top: got %d, want 0", e.Top)
	}
	if e.Env != len(e.HeapData) {
		t.Errorf("Env: got %d, want %d", e.Env, len(e.HeapData))
	}
	if e.Cho != len(e.HeapData) {
		t.Errorf("Cho: got %d, want %d", e.Cho, len(e.HeapData))
	}
	if e.Sim != 0xffff {
		t.Errorf("Sim: got %d, want 0xffff", e.Sim)
	}
	if e.Aux != 0 {
		t.Errorf("Aux: got %d, want 0", e.Aux)
	}
	if e.Trl != len(e.AuxData) {
		t.Errorf("Trl: got %d, want %d", e.Trl, len(e.AuxData))
	}
	if e.Nob != get16(e.Init, 0) {
		t.Errorf("Nob: got %d, want %d", e.Nob, get16(e.Init, 0))
	}

	// Verify initial state was captured
	if e.InitState == nil {
		t.Fatal("InitState is nil")
	}
	if len(e.InitState.Data) == 0 {
		t.Fatal("InitState.Data is empty")
	}
	if len(e.InitState.Regs) == 0 {
		t.Fatal("InitState.Regs is empty")
	}

	t.Logf("Engine initialized: heap=%d, aux=%d, ram=%d, nob=%d",
		len(e.HeapData), len(e.AuxData), len(e.RamData), e.Nob)
}

func TestEngineInitGosling(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()

	e := NewEngine(data, io, 1234, true, false, false)

	if len(e.HeapData) == 0 {
		t.Fatal("HeapData not allocated")
	}
	if len(e.AuxData) == 0 {
		t.Fatal("AuxData not allocated")
	}
	if len(e.RamData) == 0 {
		t.Fatal("RamData not allocated")
	}

	t.Logf("Gosling engine: heap=%d, aux=%d, ram=%d, nob=%d, format=%d.%d",
		len(e.HeapData), len(e.AuxData), len(e.RamData), e.Nob,
		e.Head[0], e.Head[1])
}

func TestVMStateCaptureRestore(t *testing.T) {
	data := loadStory(t, "../../test/body_not_status/body_not_status.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Capture state
	state := e.VMCaptureState(e.Inst)

	// Verify we can restore
	e.VMRestoreState(state)

	// Verify state matches
	if e.Inst != 1 {
		t.Errorf("After restore, Inst: got %d, want 1", e.Inst)
	}
	if e.Nob != get16(e.Init, 0) {
		t.Errorf("After restore, Nob: got %d, want %d", e.Nob, get16(e.Init, 0))
	}
}

func TestRLEStateRoundTrip(t *testing.T) {
	data := loadStory(t, "../../test/body_not_status/body_not_status.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Capture and RLE encode
	state1 := e.VMCaptureState(e.Inst)
	encoded := VMRLEncState(e.InitState, state1)

	// RLE decode
	decoded := VMRLDecState(e.InitState, encoded)

	// Restore the decoded state
	e.VMRestoreState(decoded)

	// Verify key fields
	if e.Inst != 1 {
		t.Errorf("After RLE round-trip, Inst: got %d, want 1", e.Inst)
	}
}

func TestDictLookup(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// The dictionary should have entries
	numWords := get16(e.Dict, 0)
	t.Logf("Dictionary has %d words", numWords)
	if numWords == 0 {
		t.Error("Dictionary has no words")
	}

	// Try to decode a word from the dictionary
	entry := 2 // First word entry
	wordLen := int(e.Dict[entry])
	wordAddr := get16(e.Dict, entry+1)
	t.Logf("First word: len=%d, addr=%d", wordLen, wordAddr)

	if wordLen == 0 {
		t.Error("First word has length 0")
	}
	if wordAddr == 0 {
		t.Error("First word has address 0")
	}
}

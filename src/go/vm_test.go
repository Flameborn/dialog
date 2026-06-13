package main

import (
	"testing"
)

func TestFCodeSignExtension(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// fcode reads bytes from e.Code starting at e.Inst, advances e.Inst,
	// and returns an absolute address. Let's verify the raw decode logic.

	// Test 1: Zero returns 0, inst unchanged (only 1 byte consumed)
	e.Code = []byte{0x00, 0xff, 0xff}
	e.Inst = 0
	result := e.fcode()
	if result != 0 {
		t.Errorf("fcode(0x00): got %d, want 0", result)
	}
	if e.Inst != 1 {
		t.Errorf("fcode(0x00) inst: got %d, want 1", e.Inst)
	}

	// Test 2: Small positive offset (v < 0x40): result = inst + v
	// inst starts at 0, v = 5, after reading 1 byte inst = 1, result = 1 + 5 = 6
	e.Code = []byte{0x05, 0xff, 0xff}
	e.Inst = 0
	result = e.fcode()
	if result != 6 {
		t.Errorf("fcode(0x05): got %d, want 6", result)
	}

	// Test 3: 2-byte form, positive offset (bit 13 = 0)
	// 0x40 0x01 => v = (0x00 << 8) | 0x01 = 1, inst = 2, result = 2 + 1 = 3
	e.Code = []byte{0x40, 0x01, 0xff}
	e.Inst = 0
	result = e.fcode()
	if result != 3 {
		t.Errorf("fcode(0x40 0x01): got %d, want 3", result)
	}

	// Test 4: 2-byte form, sign-extended negative offset (bit 13 = 1)
	// 0x60 0x00 => (0x20 << 8) | 0x00 = 0x2000, bit 13 set
	// result = 2 + 0x2000 - 0x4000 = -8190
	e.Code = []byte{0x60, 0x00, 0xff}
	e.Inst = 0
	result = e.fcode()
	expected := 2 + 0x2000 - 0x4000
	if result != expected {
		t.Errorf("fcode(0x60 0x00): got %d, want %d", result, expected)
	}

	// Test 5: 2-byte form, max negative
	// 0x7f 0xff => (0x3f << 8) | 0xff = 0x3fff, bit 13 set
	// result = 2 + 0x3fff - 0x4000 = 1
	e.Code = []byte{0x7f, 0xff, 0xff}
	e.Inst = 0
	result = e.fcode()
	expected = 2 + 0x3fff - 0x4000
	if result != expected {
		t.Errorf("fcode(0x7f 0xff): got %d, want %d", result, expected)
	}

	// Test 6: 2-byte form, bit 13 just below
	// 0x5f 0xff => (0x1f << 8) | 0xff = 0x1fff, bit 13 = 0
	// result = 2 + 0x1fff = 8193
	e.Code = []byte{0x5f, 0xff, 0xff}
	e.Inst = 0
	result = e.fcode()
	expected = 2 + 0x1fff
	if result != expected {
		t.Errorf("fcode(0x5f 0xff): got %d, want %d", result, expected)
	}

	// Test 7: 3-byte form
	// 0x80 0x00 0x01 => v = (0x00 << 16) | (0x00 << 8) | 0x01 = 1
	e.Code = []byte{0x80, 0x00, 0x01}
	e.Inst = 0
	result = e.fcode()
	if result != 1 {
		t.Errorf("fcode(0x80 0x00 0x01): got %d, want 1", result)
	}
	if e.Inst != 3 {
		t.Errorf("fcode 3-byte inst: got %d, want 3", e.Inst)
	}
}

func TestFValue(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Test register reference (v >= 0x80, v < 0xc0)
	e.Code = []byte{0x85} // Register 5
	e.Reg[5] = 0x1234
	e.Inst = 0
	result := e.fvalue()
	if result != 0x1234 {
		t.Errorf("fvalue reg 5: got 0x%x, want 0x1234", result)
	}

	// Test immediate 2-byte value (v < 0x80)
	e.Code = []byte{0x42, 0x56} // (0x42 << 8) | 0x56 = 0x4256
	e.Inst = 0
	result = e.fvalue()
	if result != 0x4256 {
		t.Errorf("fvalue immediate: got 0x%x, want 0x4256", result)
	}

	// Test heap reference (v >= 0xc0)
	// Create a fresh engine with larger heap for this test
	e2 := &Engine{}
	e2.HeapData = make([]uint16, 2000)
	e2.Reg = [64]uint16{}
	e2.Env = 1500 // point env somewhere with room
	e2.Code = []byte{0xc3} // Heap env+4+3 = env+7
	e2.Inst = 0
	e2.HeapData[1500+4+3] = 0x5678
	result = e2.fvalue()
	if result != 0x5678 {
		t.Errorf("fvalue heap: got 0x%x, want 0x5678", result)
	}
}

func TestFWord(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	e.Code = []byte{0xab, 0xcd}
	e.Inst = 0
	result := e.fword()
	if result != 0xabcd {
		t.Errorf("fword: got 0x%x, want 0xabcd", result)
	}
}

func TestFIndex(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Small index (v < 0xc0)
	e.Code = []byte{0x42}
	e.Inst = 0
	result := e.findex()
	if result != 0x42 {
		t.Errorf("findex small: got 0x%x, want 0x42", result)
	}

	// Large index (v >= 0xc0): (0x00 << 8) | 0xab = 0xab
	e.Code = []byte{0xc0, 0xab}
	e.Inst = 0
	result = e.findex()
	if result != 0xab {
		t.Errorf("findex large: got 0x%x, want 0xab", result)
	}
}

func TestDeref(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Unbound variable (heap[addr] = 0) returns itself
	addr := e.Top
	e.Top += 2
	e.HeapData[addr] = 0
	v := 0x8000 | addr
	result := e.deref(v)
	if result != v {
		t.Errorf("deref unbound: got 0x%x, want 0x%x", result, v)
	}

	// Bound variable follows chain to value
	addr2 := e.Top
	e.Top += 2
	e.HeapData[addr2] = 0x1234
	v2 := 0x8000 | addr2
	result = e.deref(v2)
	if result != 0x1234 {
		t.Errorf("deref bound: got 0x%x, want 0x1234", result)
	}

	// Non-variable returns itself
	result = e.deref(0x4005)
	if result != 0x4005 {
		t.Errorf("deref non-var: got 0x%x, want 0x4005", result)
	}
}

func TestUnify(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Equal constants
	if !e.unify(0x4005, 0x4005) {
		t.Error("unify equal constants should succeed")
	}

	// Different constants
	if e.unify(0x4005, 0x4006) {
		t.Error("unify different constants should fail")
	}

	// Unbound variable with constant
	addr := e.Top
	e.Top += 2
	e.HeapData[addr] = 0
	v := 0x8000 | addr
	if !e.unify(v, 0x4005) {
		t.Error("unify unbound with constant should succeed")
	}
	if e.HeapData[addr] != 0x4005 {
		t.Errorf("after unify, heap[addr] = 0x%x, want 0x4005", e.HeapData[addr])
	}

	// Two unbound variables
	addr1 := e.Top
	e.Top++
	e.HeapData[addr1] = 0
	v1 := 0x8000 | addr1
	addr2 := e.Top
	e.Top++
	e.HeapData[addr2] = 0
	v2 := 0x8000 | addr2
	if !e.unify(v1, v2) {
		t.Error("unify two unbound should succeed")
	}
	// Lower-addressed var points to higher-addressed
	if addr1 < addr2 {
		if e.HeapData[addr2] != uint16(v1) {
			t.Errorf("heap[%d] = 0x%x, want 0x%x", addr2, e.HeapData[addr2], v1)
		}
	} else {
		if e.HeapData[addr1] != uint16(v2) {
			t.Errorf("heap[%d] = 0x%x, want 0x%x", addr1, e.HeapData[addr1], v2)
		}
	}
}

func TestCompatRand(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Deterministic sequence
	e.RandomState = 1234
	r1 := e.compatRand()
	r2 := e.compatRand()
	r3 := e.compatRand()

	t.Logf("CompatRand: %d, %d, %d", r1, r2, r3)

	if r1 > 0x7fff || r2 > 0x7fff || r3 > 0x7fff {
		t.Error("CompatRand out of range")
	}

	// Determinism
	e.RandomState = 1234
	r1b := e.compatRand()
	r2b := e.compatRand()
	r3b := e.compatRand()
	if r1 != r1b || r2 != r2b || r3 != r3b {
		t.Error("CompatRand not deterministic")
	}
}

func TestCreatePair(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	addr := e.createPair(0x4005, 0x3f00)
	if addr&0xe000 != 0xc000 {
		t.Errorf("createPair tag: got 0x%x, want 0xc000 tag", addr)
	}
	if e.HeapData[addr&0x1fff] != 0x4005 {
		t.Errorf("createPair head: got 0x%x, want 0x4005", e.HeapData[addr&0x1fff])
	}
	if e.HeapData[(addr&0x1fff)+1] != 0x3f00 {
		t.Errorf("createPair tail: got 0x%x, want 0x3f00", e.HeapData[(addr&0x1fff)+1])
	}
}

func TestStore(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Store to register (dest < 0x40)
	e.store(5, 0x1234)
	if e.Reg[5] != 0x1234 {
		t.Errorf("store reg: got 0x%x, want 0x1234", e.Reg[5])
	}

	// Store to unbound variable (dest >= 0x80) via unify
	vaddr := e.Top
	e.Top++
	e.HeapData[vaddr] = 0
	e.Reg[10] = uint16(0x8000 | vaddr)
	e.store(0x8a, 0x5678)
	derefResult := e.deref(int(e.Reg[10]))
	if derefResult != 0x5678 {
		t.Errorf("store unify reg: got 0x%x, want 0x5678", derefResult)
	}
}

func TestFieldAddr(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")
	io := NewMockIO()
	e := NewEngine(data, io, 1234, true, false, false)

	// Object 1 should have its RAM address set during init
	if e.Nob < 1 {
		t.Skip("No objects in this story")
	}

	addr := e.fieldaddr(0, 1) // parent field of object 1
	t.Logf("fieldaddr(0, 1) = %d (RAM base = %d)", addr, e.RamData[1])

	if addr < 0 || addr >= len(e.RamData) {
		t.Errorf("fieldaddr out of range: %d", addr)
	}
}

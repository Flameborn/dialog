package main

// Engine initialization and state management

func NewEngine(fileArray []byte, io IO, seed uint32, quit, toparea, inlinearea bool) *Engine {
	e := &Engine{}

	if get4(fileArray, 0) != "FORM" || get4(fileArray, 8) != "AAVM" {
		panic("Not an aastory file")
	}

	e.Head = findChunk(fileArray, "HEAD")
	e.Code = findChunk(fileArray, "CODE")
	e.Dict = findChunk(fileArray, "DICT")
	e.Init = findChunk(fileArray, "INIT")
	e.Lang = findChunk(fileArray, "LANG")
	e.Maps = findChunk(fileArray, "MAPS")
	e.Tags = findChunk(fileArray, "TAGS")
	e.Writ = findChunk(fileArray, "WRIT")
	e.Look = findChunk(fileArray, "LOOK")
	e.Meta = findChunk(fileArray, "META")
	e.Urls = findChunk(fileArray, "URLS")
	e.Files = findFiles(fileArray)

	if e.Head == nil || e.Code == nil || e.Dict == nil || e.Init == nil || e.Lang == nil || e.Maps == nil || e.Writ == nil || e.Look == nil {
		panic("Missing required chunks")
	}

	e.RandomSeed = seed
	e.StrShift = 0
	e.ExtChars = 0
	e.EscBits = 7
	e.EscBoundary = 0
	e.IO = io
	e.HaveQuit = quit
	e.HaveTop = toparea
	e.HaveInline = inlinearea

	if e.Head[0] > byte(VerMajor) || (e.Head[0] == byte(VerMajor) && e.Head[1] > byte(VerMinor)) {
		panic("Unsupported aastory file format version")
	}
	if e.Head[2] != 2 {
		panic("Unsupported word size")
	}

	e.HeapData = make([]uint16, get16(e.Head, 16)+128)
	e.AuxData = make([]uint16, get16(e.Head, 18)+128)
	e.RamData = make([]uint16, get16(e.Head, 20)+128)
	e.StrShift = int(e.Head[3])
	e.ExtChars = get16(e.Lang, 2)

	if e.Head[0] > 0 || e.Head[1] >= 4 {
		e.EscBoundary = int(e.Lang[e.ExtChars]) - 32
		if e.EscBoundary < 0 {
			e.EscBoundary = 0
		}
		i := e.EscBoundary + get16(e.Dict, 0) - 1
		e.EscBits = 0
		for i > 0 {
			i >>= 1
			e.EscBits++
		}
	}

	stopptr := get16(e.Lang, 6)
	stopend := stopptr
	for e.Lang[stopend] != 0 {
		stopend++
	}
	e.StopChars = make([]byte, stopend-stopptr)
	copy(e.StopChars, e.Lang[stopptr:stopend])

	if e.Head[0] > 0 || e.Head[1] >= 4 {
		stopptr = stopend + 1
		stopend = stopptr
		for e.Lang[stopend] != 0 {
			stopend++
		}
		e.NospBefore = make([]byte, stopend-stopptr)
		copy(e.NospBefore, e.Lang[stopptr:stopend])

		stopptr = stopend + 1
		stopend = stopptr
		for e.Lang[stopend] != 0 {
			stopend++
		}
		e.NospAfter = make([]byte, stopend-stopptr)
		copy(e.NospAfter, e.Lang[stopptr:stopend])
	}

	e.VMReinit()
	e.VMReset(0, true)
	e.InitState = e.VMCaptureState(1)
	io.Reset()

	return e
}

func (e *Engine) VMReinit() {
	e.Nob = get16(e.Init, 0)
	e.Ltb = get16(e.Init, 2)
	e.Ltt = get16(e.Init, 4)
	for i := range e.HeapData {
		e.HeapData[i] = 0x3f3f
	}
	for i := range e.AuxData {
		e.AuxData[i] = 0x3f3f
	}
	for i := (len(e.Init) - 6) >> 1; i < len(e.RamData); i++ {
		e.RamData[i] = 0x3f3f
	}
	for i := 6; i < len(e.Init); i += 2 {
		e.RamData[(i-6)>>1] = uint16(get16(e.Init, i))
	}
}

func (e *Engine) VMReset(arg0 int, clearUndo bool) {
	e.Reg[0] = uint16(arg0)
	for i := 1; i < 64; i++ {
		e.Reg[i] = 0
	}
	e.Inst = 1
	e.Cont = 0
	e.Top = 0
	e.Env = len(e.HeapData)
	e.Cho = len(e.HeapData)
	e.Sim = 0xffff
	e.Aux = 0
	e.Trl = len(e.AuxData)
	e.Sta = 0
	e.Stc = 0
	e.Cwl = 0
	e.Spc = SPLine
	e.Divs = []int{}
	e.Upper = false
	e.InStatus = false
	e.NSpan = 0
	e.NLink = 0
	if clearUndo {
		e.UndoData = nil
		e.PrunedUndo = false
	}
	if e.RandomSeed != 0 {
		e.RandomState = e.RandomSeed
	}
}

func (e *Engine) VMCaptureState(newInst int) *EncodedState {
	nword := 3 + len(e.RamData) + len(e.AuxData) + len(e.HeapData)
	data := make([]byte, nword*2)
	regs := make([]byte, 128+26+2+len(e.Divs)*2)

	j := 0
	j = put16(data, j, e.Nob)
	j = put16(data, j, e.Ltb)
	j = put16(data, j, e.Ltt)
	for i := 0; i < len(e.RamData); i++ {
		if i < e.Ltt {
			j = put16(data, j, int(e.RamData[i]))
		} else {
			j = put16(data, j, 0x3f3f)
		}
	}
	for i := 0; i < len(e.AuxData); i++ {
		if i < e.Aux || i >= e.Trl {
			j = put16(data, j, int(e.AuxData[i]))
		} else {
			j = put16(data, j, 0x3f3f)
		}
	}
	for i := 0; i < len(e.HeapData); i++ {
		if i < e.Top || i >= e.Env || i >= e.Cho {
			j = put16(data, j, int(e.HeapData[i]))
		} else {
			j = put16(data, j, 0x3f3f)
		}
	}

	j = 0
	for i := 0; i < 64; i++ {
		j = put16(regs, j, int(e.Reg[i]))
	}
	j = put32(regs, j, newInst)
	j = put32(regs, j, e.Cont)
	j = put16(regs, j, e.Top)
	j = put16(regs, j, e.Env)
	j = put16(regs, j, e.Cho)
	j = put16(regs, j, e.Sim)
	j = put16(regs, j, e.Aux)
	j = put16(regs, j, e.Trl)
	j = put16(regs, j, e.Sta)
	j = put16(regs, j, e.Stc)
	regs[j] = byte(e.Cwl)
	j++
	regs[j] = byte(e.Spc)
	j++
	j = put16(regs, j, len(e.Divs))
	for i := 0; i < len(e.Divs); i++ {
		j = put16(regs, j, e.Divs[i])
	}

	return &EncodedState{Data: data, Regs: regs}
}

func (e *Engine) VMClearDivs() {
	e.IO.LeaveAll()
	e.InStatus = false
	e.NSpan = 0
	e.NLink = 0
	e.Divs = []int{}
}

func (e *Engine) VMRestoreState(state *EncodedState) {
	data := state.Data
	regs := state.Regs
	j := 0

	e.Nob = get16(data, j)
	j += 2
	e.Ltb = get16(data, j)
	j += 2
	e.Ltt = get16(data, j)
	j += 2
	for i := 0; i < len(e.RamData); i++ {
		e.RamData[i] = uint16(get16(data, j))
		j += 2
	}
	for i := 0; i < len(e.AuxData); i++ {
		e.AuxData[i] = uint16(get16(data, j))
		j += 2
	}
	for i := 0; i < len(e.HeapData); i++ {
		e.HeapData[i] = uint16(get16(data, j))
		j += 2
	}

	j = 0
	for i := 0; i < 64; i++ {
		e.Reg[i] = uint16(get16(regs, j))
		j += 2
	}
	e.Inst = get32(regs, j)
	j += 4
	e.Cont = get32(regs, j)
	j += 4
	e.Top = get16(regs, j)
	j += 2
	e.Env = get16(regs, j)
	j += 2
	e.Cho = get16(regs, j)
	j += 2
	e.Sim = get16(regs, j)
	j += 2
	e.Aux = get16(regs, j)
	j += 2
	e.Trl = get16(regs, j)
	j += 2
	e.Sta = get16(regs, j)
	j += 2
	e.Stc = get16(regs, j)
	j += 2
	e.Cwl = int(regs[j])
	j++
	e.Spc = int(regs[j])
	j++
	ndiv := get16(regs, j)
	j += 2
	for i := 0; i < ndiv; i++ {
		d := get16(regs, j)
		j += 2
		e.Divs = append(e.Divs, d)
		e.IO.EnterDiv(d)
	}
}

// RLE encoding/decoding for undo/save

func VMRLEncState(reference, state *EncodedState) *EncodedState {
	bytes := 0
	nz := 0

	for i := 0; i < len(reference.Data); i++ {
		if reference.Data[i]^state.Data[i] != 0 {
			bytes++
			nz = 0
		} else {
			if nz != 0 && nz < 0x100 {
				nz++
			} else {
				bytes += 2
				nz = 1
			}
		}
	}

	encoded := make([]byte, bytes)
	j := 0
	nz = 0
	for i := 0; i < len(reference.Data); i++ {
		diff := reference.Data[i] ^ state.Data[i]
		if diff != 0 {
			encoded[j] = diff
			j++
		} else {
			encoded[j] = 0
			j++
			nz = 1
			for nz < 0x100 && i+nz < len(reference.Data) && reference.Data[i+nz]^state.Data[i+nz] == 0 {
				nz++
			}
			encoded[j] = byte(nz - 1)
			j++
			i += nz - 1
		}
	}

	return &EncodedState{Data: encoded, Regs: state.Regs}
}

func VMRLDecState(reference *EncodedState, encoded *EncodedState) *EncodedState {
	array := make([]byte, len(reference.Data))
	j := 0

	for i := 0; i < len(encoded.Data); i++ {
		diff := encoded.Data[i]
		if diff != 0 {
			array[j] = reference.Data[j] ^ diff
			j++
		} else {
			i++
			nz := int(encoded.Data[i]) + 1
			for k := 0; k < nz; k++ {
				array[j] = reference.Data[j]
				j++
			}
		}
	}

	return &EncodedState{Data: array, Regs: encoded.Regs}
}

func VMWrapSavefile(e *Engine, encoded *EncodedState) []byte {
	makechunk := func(tag string, array []byte) []byte {
		size := (len(array) + 1) & ^1
		result := make([]byte, 8+size)
		put4(result, 0, tag)
		put32(result, 4, len(array))
		copy(result[8:], array)
		return result
	}

	head := makechunk("HEAD", e.Head)
	data := makechunk("DATA", encoded.Data)
	regs := makechunk("REGS", encoded.Regs)
	size := 4 + len(head) + len(data) + len(regs)
	result := make([]byte, 8+size)

	put4(result, 0, "FORM")
	put32(result, 4, size)
	put4(result, 8, "AASV")
	copy(result[12:], head)
	copy(result[12+len(head):], data)
	copy(result[12+len(head)+len(data):], regs)

	return result
}

func VMUnwrapSavefile(e *Engine, filedata []byte) *EncodedState {
	if get4(filedata, 0) != "FORM" || get4(filedata, 8) != "AASV" {
		e.IO.Print("Not an aasave file!")
		e.IO.Line()
		return nil
	}
	head := findChunk(filedata, "HEAD")
	data := findChunk(filedata, "DATA")
	regs := findChunk(filedata, "REGS")
	if head == nil || data == nil || regs == nil {
		e.IO.Print("Incomplete aasave file!")
		e.IO.Line()
		return nil
	}
	i := 0
	for i < len(head) && i < len(e.Head) {
		if head[i] != e.Head[i] {
			break
		}
		i++
	}
	if i != len(head) || i != len(e.Head) {
		e.IO.Print("This savefile is from another story (or another version of the present story).")
		e.IO.Line()
		return nil
	}
	return &EncodedState{Data: data, Regs: regs}
}

package main

import "fmt"

// Operand decoders - these must be byte-exact matches with the JS engine

// fvalue reads a value operand from the code stream
func (e *Engine) fvalue() int {
	v := int(e.Code[e.Inst])
	e.Inst++
	if v >= 0xc0 {
		return int(e.HeapData[e.Env+4+(v&0x3f)])
	} else if v >= 0x80 {
		return int(e.Reg[v&0x3f])
	} else {
		r := (v << 8) | int(e.Code[e.Inst])
		e.Inst++
		return r
	}
}

// findex reads an index operand from the code stream
func (e *Engine) findex() int {
	v := int(e.Code[e.Inst])
	e.Inst++
	if v >= 0xc0 {
		r := ((v & 0x3f) << 8) | int(e.Code[e.Inst])
		e.Inst++
		return r
	} else {
		return v
	}
}

// fcode reads a code address operand from the code stream
// CRITICAL: bit 13 sign extension for 2-byte form
func (e *Engine) fcode() int {
	v := int(e.Code[e.Inst])
	e.Inst++
	if v == 0 {
		return 0
	} else if v < 0x40 {
		return e.Inst + v
	} else if v < 0x80 {
		v = ((v & 0x3f) << 8) | int(e.Code[e.Inst])
		e.Inst++
		if v&0x2000 != 0 {
			return e.Inst + v - 0x4000
		} else {
			return e.Inst + v
		}
	} else {
		v = ((v & 0x7f) << 16) | (int(e.Code[e.Inst]) << 8)
		e.Inst++
		v |= int(e.Code[e.Inst])
		e.Inst++
		return v
	}
}

// fstring reads a string address operand from the code stream
func (e *Engine) fstring() int {
	v := int(e.Code[e.Inst])
	e.Inst++
	if v >= 0xc0 {
		v = ((v & 0x3f) << 16) | (int(e.Code[e.Inst]) << 8)
		e.Inst++
		v |= int(e.Code[e.Inst])
		e.Inst++
		return v << e.StrShift
	} else if v >= 0x80 {
		v = ((v & 0x3f) << 8) | int(e.Code[e.Inst])
		e.Inst++
		return v << e.StrShift
	} else {
		return v << 1
	}
}

// fword reads a 16-bit word operand from the code stream
func (e *Engine) fword() int {
	v := int(e.Code[e.Inst])
	e.Inst++
	r := (v << 8) | int(e.Code[e.Inst])
	e.Inst++
	return r
}

// --- Core VM operations ---

// deref follows unbound variable chains through the heap
func (e *Engine) deref(v int) int {
	for (v & 0xe000) == 0x8000 {
		t := int(e.HeapData[v&0x1fff])
		if t == 0 {
			return v
		}
		v = t
	}
	return v
}

// fail sets inst to the choice point's next pointer
func (e *Engine) fail() {
	e.Inst = int(e.HeapData[e.Cho+4])<<16 | int(e.HeapData[e.Cho+5])
}

// unify attempts to make a and b equal, using trail for backtracking
func (e *Engine) unify(a, b int) bool {
	for {
		a = e.deref(a)
		b = e.deref(b)
		if (a&0xe000) == 0x8000 && (b&0xe000) == 0x8000 {
			if e.Trl <= e.Aux {
				panic(AuxFull)
			}
			if a < b {
				e.Trl--
				e.AuxData[e.Trl] = uint16(b & 0x1fff)
				e.HeapData[b&0x1fff] = uint16(a)
			} else if a > b {
				e.Trl--
				e.AuxData[e.Trl] = uint16(a & 0x1fff)
				e.HeapData[a&0x1fff] = uint16(b)
			}
			return true
		} else if (a & 0xe000) == 0x8000 {
			if e.Trl <= e.Aux {
				panic(AuxFull)
			}
			e.Trl--
			e.AuxData[e.Trl] = uint16(a & 0x1fff)
			e.HeapData[a&0x1fff] = uint16(b)
			return true
		} else if (b & 0xe000) == 0x8000 {
			if e.Trl <= e.Aux {
				panic(AuxFull)
			}
			e.Trl--
			e.AuxData[e.Trl] = uint16(b & 0x1fff)
			e.HeapData[b&0x1fff] = uint16(a)
			return true
		} else if a >= 0xe000 && b >= 0xe000 {
			a = int(e.HeapData[a&0x1fff])
			b = int(e.HeapData[b&0x1fff])
		} else if a >= 0xe000 {
			a = int(e.HeapData[a&0x1fff])
		} else if b >= 0xe000 {
			b = int(e.HeapData[b&0x1fff])
		} else if a == b {
			return true
		} else if a >= 0xc000 && b >= 0xc000 {
			if !e.unify(a-0x4000, b-0x4000) {
				return false
			}
			a = a - 0x3fff
			b = b - 0x3fff
		} else {
			return false
		}
	}
}

// would_unify checks if a and b could unify without actually binding
func (e *Engine) would_unify(a, b int) bool {
	for {
		a = e.deref(a)
		b = e.deref(b)
		if (a&0xe000) == 0x8000 || (b&0xe000) == 0x8000 {
			return true
		} else if a >= 0xe000 && b >= 0xe000 {
			a = int(e.HeapData[a&0x1fff])
			b = int(e.HeapData[b&0x1fff])
		} else if a >= 0xe000 {
			a = int(e.HeapData[a&0x1fff])
		} else if b >= 0xe000 {
			b = int(e.HeapData[b&0x1fff])
		} else if a == b {
			return true
		} else if a >= 0xc000 && b >= 0xc000 {
			if !e.would_unify(a-0x4000, b-0x4000) {
				return false
			}
			a = a - 0x3fff
			b = b - 0x3fff
		} else {
			return false
		}
	}
}

// destvalue reads a destination value (for make_pair)
func (e *Engine) destvalue(dest int) int {
	if dest&0x40 != 0 {
		return int(e.HeapData[e.Env+4+(dest&0x3f)])
	}
	return int(e.Reg[dest&0x3f])
}

// store writes a value to a destination, using unify for bound destinations
func (e *Engine) store(dest, src int) {
	if dest >= 0xc0 {
		if !e.unify(int(e.HeapData[e.Env+4+(dest&0x3f)]), src) {
			e.fail()
		}
	} else if dest >= 0x80 {
		if !e.unify(int(e.Reg[dest&0x3f]), src) {
			e.fail()
		}
	} else if dest >= 0x40 {
		e.HeapData[e.Env+4+(dest&0x3f)] = uint16(src)
	} else {
		e.Reg[dest] = uint16(src)
	}
}

// push_cho creates a choice point
func (e *Engine) pushCho(narg, next int) {
	addr := e.Env
	if e.Cho < e.Env {
		addr = e.Cho
	}
	addr -= 9 + narg
	if addr < e.Top {
		panic(HeapFull)
	}
	e.HeapData[addr] = uint16(e.Env)
	e.HeapData[addr+1] = uint16(e.Sim)
	e.HeapData[addr+2] = uint16(e.Cont >> 16)
	e.HeapData[addr+3] = uint16(e.Cont & 0xffff)
	e.HeapData[addr+4] = uint16(next >> 16)
	e.HeapData[addr+5] = uint16(next & 0xffff)
	e.HeapData[addr+6] = uint16(e.Cho)
	e.HeapData[addr+7] = uint16(e.Top)
	e.HeapData[addr+8] = uint16(e.Trl)
	for i := 0; i < narg; i++ {
		e.HeapData[addr+9+i] = e.Reg[i]
	}
	e.Cho = addr
}

// push_aux pushes a value onto the auxiliary stack
func (e *Engine) pushAux(v int) {
	v = e.deref(v)
	if v >= 0xe000 {
		e.pushAux(int(e.HeapData[(v&0x1fff)+1]))
		e.pushAux(int(e.HeapData[(v&0x1fff)+0]))
		v = 0x8100
	} else if v >= 0xc000 {
		count := 0
		for {
			e.pushAux(v - 0x4000)
			count++
			v = e.deref(v - 0x3fff)
			if v == 0x3f00 {
				v = 0xc000 | count
				break
			} else if (v & 0xe000) != 0xc000 {
				e.pushAux(v)
				v = 0xe000 | count
				break
			}
		}
	} else if v >= 0x8000 {
		v = 0x8000
	}
	if e.Aux >= e.Trl {
		panic(AuxFull)
	}
	e.AuxData[e.Aux] = uint16(v)
	e.Aux++
}

// pop_aux pops a value from the auxiliary stack
func (e *Engine) popAux() int {
	e.Aux--
	v := int(e.AuxData[e.Aux])

	if v == 0x8000 {
		addr := e.Top
		e.Top++
		if e.Top > e.Env || e.Top > e.Cho {
			panic(HeapFull)
		}
		e.HeapData[addr] = 0
		v = 0x8000 | addr
	} else if v == 0x8100 {
		addr := e.Top
		e.Top += 2
		if e.Top > e.Env || e.Top > e.Cho {
			panic(HeapFull)
		}
		e.HeapData[addr] = uint16(e.popAux())
		e.HeapData[addr+1] = uint16(e.popAux())
		v = 0xe000 | addr
	} else if v >= 0xc000 {
		count := v & 0x1fff
		if v&0x2000 != 0 {
			v = e.popAux()
		} else {
			v = 0x3f00
		}
		for count > 0 {
			count--
			addr := e.Top
			e.Top += 2
			if e.Top > e.Env || e.Top > e.Cho {
				panic(HeapFull)
			}
			e.HeapData[addr] = uint16(e.popAux())
			e.HeapData[addr+1] = uint16(v)
			v = 0xc000 | addr
		}
	}
	return v
}

// pop_aux_list pops a list from the auxiliary stack
func (e *Engine) popAuxList() int {
	list := 0x3f00
	for {
		v := e.popAux()
		if v == 0 {
			break
		}
		addr := e.Top
		e.Top += 2
		if e.Top > e.Env || e.Top > e.Cho {
			panic(HeapFull)
		}
		e.HeapData[addr] = uint16(v)
		e.HeapData[addr+1] = uint16(list)
		list = 0xc000 | addr
	}
	return list
}

// fieldaddr returns the RAM address of a field for an object
func (e *Engine) fieldaddr(field, obj int) int {
	obj = e.deref(obj)
	if obj > e.Nob {
		panic(ExpectObj)
	}
	return int(e.RamData[obj]) + field
}

// readfield reads a field from an object
func (e *Engine) readfield(field, obj int) int {
	obj = e.deref(obj)
	if obj > e.Nob {
		return 0
	}
	return int(e.RamData[int(e.RamData[obj])+field])
}

// unlink removes an object from a linked list
func (e *Engine) unlink(rootAddr, next, key int) {
	if key == 0 || key >= 0x2000 {
		return
	}
	tail := int(e.RamData[e.fieldaddr(next, key)])
	addr := rootAddr
	for e.RamData[addr] != 0 {
		if int(e.RamData[addr]) == key {
			e.RamData[addr] = uint16(tail)
			return
		}
		addr = e.fieldaddr(next, int(e.RamData[addr]))
	}
}

// --- LTS (Long-Term Storage) operations ---

func (e *Engine) popLTS() int {
	e.Tmp--
	v := int(e.RamData[e.Tmp])

	if v == 0x8100 {
		addr := e.Top
		e.Top += 2
		if e.Top > e.Env || e.Top > e.Cho {
			panic(HeapFull)
		}
		e.HeapData[addr] = uint16(e.popLTS())
		e.HeapData[addr+1] = uint16(e.popLTS())
		v = 0xe000 | addr
	} else if v >= 0xc000 {
		count := v & 0x1fff
		if v&0x2000 != 0 {
			v = e.popLTS()
		} else {
			v = 0x3f00
		}
		for count > 0 {
			count--
			addr := e.Top
			e.Top += 2
			if e.Top > e.Env || e.Top > e.Cho {
				panic(HeapFull)
			}
			e.HeapData[addr] = uint16(e.popLTS())
			e.HeapData[addr+1] = uint16(v)
			v = 0xc000 | addr
		}
	}
	return v
}

func (e *Engine) pushLTS(v int) {
	v = e.deref(v)
	if v >= 0xe000 {
		e.pushLTS(int(e.HeapData[(v&0x1fff)+1]))
		e.pushLTS(int(e.HeapData[(v&0x1fff)+0]))
		v = 0x8100
	} else if v >= 0xc000 {
		count := 0
		for {
			e.pushLTS(v - 0x4000)
			count++
			v = e.deref(v - 0x3fff)
			if v == 0x3f00 {
				v = 0xc000 | count
				break
			} else if (v & 0xe000) != 0xc000 {
				e.pushLTS(v)
				v = 0xe000 | count
				break
			}
		}
	} else if v >= 0x8000 {
		panic(ExpectBound)
	}
	if e.Tmp > len(e.RamData) {
		panic(LTSFull)
	}
	e.RamData[e.Tmp] = uint16(v)
	e.Tmp++
}

func (e *Engine) clearLTS(addr int) {
	v := int(e.RamData[addr])
	if v&0x8000 != 0 {
		e.RamData[addr] = 0
		v &= 0x7fff
		size := int(e.RamData[v])
		for i := v; i < e.Ltt-size; i++ {
			e.RamData[i] = e.RamData[i+size]
		}
		e.Ltt -= size
		for v < e.Ltt {
			e.RamData[int(e.RamData[v+1])] -= uint16(size)
			v += int(e.RamData[v])
		}
	}
}

func (e *Engine) getLTS(v int) int {
	if v&0x8000 != 0 {
		e.Tmp = v & 0x7fff
		e.Tmp += int(e.RamData[e.Tmp])
		return e.popLTS()
	}
	return v
}

func (e *Engine) putLTS(addr, v int) {
	e.clearLTS(addr)
	v = e.deref(v)
	if v < 0x8000 {
		e.RamData[addr] = uint16(v)
	} else {
		e.Tmp = e.Ltt + 2
		if e.Tmp > len(e.RamData) {
			panic(LTSFull)
		}
		e.pushLTS(v)
		e.RamData[addr] = uint16(0x8000 | e.Ltt)
		e.RamData[e.Ltt+0] = uint16(e.Tmp - e.Ltt)
		e.RamData[e.Ltt+1] = uint16(addr)
		e.Ltt = e.Tmp
	}
}

// --- Pair operations ---

func (e *Engine) createPair(head, tail int) int {
	addr := e.Top
	e.Top += 2
	if e.Top > e.Env || e.Top > e.Cho {
		panic(HeapFull)
	}
	e.HeapData[addr] = uint16(head)
	e.HeapData[addr+1] = uint16(tail)
	return addr | 0xc000
}

func (e *Engine) makePairSub(literal bool, arg, addr int) {
	if literal {
		e.HeapData[addr] = uint16(arg)
	} else if arg&0x80 != 0 {
		e.HeapData[addr] = uint16(e.destvalue(arg))
	} else {
		e.HeapData[addr] = 0
		e.store(arg, 0x8000|addr)
	}
}

func (e *Engine) makePair(a1val bool, a1, a2, a3 int) {
	if a3&0x80 != 0 {
		a3 = e.deref(e.destvalue(a3))
		if (a3 & 0xe000) == 0xc000 {
			if a1val {
				if !e.unify(a1, a3-0x4000) {
					e.fail()
				}
			} else {
				e.store(a1, a3-0x4000)
			}
			e.store(a2, a3-0x3fff)
		} else if (a3 & 0xe000) == 0x8000 {
			addr := e.Top
			e.Top += 2
			if e.Top > e.Env || e.Top > e.Cho {
				panic(HeapFull)
			}
			e.makePairSub(a1val, a1, addr)
			e.makePairSub(false, a2, addr+1)
			e.unify(a3, 0xc000|addr)
		} else {
			e.fail()
		}
	} else {
		addr := e.Top
		e.Top += 2
		if e.Top > e.Env || e.Top > e.Cho {
			panic(HeapFull)
		}
		e.makePairSub(a1val, a1, addr)
		e.makePairSub(false, a2, addr+1)
		e.store(a3, 0xc000|addr)
	}
}

// --- Value-to-string conversion ---

func (e *Engine) val2str(v int) string {
	v = e.deref(v)
	if v >= 0xe000 {
		str := ""
		for i := 0; i < 2; i++ {
			x := int(e.HeapData[(v&0x1fff)+i])
			if x >= 0x3f00 {
				for x >= 0xc000 {
					str += e.val2str(int(e.HeapData[x&0x1fff]))
					x = int(e.HeapData[(x&0x1fff)+1])
				}
			} else {
				str += e.val2str(x)
			}
		}
		return str
	} else if v >= 0xc000 {
		needsp := false
		e.Upper = false
		str := "["
		for (v & 0xe000) == 0xc000 {
			if needsp {
				str += " "
			}
			str += e.val2str(v - 0x4000)
			needsp = true
			v = e.deref(v - 0x3fff)
		}
		if v == 0x3f00 {
			str += "]"
		} else {
			str += " | " + e.val2str(v) + "]"
		}
		return str
	} else if v >= 0x8000 {
		e.Upper = false
		return "$"
	} else if v >= 0x4000 {
		e.Upper = false
		return fmt.Sprintf("%d", v&0x3fff)
	} else if v >= 0x3f00 {
		e.Upper = false
		return "[]"
	} else if v >= 0x3e00 {
		return e.DecodeChar(v & 0xff)
	} else if v >= 0x2000 {
		entry := 2 + (v&0x1fff)*3
		l := int(e.Dict[entry])
		addr := get16(e.Dict, entry+1)
		str := ""
		for i := 0; i < l; i++ {
			str += e.DecodeChar(int(e.Dict[addr+i]))
		}
		return str
	} else if v != 0 {
		e.Upper = false
		str := "#"
		if e.Tags != nil {
			addr := get16(e.Tags, v*2)
			for {
				ch := int(e.Tags[addr])
				addr++
				if ch == 0 {
					break
				}
				str += e.DecodeChar(ch)
			}
		}
		return str
	}
	return ""
}

// --- Random number generator ---

func (e *Engine) compatRand() uint32 {
	high := (e.RandomState >> 16) & 0xffff
	low := e.RandomState & 0xffff
	newhigh := ((0x15a * low) + (0x4e35 * high)) & 0xffff
	e.RandomState = ((newhigh << 16) + (0x4e35 * low) + 1) & 0xffffffff
	return (e.RandomState >> 16) & 0x7fff
}

// --- Word map lookup ---

func (e *Engine) wordMap(mapnum, v int) bool {
	mapOff := get16(e.Maps, 2+mapnum*2)
	start := 0
	end := get16(e.Maps, mapOff)
	for start < end {
		mid := (start + end) >> 1
		midval := get16(e.Maps, mapOff+2+mid*4)
		if midval == v {
			ptr := get16(e.Maps, mapOff+4+mid*4)
			if ptr == 0 {
				return false
			} else if ptr&0xe000 != 0 {
				if e.Aux >= e.Trl {
					panic(AuxFull)
				}
				e.AuxData[e.Aux] = uint16(ptr & 0x1fff)
				e.Aux++
				return true
			} else {
				for {
					o := int(e.Maps[ptr])
					ptr++
					if o == 0 {
						break
					}
					if o >= 0xe0 {
						o = ((o & 0x1f) << 8) | int(e.Maps[ptr])
						ptr++
					}
					if e.Aux >= e.Trl {
						panic(AuxFull)
					}
					e.AuxData[e.Aux] = uint16(o)
					e.Aux++
				}
				return true
			}
		} else if midval > v {
			end = mid
		} else {
			start = mid + 1
		}
	}
	return true
}

// --- PrependChars and WordsToCharlist ---

func (e *Engine) prependChars(v, list int) int {
	entry := 2 + (v&0x1fff)*3
	l := int(e.Dict[entry])
	addr := get16(e.Dict, entry+1)
	for i := l - 1; i >= 0; i-- {
		ch := int(e.Dict[addr+i])
		if ch >= '0' && ch <= '9' {
			ch += 0x4000 - '0'
		} else {
			ch |= 0x3e00
		}
		list = e.createPair(ch, list)
	}
	return list
}

func containsByte(slice []byte, val int) bool {
	for _, b := range slice {
		if int(b) == val {
			return true
		}
	}
	return false
}

func (e *Engine) wordsToCharlist(list int) []int {
	var buf []int
	for {
		v := e.deref(int(e.HeapData[(list&0x1fff)+0]))
		if v >= 0xe000 {
			part1 := int(e.HeapData[(v&0x1fff)+0])
			if part1 >= 0x8000 {
				sub := e.wordsToCharlist(part1)
				if sub == nil {
					return nil
				}
				buf = append(buf, sub...)
			} else {
				sub := e.wordsToCharlist(v)
				if sub == nil {
					return nil
				}
				buf = append(buf, sub...)
			}
		} else if v >= 0x4000 && v < 0x8000 {
			str := fmt.Sprintf("%d", v&0x3fff)
			for _, c := range str {
				buf = append(buf, int(c))
			}
		} else if v >= 0x3e00 && v < 0x3f00 {
			ch := v & 0xff
			if ch <= 0x20 {
				return nil
			}
			if containsByte(e.StopChars, ch) {
				return nil
			}
			buf = append(buf, ch)
		} else if v >= 0x2000 && v < 0x3e00 {
			entry := 2 + (v&0x1fff)*3
			l := int(e.Dict[entry])
			addr := get16(e.Dict, entry+1)
			for i := 0; i < l; i++ {
				buf = append(buf, int(e.Dict[addr+i]))
			}
		} else {
			return nil
		}
		list = e.deref(int(e.HeapData[(list&0x1fff)+1]))
		if (list & 0xe000) != 0xc000 {
			break
		}
	}
	if list != 0x3f00 {
		return nil
	}
	return buf
}

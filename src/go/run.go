package main

import "fmt"

// VMRun executes the VM loop. Returns status code (0=quit, 1=get_input, 2=get_key, 3=restore).
// Uses an inner closure with defer/recover for VM exception handling (panic with error codes).
// Uses a result variable to communicate return values back from the closure.
func (e *Engine) VMRun(param int) int {
	io := e.IO

	if param != 0 {
		e.store(int(e.Code[e.Inst]), param)
		e.Inst++
	}

	result := -1 // -1 = continue, >= 0 = return status

	for result < 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if x, ok := r.(int); ok && x > 0x4000 && x < 0x8000 {
						if e.Spc < SPLine {
							io.Line()
						}
						e.VMClearDivs()
						e.VMReset(x, false)
					} else {
						panic(r)
					}
				}
			}()

			for result < 0 {
				op := int(e.Code[e.Inst])
				e.Inst++

				switch op {
				case 0x00: // nop
				case 0x01: // fail
					e.fail()
				case 0x02: // set_cont code
					e.Cont = e.fcode()
				case 0x03: // proceed
					if e.Sim < 0x8000 {
						e.Cho = e.Sim
					}
					e.Inst = e.Cont
				case 0x04: // jmp code
					e.Inst = e.fcode()
				case 0x05: // jmp_multi code
					e.Sim = 0xffff
					e.Inst = e.fcode()
				case 0x85: // jmpl_multi code
					a1 := e.fcode()
					e.Cont = e.Inst
					e.Sim = 0xffff
					e.Inst = a1
				case 0x06: // jmp_simple code
					e.Sim = e.Cho
					e.Inst = e.fcode()
				case 0x86: // jmpl_simple code
					a1 := e.fcode()
					e.Cont = e.Inst
					e.Sim = e.Cho
					e.Inst = a1
				case 0x07: // jmp_tail code
					if e.Sim >= 0x8000 {
						e.Sim = e.Cho
					}
					e.Inst = e.fcode()
				case 0x87: // tail
					if e.Sim >= 0x8000 {
						e.Sim = e.Cho
					}
				case 0x08, 0x88: // push_env byte/0
					a1 := 0
					if op&0x80 == 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					}
					addr := e.Env
					if e.Cho < e.Env {
						addr = e.Cho
					}
					addr -= 4 + a1
					if addr < e.Top {
						panic(HeapFull)
					}
					e.HeapData[addr] = uint16(e.Env)
					e.HeapData[addr+1] = uint16(e.Sim)
					e.HeapData[addr+2] = uint16(e.Cont >> 16)
					e.HeapData[addr+3] = uint16(e.Cont & 0xffff)
					e.Env = addr
				case 0x09: // pop_env
					e.Cont = int(e.HeapData[e.Env+2])<<16 | int(e.HeapData[e.Env+3])
					e.Sim = int(e.HeapData[e.Env+1])
					e.Env = int(e.HeapData[e.Env])
				case 0x89: // pop_env_proceed
					e.Inst = int(e.HeapData[e.Env+2])<<16 | int(e.HeapData[e.Env+3])
					if int(e.HeapData[e.Env+1]) < 0x8000 {
						e.Cho = int(e.HeapData[e.Env+1])
					}
					e.Env = int(e.HeapData[e.Env])
				case 0x0a, 0x8a: // push_choice byte/0 next
					a1 := 0
					if op&0x80 == 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					}
					e.pushCho(a1, e.fcode())
				case 0x0b, 0x8b: // pop_choice byte/0
					a1 := 0
					if op&0x80 == 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					}
					for i := 0; i < a1; i++ {
						e.Reg[i] = e.HeapData[e.Cho+9+i]
					}
					for e.Trl < int(e.HeapData[e.Cho+8]) {
						e.HeapData[int(e.AuxData[e.Trl])] = 0
						e.Trl++
					}
					e.Top = int(e.HeapData[e.Cho+7])
					e.Cont = int(e.HeapData[e.Cho+2])<<16 | int(e.HeapData[e.Cho+3])
					e.Sim = int(e.HeapData[e.Cho+1])
					e.Env = int(e.HeapData[e.Cho])
					e.Cho = int(e.HeapData[e.Cho+6])
				case 0x0c, 0x8c: // pop_push_choice byte/0 code
					a1 := 0
					if op&0x80 == 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					}
					a2 := e.fcode()
					e.HeapData[e.Cho+4] = uint16(a2 >> 16)
					e.HeapData[e.Cho+5] = uint16(a2 & 0xffff)
					for i := 0; i < a1; i++ {
						e.Reg[i] = e.HeapData[e.Cho+9+i]
					}
					for e.Trl < int(e.HeapData[e.Cho+8]) {
						e.HeapData[int(e.AuxData[e.Trl])] = 0
						e.Trl++
					}
					e.Top = int(e.HeapData[e.Cho+7])
					e.Cont = int(e.HeapData[e.Cho+2])<<16 | int(e.HeapData[e.Cho+3])
					e.Sim = int(e.HeapData[e.Cho+1])
					e.Env = int(e.HeapData[e.Cho])
				case 0x0d: // cut_choice
					e.Cho = int(e.HeapData[e.Cho+6])
				case 0x0e: // get_cho dest
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, e.Cho)
				case 0x0f: // set_cho value
					e.Cho = e.fvalue()
				case 0x10, 0x90: // assign value/vbyte dest
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fvalue()
					}
					a2 := int(e.Code[e.Inst])
					e.Inst++
					e.store(a2, a1)
				case 0x11: // make_var dest
					a1 := int(e.Code[e.Inst])
					e.Inst++
					addr := e.Top
					e.Top++
					if e.Top > e.Env || e.Top > e.Cho {
						panic(HeapFull)
					}
					e.HeapData[addr] = 0
					e.store(a1, 0x8000|addr)
				case 0x12: // make_pair DEST DEST DEST
					a1 := int(e.Code[e.Inst])
					e.Inst++
					a2 := int(e.Code[e.Inst])
					e.Inst++
					a3 := int(e.Code[e.Inst])
					e.Inst++
					e.makePair(false, a1, a2, a3)
				case 0x13, 0x93: // make_pair WORD/VBYTE DEST DEST
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fword()
					}
					a2 := int(e.Code[e.Inst])
					e.Inst++
					a3 := int(e.Code[e.Inst])
					e.Inst++
					e.makePair(true, a1, a2, a3)
				case 0x14: // aux_push_val value
					e.pushAux(e.fvalue())
				case 0x94: // aux_push_raw 0
					if e.Aux >= e.Trl {
						panic(AuxFull)
					}
					e.AuxData[e.Aux] = 0
					e.Aux++
				case 0x15: // aux_push_raw word
					if e.Aux >= e.Trl {
						panic(AuxFull)
					}
					e.AuxData[e.Aux] = uint16(e.fword())
					e.Aux++
				case 0x95: // aux_push_raw vbyte
					if e.Aux >= e.Trl {
						panic(AuxFull)
					}
					e.AuxData[e.Aux] = uint16(e.Code[e.Inst])
					e.Inst++
					e.Aux++
				case 0x16: // aux_pop_val dest
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, e.popAux())
				case 0x17: // aux_pop_list dest
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, e.popAuxList())
				case 0x18: // aux_pop_list_chk value
					a1 := e.deref(e.fvalue())
					flag := false
					for {
						e.Aux--
						v := int(e.AuxData[e.Aux])
						if v == 0 {
							break
						}
						if v == a1 {
							flag = true
						}
					}
					if !flag {
						e.fail()
					}
				case 0x19: // aux_pop_list_match value
					e.Tmp = e.Top
					a1 := e.deref(e.fvalue())
					v := e.popAuxList()
					for (a1 & 0xe000) == 0xc000 {
						iter := v
						match := false
						for (iter & 0xe000) == 0xc000 && !match {
							if e.would_unify(iter-0x4000, a1-0x4000) {
								match = true
							}
							iter = e.deref(iter - 0x3fff)
						}
						if !match {
							e.fail()
							break
						}
						a1 = e.deref(a1 - 0x3fff)
					}
					e.Top = e.Tmp
				case 0x1b: // split_list value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					v := 0x3f00
					if a1 != a2 && (a1&0xe000) == 0xc000 {
						curr := e.Top
						v = 0xc000 | curr
						for {
							e.Top += 2
							if e.Top > e.Env || e.Top > e.Cho {
								panic(HeapFull)
							}
							e.HeapData[curr] = e.HeapData[a1&0x1fff]
							a1 = e.deref(a1 - 0x3fff)
							if a1 == a2 || (a1&0xe000) != 0xc000 {
								break
							}
							e.HeapData[curr+1] = uint16(0xc000 | e.Top)
							curr = e.Top
						}
						e.HeapData[curr+1] = 0x3f00
					}
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, v)
				case 0x1c: // stop
					e.Cho = e.Stc
					e.Inst = int(e.HeapData[e.Cho+4])<<16 | int(e.HeapData[e.Cho+5])
				case 0x1d: // push_stop code
					if e.Aux+2 > e.Trl {
						panic(AuxFull)
					}
					e.AuxData[e.Aux] = uint16(e.Stc)
					e.Aux++
					e.AuxData[e.Aux] = uint16(e.Sta)
					e.Aux++
					e.Sta = e.Aux
					e.pushCho(0, e.fcode())
					e.Stc = e.Cho
				case 0x1e: // pop_stop
					e.Aux = e.Sta
					e.Aux--
					e.Sta = int(e.AuxData[e.Aux])
					e.Aux--
					e.Stc = int(e.AuxData[e.Aux])
				case 0x1f: // split_word value dest
					a1 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x2000 && a1 < 0x3e00 {
						v := e.prependChars(a1, 0x3f00)
						e.store(d, v)
					} else if a1 >= 0x3e00 && a1 < 0x3f00 {
						v := e.createPair(a1, 0x3f00)
						e.store(d, v)
					} else if a1 >= 0x4000 && a1 < 0x8000 {
						i := a1 & 0x3fff
						v := 0x3f00
						for {
							v = e.createPair(0x4000|(i%10), v)
							i /= 10
							if i == 0 {
								break
							}
						}
						e.store(d, v)
					} else if a1 >= 0xe000 {
						a2 := int(e.HeapData[(a1&0x1fff)+0])
						if a2 >= 0x8000 {
							e.store(d, a2)
						} else {
							a3 := int(e.HeapData[(a1&0x1fff)+1])
							v := e.prependChars(a2, a3)
							e.store(d, v)
						}
					} else {
						e.fail()
					}
				case 0x9f: // join_words value dest
					a1 := e.deref(e.fvalue())
					if (a1 & 0xe000) != 0xc000 {
						e.fail()
						break
					}
					a2 := e.deref(int(e.HeapData[(a1&0x1fff)+0]))
					if (a2 & 0xff00) == 0x3e00 {
						a3 := e.deref(int(e.HeapData[(a1&0x1fff)+1]))
						if a3 == 0x3f00 {
							d := int(e.Code[e.Inst])
							e.Inst++
							e.store(d, a2)
							break
						}
					}
					tmp := e.wordsToCharlist(a1)
					d := int(e.Code[e.Inst])
					e.Inst++
					if tmp != nil {
						e.store(d, parseWord(tmp, e))
					} else {
						e.fail()
					}
				case 0x20, 0xa0: // load_word value/0 index dest
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, e.readfield(a2, a1))
				case 0x21, 0xa1: // load_byte value/0 index dest
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					v := e.readfield(a2>>1, a1)
					d := int(e.Code[e.Inst])
					e.Inst++
					if a2&1 != 0 {
						e.store(d, v&0xff)
					} else {
						e.store(d, v>>8)
					}
				case 0x22, 0xa2: // load_val value/0 index dest
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					v := e.getLTS(e.readfield(a2, a1))
					d := int(e.Code[e.Inst])
					e.Inst++
					if v != 0 {
						e.store(d, v)
					} else {
						e.fail()
					}
				case 0x24, 0xa4: // store_word value/0 index value
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					e.RamData[e.fieldaddr(a2, a1)] = uint16(e.fvalue())
				case 0x25, 0xa5: // store_byte value/0 index value
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := e.fvalue()
					addr := e.fieldaddr(a2>>1, a1)
					if a2&1 != 0 {
						e.RamData[addr] = (e.RamData[addr] & 0xff00) | uint16(a3&0xff)
					} else {
						e.RamData[addr] = (e.RamData[addr] & 0x00ff) | uint16((a3&0xff)<<8)
					}
				case 0x26, 0xa6: // store_val value/0 index value
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.deref(e.fvalue())
					}
					a2 := e.findex()
					a3 := e.fvalue()
					if a1 <= e.Nob || a3 != 0 {
						e.putLTS(e.fieldaddr(a2, a1), a3)
					}
				case 0x28, 0xa8: // set_flag value/0 index
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					e.RamData[e.fieldaddr(a2>>4, a1)] |= uint16(0x8000 >> (a2 & 15))
				case 0x29, 0xa9: // reset_flag value/0 index
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.deref(e.fvalue())
					}
					a2 := e.findex()
					if a1 <= e.Nob {
						e.RamData[e.fieldaddr(a2>>4, a1)] &^= uint16(0x8000 >> (a2 & 15))
					}
				case 0x2d, 0xad: // unlink value/0 index index value
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := e.findex()
					e.unlink(e.fieldaddr(a2, a1), a3, e.deref(e.fvalue()))
				case 0x2e, 0x2f, 0xae, 0xaf: // set_parent value/vbyte value/vbyte
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.deref(e.fvalue())
					}
					a2 := 0
					if op&0x01 != 0 {
						a2 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a2 = e.deref(e.fvalue())
					}
					if a1 < e.Nob || a2 != 0 {
						if a1 >= 0x2000 || a2 >= 0x2000 {
							panic(ExpectObj)
						}
						v := int(e.RamData[e.fieldaddr(0, a1)])
						if v != 0 {
							e.unlink(e.fieldaddr(1, v), 2, a1)
						}
						e.RamData[e.fieldaddr(0, a1)] = uint16(a2)
						if a2 != 0 {
							e.RamData[e.fieldaddr(2, a1)] = e.RamData[e.fieldaddr(1, a2)]
							e.RamData[e.fieldaddr(1, a2)] = uint16(a1)
						}
					}
				case 0x30, 0xb0: // if_raw_eq word/0 value code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fword()
					}
					a2 := e.fvalue()
					a3 := e.fcode()
					if a1 == a2 {
						e.Inst = a3
					}
				case 0x31: // if_bound value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if (a1 & 0xe000) != 0x8000 {
						e.Inst = a2
					}
				case 0x32: // if_empty value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 == 0x3f00 {
						e.Inst = a2
					}
				case 0x33: // if_num value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 >= 0x4000 && a1 < 0x8000 {
						e.Inst = a2
					}
				case 0x34: // if_pair value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if (a1 & 0xe000) == 0xc000 {
						e.Inst = a2
					}
				case 0x35: // if_obj value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 < 0x2000 {
						e.Inst = a2
					}
				case 0x36: // if_word value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 >= 0xe000 || (a1 >= 0x2000 && a1 < 0x3f00) {
						e.Inst = a2
					}
				case 0xb6: // if_listword value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 >= 0xe000 && (int(e.HeapData[a1&0x1fff])&0xe000) == 0xc000 {
						e.Inst = a2
					}
				case 0x37: // if_unify value value code
					a1 := e.fvalue()
					a2 := e.fvalue()
					a3 := e.fcode()
					if e.would_unify(a1, a2) {
						e.Inst = a3
					}
				case 0x38: // if_gt value value code
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					a3 := e.fcode()
					if a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 && a1 > a2 {
						e.Inst = a3
					}
				case 0x39, 0xb9: // if_eq word/vbyte value code
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fword()
					}
					a2 := e.fvalue()
					a3 := e.fcode()
					if a1 == e.deref(a2) {
						e.Inst = a3
					}
				case 0x3a, 0xba: // if_mem_eq value/0 index value code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := e.fvalue()
					a4 := e.fcode()
					if e.readfield(a2, a1) == a3 {
						e.Inst = a4
					}
				case 0x3b, 0xbb: // if_flag value/0 index code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := e.fcode()
					if int(e.readfield(a2>>4, a1))&(0x8000>>(a2&15)) != 0 {
						e.Inst = a3
					}
				case 0x3c: // if_cwl code
					a1 := e.fcode()
					if e.Cwl != 0 {
						e.Inst = a1
					}
				case 0x3d, 0xbd: // if_mem_eq value/0 index vbyte code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := int(e.Code[e.Inst])
					e.Inst++
					a4 := e.fcode()
					if e.readfield(a2, a1) == a3 {
						e.Inst = a4
					}
				case 0x40, 0xc0: // ifn_raw_eq word/0 value code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fword()
					}
					a2 := e.fvalue()
					a3 := e.fcode()
					if a1 != a2 {
						e.Inst = a3
					}
				case 0x41: // ifn_bound value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if (a1 & 0xe000) == 0x8000 {
						e.Inst = a2
					}
				case 0x42: // ifn_empty value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 != 0x3f00 {
						e.Inst = a2
					}
				case 0x43: // ifn_num value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 < 0x4000 || a1 >= 0x8000 {
						e.Inst = a2
					}
				case 0x44: // ifn_pair value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if (a1 & 0xe000) != 0xc000 {
						e.Inst = a2
					}
				case 0x45: // ifn_obj value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 >= 0x2000 {
						e.Inst = a2
					}
				case 0x46: // ifn_word value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 < 0xe000 && (a1 < 0x2000 || a1 >= 0x3f00) {
						e.Inst = a2
					}
				case 0xc6: // ifn_listword value code
					a1 := e.deref(e.fvalue())
					a2 := e.fcode()
					if a1 < 0xe000 || (int(e.HeapData[a1&0x1fff])&0xe000) != 0xc000 {
						e.Inst = a2
					}
				case 0x47: // ifn_unify value value code
					a1 := e.fvalue()
					a2 := e.fvalue()
					a3 := e.fcode()
					if !e.would_unify(a1, a2) {
						e.Inst = a3
					}
				case 0x48: // ifn_gt value value code
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					a3 := e.fcode()
					if !(a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 && a1 > a2) {
						e.Inst = a3
					}
				case 0x49, 0xc9: // ifn_eq word/vbyte value code
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fword()
					}
					a2 := e.fvalue()
					a3 := e.fcode()
					if a1 != e.deref(a2) {
						e.Inst = a3
					}
				case 0x4a, 0xca: // ifn_mem_eq value/0 index value code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := e.fvalue()
					a4 := e.fcode()
					if e.readfield(a2, a1) != a3 {
						e.Inst = a4
					}
				case 0x4b, 0xcb: // ifn_flag value/0 index code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := e.fcode()
					if int(e.readfield(a2>>4, a1))&(0x8000>>(a2&15)) == 0 {
						e.Inst = a3
					}
				case 0x4c: // ifn_cwl code
					a1 := e.fcode()
					if e.Cwl == 0 {
						e.Inst = a1
					}
				case 0x4d, 0xcd: // ifn_mem_eq value/0 index vbyte code
					a1 := 0
					if op&0x80 == 0 {
						a1 = e.fvalue()
					}
					a2 := e.findex()
					a3 := int(e.Code[e.Inst])
					e.Inst++
					a4 := e.fcode()
					if e.readfield(a2, a1) != a3 {
						e.Inst = a4
					}
				case 0x50: // add_raw value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, (a1+a2)&0xffff)
				case 0xd0: // inc_raw value dest
					a1 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, (a1+1)&0xffff)
				case 0x51: // sub_raw value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, (a1-a2)&0xffff)
				case 0xd1: // dec_raw value dest
					a1 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, (a1-1)&0xffff)
				case 0x52: // rand_raw byte dest
					a1 := int(e.Code[e.Inst])
					e.Inst++
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, int(e.compatRand())%(a1+1))
				case 0x58: // add_num value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 {
						v := (a1 & 0x3fff) + (a2 & 0x3fff)
						if v < 0x4000 {
							e.store(d, v|0x4000)
						} else {
							e.fail()
						}
					} else {
						e.fail()
					}
				case 0xd8: // inc_num value dest
					a1 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x7fff {
						e.store(d, a1+1)
					} else {
						e.fail()
					}
				case 0x59: // sub_num value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 {
						v := (a1 & 0x3fff) - (a2 & 0x3fff)
						if v >= 0 {
							e.store(d, v|0x4000)
						} else {
							e.fail()
						}
					} else {
						e.fail()
					}
				case 0xd9: // dec_num value dest
					a1 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 > 0x4000 && a1 < 0x8000 {
						e.store(d, a1-1)
					} else {
						e.fail()
					}
				case 0x5a: // rand_num value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 && a2 >= a1 {
						v := a1 + int(e.compatRand())%(a2-a1+1)
						e.store(d, v)
					} else {
						e.fail()
					}
				case 0x5b: // mul_num value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 {
						v := ((a1 & 0x3fff) * (a2 & 0x3fff)) & 0x3fff
						e.store(d, v|0x4000)
					} else {
						e.fail()
					}
				case 0x5c: // div_num value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x8000 && a2 > 0x4000 && a2 < 0x8000 {
						v := (a1 & 0x3fff) / (a2 & 0x3fff)
						e.store(d, v|0x4000)
					} else {
						e.fail()
					}
				case 0x5d: // mod_num value value dest
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					d := int(e.Code[e.Inst])
					e.Inst++
					if a1 >= 0x4000 && a1 < 0x8000 && a2 > 0x4000 && a2 < 0x8000 {
						v := (a1 & 0x3fff) % (a2 & 0x3fff)
						e.store(d, v|0x4000)
					} else {
						e.fail()
					}
				case 0x60: // print_a_str_a string
					if e.Spc == SPAuto || e.Spc == SPPending {
						io.Space()
					} else if e.Spc == SPNBSP {
						io.Nbsp()
					}
					io.Print(e.DecodeStr(e.fstring()))
					e.Spc = SPAuto
				case 0xe0: // print_n_str_a string
					if e.Spc == SPPending {
						io.Space()
					} else if e.Spc == SPNBSP {
						io.Nbsp()
					}
					io.Print(e.DecodeStr(e.fstring()))
					e.Spc = SPAuto
				case 0x61: // print_a_str_n string
					if e.Spc == SPAuto || e.Spc == SPPending {
						io.Space()
					} else if e.Spc == SPNBSP {
						io.Nbsp()
					}
					io.Print(e.DecodeStr(e.fstring()))
					e.Spc = SPNoSpace
				case 0xe1: // print_n_str_n string
					if e.Spc == SPPending {
						io.Space()
					} else if e.Spc == SPNBSP {
						io.Nbsp()
					}
					io.Print(e.DecodeStr(e.fstring()))
					e.Spc = SPNoSpace
				case 0x62: // nospace
					if e.Cwl == 0 {
						if e.Spc < SPNoSpace {
							e.Spc = SPNoSpace
						}
					}
				case 0xe2: // space
					if e.Cwl == 0 {
						if e.Spc < SPPending {
							e.Spc = SPPending
						}
					}
				case 0x63: // line
					if e.Cwl == 0 {
						if e.Spc < SPLine {
							io.Line()
							e.Spc = SPLine
						}
					}
				case 0xe3: // par
					if e.Cwl == 0 {
						if e.Spc < SPPar {
							if e.NSpan != 0 {
								io.Line()
								io.Line()
							} else {
								io.Par()
							}
							e.Spc = SPPar
						}
					}
				case 0x64: // space_n value
					a1 := e.deref(e.fvalue())
					if e.Cwl == 0 && a1 > 0x4000 && a1 < 0x8000 {
						io.SpaceN(a1 & 0x3fff)
						e.Spc = SPSpace
					}
				case 0x65: // print_val value
					a1 := e.deref(e.fvalue())
					if e.Cwl != 0 {
						e.pushAux(a1)
					} else {
						if (a1 & 0xff00) == 0x3e00 {
							tmp := a1 & 0xff
							if e.Spc == SPPending || (e.Spc == SPAuto && !containsByte(e.NospBefore, tmp)) {
								io.Space()
							}
							io.Print(e.DecodeChar(tmp))
							if containsByte(e.NospAfter, tmp) {
								e.Spc = SPNoSpace
							} else {
								e.Spc = SPAuto
							}
						} else {
							if e.Spc == SPAuto || e.Spc == SPPending {
								io.Space()
							}
							io.Print(e.val2str(a1))
							e.Spc = SPAuto
						}
					}
				case 0x66: // enter_div index
					a1 := e.findex()
					if e.Cwl == 0 {
						if e.NSpan != 0 {
							panic(IOState)
						}
						io.EnterDiv(a1)
						e.Divs = append(e.Divs, a1)
						e.Spc = SPPar
					}
				case 0xe6: // leave_div
					if e.Cwl == 0 {
						last := len(e.Divs) - 1
						io.LeaveDiv(e.Divs[last])
						e.Divs = e.Divs[:last]
						e.Spc = SPLine
					}
				case 0x67:
					if e.Head[0] < 1 { // enter_status 0 index (pre-1.0)
						a1 := e.findex()
						if e.Cwl == 0 {
							if e.InStatus || e.NSpan != 0 {
								panic(IOState)
							}
							io.EnterStatus(0, a1)
							e.InStatus = true
							e.Spc = SPPar
						}
					} else { // set_body index (1.0+)
						a1 := e.findex()
						if e.InStatus || e.NSpan != 0 {
							panic(IOState)
						}
						io.SetBody(a1)
					}
				case 0xe7:
					if e.Head[0] < 1 { // leave_status (pre-1.0)
						if e.Cwl == 0 {
							io.LeaveStatus()
							e.InStatus = false
							e.Spc = SPPar
						}
					}
				case 0x68: // enter_link_res value
					a1 := e.deref(e.fvalue())
					if e.Cwl == 0 {
						if e.NLink == 0 {
							if e.Spc == SPAuto || e.Spc == SPPending {
								io.Space()
							}
							io.EnterLinkRes(e.GetRes(a1 & 0x1fff))
							e.Spc = SPNoSpace
						}
						e.NLink++
						e.NSpan++
					}
				case 0xe8: // leave_link_res
					if e.Cwl == 0 {
						e.NLink--
						e.NSpan--
						if e.NLink == 0 {
							io.LeaveLinkRes()
						}
					}
				case 0x69: // enter_link value
					a1 := e.deref(e.fvalue())
					if e.Cwl == 0 {
						if e.NLink == 0 {
							if e.Spc == SPAuto || e.Spc == SPPending {
								io.Space()
							}
							savedUpper := e.Upper
							e.Upper = false
							str := ""
							for (a1 & 0xe000) == 0xc000 {
								v := e.deref(a1 - 0x4000)
								if (v >= 0x2000 && v < 0x8000) || v >= 0xe000 {
									if str != "" {
										str += " "
									}
									str += e.val2str(v)
								}
								a1 = e.deref(a1 - 0x3fff)
							}
							io.EnterLink(str)
							e.Upper = savedUpper
							e.Spc = SPNoSpace
						}
						e.NLink++
						e.NSpan++
					}
				case 0xe9: // leave_link
					if e.Cwl == 0 {
						e.NLink--
						e.NSpan--
						if e.NLink == 0 {
							io.LeaveLink()
						}
					}
				case 0x6a: // enter_self_link
					if e.Cwl == 0 {
						if e.NLink == 0 {
							if e.Spc == SPAuto || e.Spc == SPPending {
								io.Space()
							}
							io.EnterSelfLink()
							e.Spc = SPNoSpace
						}
						e.NLink++
						e.NSpan++
					}
				case 0xea: // leave_self_link
					if e.Cwl == 0 {
						e.NLink--
						e.NSpan--
						if e.NLink == 0 {
							io.LeaveSelfLink()
						}
					}
				case 0x6b: // set_style byte
					a1 := int(e.Code[e.Inst])
					e.Inst++
					if e.Cwl == 0 {
						if e.Spc == SPAuto || e.Spc == SPPending {
							io.Space()
						}
						io.SetStyle(a1)
						e.Spc = SPSpace
					}
				case 0xeb: // reset_style byte
					a1 := int(e.Code[e.Inst])
					e.Inst++
					if e.Cwl == 0 {
						io.ResetStyle(a1)
					}
				case 0x6c: // embed_res value
					a1 := e.deref(e.fvalue())
					if e.Spc == SPAuto || e.Spc == SPPending {
						io.Space()
					}
					io.EmbedRes(e.GetRes(a1 & 0x1fff))
					e.Spc = SPAuto
				case 0xec: // can_embed_res value dest
					a1 := e.deref(e.fvalue())
					v := 0
					if io.CanEmbedRes(e.GetRes(a1 & 0x1fff)) {
						v = 1
					}
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, v)
				case 0x6d: // progress value value
					a1 := e.deref(e.fvalue())
					a2 := e.deref(e.fvalue())
					if e.Cwl == 0 {
						if a1 >= 0x4000 && a1 < 0x8000 && a2 >= 0x4000 && a2 < 0x8000 {
							io.ProgressBar(a1&0x3fff, a2&0x3fff)
						}
					}
				case 0x6e: // enter_span index
					a1 := e.findex()
					if e.Cwl == 0 {
						if e.Spc == SPAuto || e.Spc == SPPending {
							io.Space()
						}
						io.EnterSpan(a1)
						e.NSpan++
						e.Spc = SPNoSpace
					}
				case 0xee: // leave_span
					if e.Cwl == 0 {
						io.LeaveSpan()
						e.NSpan--
						e.Spc = SPAuto
					}
				case 0x6f: // enter_status byte index (1.0+)
					a1 := int(e.Code[e.Inst])
					e.Inst++
					a2 := e.findex()
					if e.Cwl == 0 {
						if e.InStatus || e.NSpan != 0 {
							panic(IOState)
						}
						io.EnterStatus(a1, a2)
						e.InStatus = true
						e.Spc = SPPar
					}
				case 0xef: // leave_status (1.0+)
					if e.Head[0] > 0 {
						if e.Cwl == 0 {
							io.LeaveStatus()
							e.InStatus = false
							e.Spc = SPPar
						}
					}
				case 0x70: // ext0 byte
					a1 := int(e.Code[e.Inst])
					e.Inst++
					switch a1 {
					case 0x00: // quit
						io.Flush()
						result = 0
						return
					case 0x01: // restart
						e.VMClearDivs()
						e.VMReset(0, true)
						e.VMRestoreState(e.InitState)
						io.Reset()
					case 0x02: // restore
						io.Flush()
						io.Restore()
						result = 3
						return
					case 0x03: // undo
						if len(e.UndoData) > 0 {
							e.VMClearDivs()
							last := len(e.UndoData) - 1
							e.VMRestoreState(VMRLDecState(e.InitState, &e.UndoData[last]))
							e.UndoData = e.UndoData[:last]
						} else if !e.PrunedUndo {
							e.fail()
						}
					case 0x04: // unstyle
						if e.Cwl == 0 {
							io.Unstyle()
						}
					case 0x05: // print_serial
						if e.Cwl == 0 {
							if e.Spc == SPAuto || e.Spc == SPPending {
								io.Space()
							}
							for i := 0; i < 6; i++ {
								io.Print(string(rune(e.Head[6+i])))
							}
							e.Spc = SPAuto
						}
					case 0x06, 0x07: // clear, clear_all
						if e.InStatus || e.NSpan != 0 {
							panic(IOState)
						}
						tmp := e.Divs
						e.VMClearDivs()
						if a1 == 0x06 {
							io.Clear()
						} else {
							io.ClearAll()
						}
						for _, d := range tmp {
							io.EnterDiv(d)
						}
						e.Divs = tmp
					case 0x08: // script_on
						if !io.ScriptOn() {
							e.fail()
						}
					case 0x09: // script_off
						io.ScriptOff()
					case 0x0a: // trace_on
						e.Trace = true
					case 0x0b: // trace_off
						e.Trace = false
					case 0x0c: // inc_cwl
						e.Cwl++
					case 0x0d: // dec_cwl
						e.Cwl--
					case 0x0e: // uppercase
						if e.Cwl == 0 {
							e.Upper = true
						}
					case 0x0f: // clear_links
						io.ClearLinks()
					case 0x10: // clear_old
						if e.NSpan != 0 {
							panic(IOState)
						}
						io.ClearOld()
					case 0x11: // clear_div
						io.ClearDiv()
					case 0x12: // clear_status
						if e.InStatus {
							panic(IOState)
						}
						io.ClearStatus()
					case 0x13: // nbsp
						if e.Cwl == 0 {
							if e.Spc < SPNBSP {
								e.Spc = SPNBSP
							}
						}
					}
				case 0x72: // save code
					a1 := e.fcode()
					if e.InStatus || e.NSpan != 0 {
						panic(IOState)
					}
					if !io.Save(VMWrapSavefile(e, VMRLEncState(e.InitState, e.VMCaptureState(a1)))) {
						e.fail()
					}
				case 0xf2: // save_undo code
					a1 := e.fcode()
					if e.InStatus || e.NSpan != 0 {
						panic(IOState)
					}
					if len(e.UndoData) > 50 {
						e.UndoData = e.UndoData[1:]
						e.PrunedUndo = true
					}
					e.UndoData = append(e.UndoData, *VMRLEncState(e.InitState, e.VMCaptureState(a1)))
				case 0x73: // get_input dest
					if e.Spc == SPAuto || e.Spc == SPPending {
						io.Space()
					} else if e.Spc == SPNBSP {
						io.Nbsp()
					}
					io.Flush()
					result = 1
					return
				case 0xf3: // get_key dest
					if e.Spc == SPAuto || e.Spc == SPPending {
						io.Space()
					} else if e.Spc == SPNBSP {
						io.Nbsp()
					}
					io.Flush()
					result = 2
					return
				case 0x74: // vm_info byte dest
					a1 := int(e.Code[e.Inst])
					e.Inst++
					v := 0
					switch a1 {
					case 0x00: // peak heap
						for i := 0; i < len(e.HeapData); i++ {
							if e.HeapData[i] != 0x3f3f {
								v++
							}
						}
						v += 0x4000
					case 0x01: // peak aux
						for i := 0; i < len(e.AuxData); i++ {
							if e.AuxData[i] != 0x3f3f {
								v++
							}
						}
						v += 0x4000
					case 0x02: // peak lts
						for i := e.Ltb; i < len(e.RamData); i++ {
							if e.RamData[i] != 0x3f3f {
								v++
							}
						}
						v += 0x4000
					case 0x20: // div width
						v = 0x4000 | io.MeasureDims(0)
					case 0x21: // div height
						v = 0x4000 | io.MeasureDims(1)
					case 0x40: // interpreter supports undo
						v = 1
					case 0x41: // interpreter supports save/restore
						v = 1
					case 0x42: // interpreter supports links
						if io.HaveLinks() {
							v = 1
						}
					case 0x43: // interpreter supports quit
						if e.HaveQuit {
							v = 1
						}
					case 0x44: // interpreter supports styling
						if io.HaveStyles() {
							v = 1
						}
					case 0x45: // interpreter supports color
						if io.HaveColor() {
							v = 1
						}
					case 0x46: // interpreter supports text-align
						if io.HaveAlign() {
							v = 1
						}
					case 0x50: // currently transcripting
						if io.ScriptActive() {
							v = 1
						}
					case 0x60: // interpreter supports top status area
						if e.HaveTop {
							v = 1
						}
					case 0x61: // interpreter supports inline status area
						if e.HaveInline {
							v = 1
						}
					default:
						if a1 < 0x40 {
							v = 0x4000
						}
					}
					d := int(e.Code[e.Inst])
					e.Inst++
					e.store(d, v)
				case 0x78: // set_idx value
					v := e.deref(e.fvalue())
					if v >= 0xe000 {
						v = int(e.HeapData[v&0x1fff])
					}
					e.Reg[0x3f] = uint16(v)
				case 0x79, 0xf9: // check_eq word/vbyte code
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fword()
					}
					a2 := e.fcode()
					if int(e.Reg[0x3f]) == a1 {
						e.Inst = a2
					}
				case 0x7a, 0xfa: // check_gt_eq word/vbyte code code
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fword()
					}
					a2 := e.fcode()
					a3 := e.fcode()
					if int(e.Reg[0x3f]) > a1 {
						e.Inst = a2
					} else if int(e.Reg[0x3f]) == a1 {
						e.Inst = a3
					}
				case 0x7b, 0xfb: // check_gt value/byte code
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fvalue()
					}
					a2 := e.fcode()
					if int(e.Reg[0x3f]) > a1 {
						e.Inst = a2
					}
				case 0x7c: // check_wordmap index code
					a1 := e.findex()
					a2 := e.fcode()
					if e.wordMap(a1, int(e.Reg[0x3f])) {
						e.Inst = a2
					}
				case 0x7d, 0xfd: // check_eq word/vbyte word/vbyte code
					a1 := 0
					if op&0x80 != 0 {
						a1 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a1 = e.fword()
					}
					a2 := 0
					if op&0x80 != 0 {
						a2 = int(e.Code[e.Inst])
						e.Inst++
					} else {
						a2 = e.fword()
					}
					a3 := e.fcode()
					if int(e.Reg[0x3f]) == a1 || int(e.Reg[0x3f]) == a2 {
						e.Inst = a3
					}
				case 0x7f: // tracepoint string string string word
					a1 := e.fstring()
					a2s := e.fstring()
					a3 := e.fstring()
					a4 := e.fword()
					if e.Trace {
						str := e.DecodeStr(a1) + "("
						a2str := e.DecodeStr(a2s)
						j := 0
						for i := 0; i < len(a2str); i++ {
							if a2str[i] == '$' {
								str += e.val2str(int(e.Reg[j]))
								j++
							} else {
								str += string(a2str[i])
							}
						}
						str += ") " + e.DecodeStr(a3) + ":" + fmt.Sprintf("%d", a4)
						io.Trace(str)
					}
				default:
					panic(fmt.Sprintf("Unimplemented op 0x%02x at 0x%06x", op, e.Inst-1))
				}
			}
		}()
	}

	return result
}

// VMStart begins execution from the start of the code section
func (e *Engine) VMStart() int {
	return e.VMRun(0)
}

// VMProceedWithInput processes a line of user input
func (e *Engine) VMProceedWithInput(str string) int {
	var chars []int
	for _, r := range str {
		c := int(r)
		if c >= 0x41 && c <= 0x5a {
			chars = append(chars, c^0x20)
		} else if c < 0x80 {
			chars = append(chars, c)
		} else {
			found := 0x3f
			for j := int(e.Lang[e.ExtChars]) - 1; j >= 0; j-- {
				entry := e.ExtChars + 1 + j*5
				if int(e.Lang[entry+2]) == (c>>16)&0xff &&
					int(e.Lang[entry+3]) == (c>>8)&0xff &&
					int(e.Lang[entry+4]) == c&0xff {
					found = int(e.Lang[entry])
					break
				}
			}
			chars = append(chars, found)
		}
	}

	var words [][]int
	start := 0
	for i := 0; i < len(chars); i++ {
		if chars[i] == 32 {
			if i != start {
				words = append(words, chars[start:i])
			}
			start = i + 1
		} else {
			if containsByte(e.StopChars, chars[i]) {
				if i != start {
					words = append(words, chars[start:i])
				}
				words = append(words, chars[i:i+1])
				start = i + 1
			}
		}
	}
	if len(chars) != start {
		words = append(words, chars[start:])
	}

	v := 0x3f00
	func() {
		defer func() {
			if r := recover(); r != nil {
				if x, ok := r.(int); ok && x == HeapFull {
					if e.Spc < SPLine {
						e.IO.Line()
					}
					e.VMClearDivs()
					e.VMReset(x, false)
					v = 0
				} else {
					panic(r)
				}
			}
		}()
		for i := len(words) - 1; i >= 0; i-- {
			v = e.createPair(parseWord(words[i], e), v)
		}
	}()

	e.Spc = SPLine
	return e.VMRun(v)
}

// VMProceedWithKey processes a keypress
func (e *Engine) VMProceedWithKey(code int) int {
	v := 0
	if code >= 0x20 && code < 0x7f {
		if code >= 0x41 && code <= 0x5a {
			code ^= 0x20
		}
		v = code
	}
	if v == 0 {
		for _, k := range Keys {
			if code == k {
				v = code
				break
			}
		}
	}
	if v == 0 {
		for i := 0; i < int(e.Lang[e.ExtChars]); i++ {
			entry := 1 + i*5
			if code == (int(e.Lang[entry+2])<<16 |
				int(e.Lang[entry+3])<<8 |
				int(e.Lang[entry+4])) {
				v = 0x80 | int(e.Lang[1+i*5])
				break
			}
		}
	}
	if v == 0 {
		return StatusGetKey
	}
	e.Spc = SPSpace
	if v >= 0x30 && v <= 0x39 {
		v += 0x4000 - 0x30
	} else {
		v |= 0x3e00
	}
	return e.VMRun(v)
}

// VMRestore restores from save data
func (e *Engine) VMRestore(filedata []byte) int {
	v := VMUnwrapSavefile(e, filedata)
	if v != nil {
		e.VMClearDivs()
		e.VMReset(0, true)
		e.VMRestoreState(VMRLDecState(e.InitState, v))
	}
	e.Spc = SPLine
	return e.VMRun(0)
}

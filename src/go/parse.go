package main

// parseWord tokenizes a character list into a dictionary word, number, character, or unknown
func parseWord(chars []int, e *Engine) int {
	state := 0
	var revEnding []int
	v := 0
	enddecoder := get16(e.Lang, 4)
	l := len(chars)

	buildlist := func(list []int) int {
		v := 0x3f00
		for i := 0; i < len(list); i++ {
			ch := list[i]
			if ch >= 0x30 && ch <= 0x39 {
				v = e.createPair(ch+0x4000-0x30, v)
			} else {
				v = e.createPair(0x3e00|ch, v)
			}
		}
		return v
	}

	finddict := func() int {
		start := 0
		end := get16(e.Dict, 0)
		for start < end {
			mid := (start + end) >> 1
			dictlen := int(e.Dict[2+3*mid])
			dictoffs := get16(e.Dict, 2+3*mid+1)
			diff := 0
			i := 0
			for i < l && i < dictlen {
				diff = chars[i] - int(e.Dict[dictoffs+i])
				if diff != 0 {
					break
				}
				i++
			}
			if i == dictlen && i == l {
				if diff == 0 {
					return 0x2000 | mid
				}
			} else if i == dictlen {
				diff = 1
			} else if i == l {
				diff = -1
			}
			if diff < 0 {
				end = mid
			} else {
				start = mid + 1
			}
		}
		return 0
	}

	// Try dictionary lookup for words longer than 1
	if l > 1 {
		v = finddict()
		if v != 0 {
			return v
		}
	}

	// Try number
	v = 0
	i := 0
	for i < len(chars) {
		if chars[i] < 0x30 || chars[i] > 0x39 {
			break
		}
		v = v*10 + chars[i] - 0x30
		if v >= 16384 {
			break
		}
		i++
	}
	if i == len(chars) {
		return 0x4000 | v
	}

	// Single character
	if len(chars) == 1 {
		return 0x3e00 | chars[0]
	}

	// Try word ending decomposition
	for {
		instr := int(e.Lang[enddecoder+state])
		state++
		if instr == 0 {
			for l > 0 {
				l--
				revEnding = append(revEnding, chars[l])
			}
			return e.createPair(buildlist(revEnding), 0x3f00) | 0xe000
		} else if instr == 1 {
			v = finddict()
			if v != 0 {
				return e.createPair(v, buildlist(revEnding)) | 0xe000
			}
		} else {
			next := int(e.Lang[enddecoder+state])
			state++
			if l > 2 && instr == chars[l-1] {
				revEnding = append(revEnding, instr)
				l--
				state = next
			}
		}
	}
}

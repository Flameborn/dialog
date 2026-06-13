-- Å-machine virtual machine engine, ported from src/js/engine.js to LuaJIT.
--
-- Original JavaScript engine: Copyright 2019-2022 Linus Åkesson.
-- See license.txt for the BSD 2-clause terms that also cover this port.
--
-- The port follows the JavaScript source closely. Arrays are 0-based Lua
-- tables carrying an explicit `length` field, matching the original index
-- arithmetic and opcode encodings so no offsets need adjusting.

local bit = require("bit")
local band, bor, bxor, bnot = bit.band, bit.bor, bit.bxor, bit.bnot
local lshift, rshift, arshift = bit.lshift, bit.rshift, bit.arshift

-- Error codes
local HEAPFULL = 0x4001
local AUXFULL = 0x4002
local EXPECTOBJ = 0x4003
local EXPECTBOUND = 0x4004
local LTSFULL = 0x4006
local IOSTATE = 0x4007

-- Opcode names
local opcodes = {
	[0x00]='nop', [0x01]='fail', [0x02]='cont-set', [0x03]='proceed', [0x04]='jmp',
	[0x05]='jmp-multi', [0x85]='jmpl-multi', [0x06]='jmpl-simple', [0x86]='jmpl-simple',
	[0x07]='jmp-tail', [0x87]='tail', [0x08]='env-push', [0x88]='env-push/0',
	[0x09]='env-pop', [0x89]='env-pop-proceed', [0x0a]='choice-push', [0x8a]='choice-push/0',
	[0x0b]='choice-pop', [0x8b]='choice-pop/0', [0x0c]='choice-pop-push', [0x8c]='choice-pop-push/0',
	[0x0d]='choice-cut', [0x0e]='cho-get', [0x0f]='cho-set', [0x10]='assign', [0x90]='assign/vbyte',
	[0x11]='make-var', [0x12]='make-pair/dest', [0x13]='make-pair/word', [0x93]='make-pair/vbyte',
	[0x14]='aux-val-push', [0x94]='aux-raw-push/0', [0x15]='aux-raw-push/word', [0x95]='aux-raw-push/vbyte',
	[0x16]='aux-val-pop', [0x17]='aux-list-pop', [0x18]='aux-list-chk-pop', [0x19]='aux-list-match-pop',
	[0x1b]='list-split', [0x1c]='stop', [0x1d]='stop-push', [0x1e]='stop-pop', [0x1f]='word-split',
	[0x9f]='words-join', [0x20]='load-word', [0xa0]='load-word/0', [0x21]='load-byte', [0xa1]='load-byte/0',
	[0x22]='load-val', [0xa2]='load-val/0', [0x24]='store-word', [0xa4]='store-word/0',
	[0x25]='store-byte', [0xa5]='store-byte/0', [0x26]='store-val', [0xa6]='store-val/0',
	[0x28]='flag-set', [0xa8]='flag-set/0', [0x29]='flag-reset', [0xa9]='flag-reset/0',
	[0x2d]='unlink', [0xad]='unlink/0', [0x2e]='parent-set', [0xae]='parent-set/vbyte',
	[0x2f]='parent-set/vbyte2', [0xaf]='parent-set/vbyte-both', [0x30]='raw-eq?/word', [0xb0]='raw-eq?/0',
	[0x31]='bound?', [0x32]='empty?', [0x33]='num?', [0x34]='pair?', [0x35]='obj?', [0x36]='word?',
	[0xb6]='uword?', [0x37]='unify?', [0x38]='gt?', [0x39]='eq?/word', [0xb9]='eq?/vbyte',
	[0x3a]='mem-eq?', [0xba]='mem-eq?/0', [0x3b]='flag?', [0xbb]='flag?/0', [0x3c]='cwl?',
	[0x3d]='mem-eq?', [0xbd]='mem-eq?', [0x40]='not-raw-eq?/word', [0xc0]='not-raw-eq?/0',
	[0x41]='not-bound?', [0x42]='not-empty?', [0x43]='not-num?', [0x44]='not-pair?', [0x45]='not-obj?',
	[0x46]='not-uword?', [0x47]='not-unify?', [0x48]='not-gt?', [0x49]='not-eq?/word', [0xc9]='not-eq?/vbyte',
	[0x4a]='not-mem-eq?', [0xca]='not-mem-eq?/0', [0x4b]='not-flag?', [0xcb]='not-flag?/0', [0x4c]='not-cwl?',
	[0x4d]='not-mem-eq?', [0xcd]='not-mem-eq?/0', [0x50]='raw-add', [0xd0]='raw-inc', [0x51]='raw-sub',
	[0xd1]='raw-dec', [0x52]='raw-rand', [0x58]='num-add', [0xd8]='num-inc', [0x59]='num-sub',
	[0xd9]='num-dec', [0x5a]='num-rand', [0x5b]='num-mul', [0x5c]='num-div', [0x5d]='num-mod',
	[0x60]='print-a-str-a', [0xe0]='print-n-str-a', [0x61]='print-a-str-n', [0xe1]='print-n-str-n',
	[0x62]='no-space', [0xe2]='space', [0x63]='line', [0xe3]='par', [0x64]='space-n', [0x65]='print-val',
	[0x66]='enter-div', [0xe6]='leave-div', [0x67]='set-body/old-enter-status', [0xe7]='old-leave-status',
	[0x68]='enter-link-res', [0xe8]='leave-link-res', [0x69]='enter-link', [0xe9]='leave-link',
	[0x6a]='enter-self-link', [0xea]='leave-self-link', [0x6b]='set-style', [0xeb]='reset-style',
	[0x6c]='embed-res', [0xec]='res-embeddable?', [0x6d]='progress', [0x6e]='enter-span', [0xee]='leave-span',
	[0x6f]='enter-status', [0xef]='leave-status', [0x70]='ext-0', [0x72]='save', [0xf2]='save-undo',
	[0x73]='get-input', [0xf3]='get-key', [0x74]='vm-info', [0x78]='idx-set', [0x79]='check-eq?',
	[0xf9]='check-eq?/vbyte', [0x7a]='check-gt-eq?', [0xfa]='check-gt-eq?/vbyte', [0x7b]='check-gt?',
	[0xfb]='check-gt?/byte', [0x7c]='check-wordmap?', [0x7d]='check-compare?', [0xfd]='check-compare?/vbyte',
	[0x7f]='tracepoint'
}

-- vm_proceed_with_key parameters
local keys = {
	KEY_BACKSPACE = 8,
	KEY_RETURN = 13,
	KEY_UP = 16,
	KEY_DOWN = 17,
	KEY_LEFT = 18,
	KEY_RIGHT = 19
}

local status = {
	quit = 0,
	get_input = 1,
	get_key = 2,
	restore = 3
}

-- Array and byte helpers
local function newbytes(n)
	local a = {length = n}
	for i = 0, n - 1 do a[i] = 0 end
	return a
end

local function slice_bytes(a, from, to)
	local r = {length = to - from}
	for i = 0, (to - from) - 1 do r[i] = a[from + i] end
	return r
end

local function bytes_set(dst, src, offset)
	for i = 0, src.length - 1 do dst[offset + i] = src[i] end
	return dst
end

local function bytes_includes(a, x)
	for i = 0, a.length - 1 do
		if a[i] == x then return true end
	end
	return false
end

local function arr_push(a, v)
	a[a.length] = v
	a.length = a.length + 1
end

local function arr_pop(a)
	a.length = a.length - 1
	local v = a[a.length]
	a[a.length] = nil
	return v
end

local function concat_arr(a, b)
	if type(b) == "table" then
		for i = 0, b.length - 1 do
			a[a.length] = b[i]
			a.length = a.length + 1
		end
	else
		a[a.length] = b
		a.length = a.length + 1
	end
	return a
end

local function getfour(a, o)
	return string.char(a[o], a[o + 1], a[o + 2], a[o + 3])
end

local function get32(a, o)
	return bor(lshift(a[o], 24), lshift(a[o + 1], 16), lshift(a[o + 2], 8), a[o + 3])
end

local function get16(a, o)
	return bor(lshift(a[o], 8), a[o + 1])
end

local function putfour(a, o, str)
	for i = 0, 3 do a[o + i] = string.byte(str, i + 1) end
end

local function put32(a, o, value)
	a[o] = band(rshift(value, 24), 0xff)
	a[o + 1] = band(rshift(value, 16), 0xff)
	a[o + 2] = band(rshift(value, 8), 0xff)
	a[o + 3] = band(value, 0xff)
	return o + 4
end

local function put16(a, o, value)
	a[o] = band(rshift(value, 8), 0xff)
	a[o + 1] = band(value, 0xff)
	return o + 2
end

local function findchunk(filedata, name)
	local size = get32(filedata, 4) + 8
	local pos = 12
	while pos < size do
		local chname = getfour(filedata, pos)
		local chsize = get32(filedata, pos + 4)
		if chname == name then
			return slice_bytes(filedata, pos + 8, pos + 8 + chsize)
		end
		pos = pos + 8 + band(chsize + 1, bnot(1))
	end
	return nil
end

-- UTF-8 helpers
local function fromCharCode(code)
	code = band(code, 0xffff)
	if code < 0x80 then
		return string.char(code)
	elseif code < 0x800 then
		return string.char(0xc0 + rshift(code, 6), 0x80 + band(code, 0x3f))
	else
		return string.char(0xe0 + rshift(code, 12), 0x80 + band(rshift(code, 6), 0x3f), 0x80 + band(code, 0x3f))
	end
end

local function utf8_codepoints(s)
	local cps = {length = 0}
	local i = 1
	local len = #s
	while i <= len do
		local c = string.byte(s, i)
		local cp, adv
		if c < 0x80 then
			cp = c; adv = 1
		elseif c < 0xe0 then
			cp = bor(lshift(band(c, 0x1f), 6), band(string.byte(s, i + 1) or 0, 0x3f)); adv = 2
		elseif c < 0xf0 then
			cp = bor(lshift(band(c, 0x0f), 12), lshift(band(string.byte(s, i + 1) or 0, 0x3f), 6), band(string.byte(s, i + 2) or 0, 0x3f)); adv = 3
		else
			cp = bor(lshift(band(c, 0x07), 18), lshift(band(string.byte(s, i + 1) or 0, 0x3f), 12), lshift(band(string.byte(s, i + 2) or 0, 0x3f), 6), band(string.byte(s, i + 3) or 0, 0x3f)); adv = 4
		end
		cps[cps.length] = cp
		cps.length = cps.length + 1
		i = i + adv
	end
	return cps
end

-- Character and string decoding
local function decodechar(e, aach)
	if e.upper then
		if aach >= 0x61 and aach <= 0x7a then
			aach = bxor(aach, 0x20)
		elseif aach >= 0x80 then
			aach = e.lang[e.extchars + 1 + band(aach, 0x7f) * 5 + 1]
		end
		e.upper = false
	end
	if aach < 0x80 then
		return fromCharCode(aach)
	else
		aach = band(aach, 0x7f)
		if aach >= e.lang[e.extchars] then
			e.upper = false
			return "??"
		else
			local entry = e.extchars + 1 + aach * 5
			local uchar = bor(lshift(e.lang[entry + 2], 16), lshift(e.lang[entry + 3], 8), e.lang[entry + 4])
			return fromCharCode(uchar)
		end
	end
end

local function decodestr(e, addr)
	local decoder = get16(e.lang, 0)
	local state, code, bits, nbit = 0, 0, 0, 0
	local out = {}
	while true do
		if nbit == 0 then bits = e.writ[addr]; addr = addr + 1; nbit = 8 end
		code = e.lang[decoder + lshift(state, 1) + (band(bits, 0x80) ~= 0 and 1 or 0)]
		bits = lshift(bits, 1)
		nbit = nbit - 1
		if code >= 0x81 then
			state = band(code, 0x7f)
		elseif code == 0x80 then
			break
		elseif code == 0x5f then
			code = 0
			for _ = 1, e.esc_bits do
				if nbit == 0 then bits = e.writ[addr]; addr = addr + 1; nbit = 8 end
				code = lshift(code, 1)
				if band(bits, 0x80) ~= 0 then code = bor(code, 1) end
				bits = lshift(bits, 1)
				nbit = nbit - 1
			end
			if e.head[0] == 0 and e.head[1] < 4 then
				out[#out + 1] = decodechar(e, 0x80 + code)
			elseif code < e.esc_boundary then
				out[#out + 1] = decodechar(e, 0xa0 + code)
			else
				out[#out + 1] = " "
				local entry = 2 + (code - e.esc_boundary) * 3
				local len = e.dict[entry]
				local charaddr = bor(lshift(e.dict[entry + 1], 8), e.dict[entry + 2])
				for i = 0, len - 1 do
					out[#out + 1] = decodechar(e, e.dict[charaddr + i])
				end
			end
			state = 0
		else
			out[#out + 1] = decodechar(e, code + 0x20)
			state = 0
		end
	end
	return table.concat(out)
end

-- VM state lifecycle
local function vm_reinit(e)
	e.nob = get16(e.init, 0)
	e.ltb = get16(e.init, 2)
	e.ltt = get16(e.init, 4)
	for i = 0, e.heapdata.length - 1 do e.heapdata[i] = 0x3f3f end
	for i = 0, e.auxdata.length - 1 do e.auxdata[i] = 0x3f3f end
	for i = rshift(e.init.length - 6, 1), e.ramdata.length - 1 do e.ramdata[i] = 0x3f3f end
	local i = 6
	while i < e.init.length do
		e.ramdata[rshift(i - 6, 1)] = get16(e.init, i)
		i = i + 2
	end
end

local function vm_reset(e, arg0, clear_undo)
	e.reg[0] = arg0
	for i = 1, 63 do e.reg[i] = 0 end
	e.inst = 1
	e.cont = 0
	e.top = 0
	e.env = e.heapdata.length
	e.cho = e.heapdata.length
	e.sim = 0xffff
	e.aux = 0
	e.trl = e.auxdata.length
	e.sta = 0
	e.stc = 0
	e.cwl = 0
	e.spc = e.SP_LINE
	e.divs = {length = 0}
	e.upper = false
	e.in_status = false
	if clear_undo then
		e.undodata = {}
		e.pruned_undo = false
	end
	if e.randomseed and e.randomseed ~= 0 then
		e.randomstate = e.randomseed
	else
		e.randomstate = (os.time() * 1000) % 4294967296
	end
end

local function vm_capture_state(e, new_inst)
	local nword = 3 + e.ramdata.length + e.auxdata.length + e.heapdata.length
	local data = newbytes(nword * 2)
	local regs = newbytes(128 + 26 + 2 + e.divs.length * 2)
	local j = 0

	j = put16(data, j, e.nob)
	j = put16(data, j, e.ltb)
	j = put16(data, j, e.ltt)
	for i = 0, e.ramdata.length - 1 do
		j = put16(data, j, (i < e.ltt) and e.ramdata[i] or 0x3f3f)
	end
	for i = 0, e.auxdata.length - 1 do
		j = put16(data, j, (i < e.aux or i >= e.trl) and e.auxdata[i] or 0x3f3f)
	end
	for i = 0, e.heapdata.length - 1 do
		j = put16(data, j, (i < e.top or i >= e.env or i >= e.cho) and e.heapdata[i] or 0x3f3f)
	end

	j = 0
	for i = 0, 63 do j = put16(regs, j, e.reg[i]) end
	j = put32(regs, j, new_inst)
	j = put32(regs, j, e.cont)
	j = put16(regs, j, e.top)
	j = put16(regs, j, e.env)
	j = put16(regs, j, e.cho)
	j = put16(regs, j, e.sim)
	j = put16(regs, j, e.aux)
	j = put16(regs, j, e.trl)
	j = put16(regs, j, e.sta)
	j = put16(regs, j, e.stc)
	regs[j] = band(e.cwl, 0xff); j = j + 1
	regs[j] = band(e.spc, 0xff); j = j + 1
	j = put16(regs, j, e.divs.length)
	for i = 0, e.divs.length - 1 do j = put16(regs, j, e.divs[i]) end

	return {data = data, regs = regs}
end

local function vm_clear_divs(e)
	e.io.leave_all()
	e.in_status = false
	e.n_span = 0
	e.n_link = 0
	e.divs = {length = 0}
end

local function vm_restore_state(e, state)
	local data, regs = state.data, state.regs
	local j = 0

	e.nob = get16(data, j); j = j + 2
	e.ltb = get16(data, j); j = j + 2
	e.ltt = get16(data, j); j = j + 2
	for i = 0, e.ramdata.length - 1 do e.ramdata[i] = get16(data, j); j = j + 2 end
	for i = 0, e.auxdata.length - 1 do e.auxdata[i] = get16(data, j); j = j + 2 end
	for i = 0, e.heapdata.length - 1 do e.heapdata[i] = get16(data, j); j = j + 2 end

	j = 0
	for i = 0, 63 do e.reg[i] = get16(regs, j); j = j + 2 end
	e.inst = get32(regs, j); j = j + 4
	e.cont = get32(regs, j); j = j + 4
	e.top = get16(regs, j); j = j + 2
	e.env = get16(regs, j); j = j + 2
	e.cho = get16(regs, j); j = j + 2
	e.sim = get16(regs, j); j = j + 2
	e.aux = get16(regs, j); j = j + 2
	e.trl = get16(regs, j); j = j + 2
	e.sta = get16(regs, j); j = j + 2
	e.stc = get16(regs, j); j = j + 2
	e.cwl = regs[j]; j = j + 1
	e.spc = regs[j]; j = j + 1
	local ndiv = get16(regs, j); j = j + 2
	e.divs = {length = 0}
	for i = 0, ndiv - 1 do
		e.divs[i] = get16(regs, j); j = j + 2
		e.divs.length = i + 1
		e.io.enter_div(e.divs[i])
	end
end

local function xat(ref, st, idx)
	local a = ref[idx]; if a == nil then a = 0 end
	local b = st[idx]; if b == nil then b = 0 end
	return bxor(a, b)
end

local function vm_rlenc_state(reference, state)
	local bytes, nz = 0, 0

	for i = 0, reference.data.length - 1 do
		if bxor(reference.data[i], state.data[i]) ~= 0 then
			bytes = bytes + 1; nz = 0
		else
			if nz ~= 0 and nz < 0x100 then
				nz = nz + 1
			else
				bytes = bytes + 2; nz = 1
			end
		end
	end

	local encoded = newbytes(bytes)
	local j = 0
	local i = 0
	while i < reference.data.length do
		local diff = bxor(reference.data[i], state.data[i])
		if diff ~= 0 then
			encoded[j] = diff; j = j + 1
		else
			encoded[j] = 0; j = j + 1
			nz = 1
			while nz < 0x100 and xat(reference.data, state.data, i + nz) == 0 do
				nz = nz + 1
			end
			encoded[j] = nz - 1; j = j + 1
			i = i + nz - 1
		end
		i = i + 1
	end

	return {rledata = encoded, regs = state.regs}
end

local function vm_rldec_state(reference, encoded)
	local array = newbytes(reference.data.length)
	local j = 0
	local i = 0
	while i < encoded.rledata.length do
		local diff = encoded.rledata[i]
		if diff ~= 0 then
			array[j] = bxor(reference.data[j] or 0, diff); j = j + 1
		else
			i = i + 1
			local nz = encoded.rledata[i] + 1
			while nz > 0 do
				nz = nz - 1
				array[j] = reference.data[j] or 0; j = j + 1
			end
		end
		i = i + 1
	end

	return {data = array, regs = encoded.regs}
end

local function vm_wrap_savefile(e, encoded)
	local function makechunk(tag, array)
		local size = band(array.length + 1, bnot(1))
		local result = newbytes(8 + size)
		putfour(result, 0, tag)
		put32(result, 4, array.length)
		bytes_set(result, array, 8)
		return result
	end

	local head = makechunk("HEAD", e.head)
	local data = makechunk("DATA", encoded.rledata)
	local regs = makechunk("REGS", encoded.regs)
	local size = 4 + head.length + data.length + regs.length
	local result = newbytes(8 + size)

	putfour(result, 0, "FORM")
	put32(result, 4, size)
	putfour(result, 8, "AASV")
	bytes_set(result, head, 12)
	bytes_set(result, data, 12 + head.length)
	bytes_set(result, regs, 12 + head.length + data.length)

	return result
end

local function vm_unwrap_savefile(e, filedata)
	if getfour(filedata, 0) ~= "FORM" or getfour(filedata, 8) ~= "AASV" then
		e.io.print("Not an aasave file!")
		e.io.line()
		return nil
	end
	local head = findchunk(filedata, "HEAD")
	local data = findchunk(filedata, "DATA")
	local regs = findchunk(filedata, "REGS")
	if not head or not data or not regs then
		e.io.print("Incomplete aasave file!")
		e.io.line()
		return nil
	end
	local i = 0
	while i < head.length and i < e.head.length do
		if head[i] ~= e.head[i] then break end
		i = i + 1
	end
	if i ~= head.length or i ~= e.head.length then
		e.io.print("This savefile is from another story (or another version of the present story).")
		e.io.line()
		return nil
	end
	return {rledata = data, regs = regs}
end

-- Resources
local function combine_arrays(oldarr, newval)
	if oldarr == true then
		return newval
	elseif type(oldarr) == "table" then
		oldarr[oldarr.length] = newval
		oldarr.length = oldarr.length + 1
		return oldarr
	else
		return {length = 2, [0] = oldarr, [1] = newval}
	end
end

local function split_opts(opts)
	local t = {}
	for token in (opts .. ","):gmatch("([^,]*),") do
		local trimmed = token:gsub("^%s+", ""):gsub("%s+$", "")
		if trimmed ~= "" then t[#t + 1] = trimmed end
	end
	return t
end

local function get_res(e, id)
	local obj = {url = "", alt = "", options = {}}
	local opts = ""
	if e.urls then
		local n = get16(e.urls, 0)
		if id < n then
			local offs = get16(e.urls, 2 + id * 2)
			obj.alt = decodestr(e, lshift(bor(lshift(e.urls[offs], 16), lshift(e.urls[offs + 1], 8), e.urls[offs + 2]), e.strshift))
			local i = 3
			while e.urls[offs + i] ~= 0 do
				obj.url = obj.url .. string.char(e.urls[offs + i])
				i = i + 1
			end
			i = i + 1
			while e.urls[offs + i] ~= 0 do
				opts = opts .. string.char(e.urls[offs + i])
				i = i + 1
			end
		end
	end
	for _, opt in ipairs(split_opts(opts)) do
		local k, v = opt:match("^(.-)%s*:%s*(.*)$")
		if k then
			if obj.options[k] ~= nil then
				v = combine_arrays(obj.options[k], v)
			end
			obj.options[k] = v
		else
			if obj.options[opt] == nil then
				obj.options[opt] = true
			end
		end
	end
	return obj
end

local function get_resources(e)
	local ress = {length = 0}
	if not e.urls then return ress end
	local n = get16(e.urls, 0)
	for i = 0, n - 1 do
		ress[ress.length] = get_res(e, i)
		ress.length = ress.length + 1
	end
	return ress
end

-- Styles and metadata
local function get_styles(e)
	local styles = {length = 0}
	local n = get16(e.look, 0)
	for i = 0, n - 1 do
		local offs = get16(e.look, 2 + i * 2)
		local map = {}
		while e.look[offs] ~= 0 do
			local chars = {}
			while true do
				local c = e.look[offs]; offs = offs + 1
				if c == 0 then break end
				chars[#chars + 1] = string.char(c)
			end
			local str = table.concat(chars)
			local ci = str:find(":", 1, true)
			if ci and ci > 1 then
				local key = str:sub(1, ci - 1)
				local p = ci + 1
				while str:sub(p, p) == " " do p = p + 1 end
				map[key] = str:sub(p)
			end
		end
		styles[styles.length] = map
		styles.length = styles.length + 1
	end
	return styles
end

local function get_metadata(e)
	local result = {title = "Untitled story", release = get16(e.head, 4)}
	local keynames = {"title", "author", "noun", "blurb", "date", "compiler"}
	if e.meta then
		local offs = 1
		for _ = 0, e.meta[0] - 1 do
			local key = e.meta[offs]; offs = offs + 1
			local val = {}
			while true do
				local ch = e.meta[offs]; offs = offs + 1
				if ch == 0 then break end
				val[#val + 1] = decodechar(e, ch)
			end
			if key >= 1 and key <= #keynames then
				result[keynames[key]] = table.concat(val)
			end
		end
	end
	return result
end

-- VM core helpers
local function create_pair(e, head, tail)
	local addr = e.top
	e.top = e.top + 2
	if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
	e.heapdata[addr + 0] = head
	e.heapdata[addr + 1] = tail
	return bor(addr, 0xc000)
end

local function deref(e, v)
	while band(v, 0xe000) == 0x8000 do
		local t = e.heapdata[band(v, 0x1fff)]
		if t == 0 then return v end
		v = t
	end
	return v
end

local function fail(e)
	e.inst = bor(lshift(e.heapdata[e.cho + 4], 16), e.heapdata[e.cho + 5])
end

local function unify(e, a, b)
	while true do
		a = deref(e, a)
		b = deref(e, b)
		if band(a, 0xe000) == 0x8000 and band(b, 0xe000) == 0x8000 then
			if e.trl <= e.aux then error(AUXFULL, 0) end
			if a < b then
				e.trl = e.trl - 1
				e.auxdata[e.trl] = band(b, 0x1fff)
				e.heapdata[band(b, 0x1fff)] = a
			elseif a > b then
				e.trl = e.trl - 1
				e.auxdata[e.trl] = band(a, 0x1fff)
				e.heapdata[band(a, 0x1fff)] = b
			end
			return true
		elseif band(a, 0xe000) == 0x8000 then
			if e.trl <= e.aux then error(AUXFULL, 0) end
			e.trl = e.trl - 1
			e.auxdata[e.trl] = band(a, 0x1fff)
			e.heapdata[band(a, 0x1fff)] = b
			return true
		elseif band(b, 0xe000) == 0x8000 then
			if e.trl <= e.aux then error(AUXFULL, 0) end
			e.trl = e.trl - 1
			e.auxdata[e.trl] = band(b, 0x1fff)
			e.heapdata[band(b, 0x1fff)] = a
			return true
		elseif a >= 0xe000 and b >= 0xe000 then
			a = e.heapdata[band(a, 0x1fff)]
			b = e.heapdata[band(b, 0x1fff)]
		elseif a >= 0xe000 then
			a = e.heapdata[band(a, 0x1fff)]
		elseif b >= 0xe000 then
			b = e.heapdata[band(b, 0x1fff)]
		elseif a == b then
			return true
		elseif a >= 0xc000 and b >= 0xc000 then
			if not unify(e, a - 0x4000, b - 0x4000) then return false end
			a = a - 0x3fff
			b = b - 0x3fff
		else
			return false
		end
	end
end

local function would_unify(e, a, b)
	while true do
		a = deref(e, a)
		b = deref(e, b)
		if band(a, 0xe000) == 0x8000 or band(b, 0xe000) == 0x8000 then
			return true
		elseif a >= 0xe000 and b >= 0xe000 then
			a = e.heapdata[band(a, 0x1fff)]
			b = e.heapdata[band(b, 0x1fff)]
		elseif a >= 0xe000 then
			a = e.heapdata[band(a, 0x1fff)]
		elseif b >= 0xe000 then
			b = e.heapdata[band(b, 0x1fff)]
		elseif a == b then
			return true
		elseif a >= 0xc000 and b >= 0xc000 then
			if not would_unify(e, a - 0x4000, b - 0x4000) then return false end
			a = a - 0x3fff
			b = b - 0x3fff
		else
			return false
		end
	end
end

local function destvalue(e, dest)
	if band(dest, 0x40) ~= 0 then
		return e.heapdata[e.env + 4 + band(dest, 0x3f)]
	else
		return e.reg[band(dest, 0x3f)]
	end
end

local function store(e, dest, src)
	if dest >= 0xc0 then
		if not unify(e, e.heapdata[e.env + 4 + band(dest, 0x3f)], src) then fail(e) end
	elseif dest >= 0x80 then
		if not unify(e, e.reg[band(dest, 0x3f)], src) then fail(e) end
	elseif dest >= 0x40 then
		e.heapdata[e.env + 4 + band(dest, 0x3f)] = band(src, 0xffff)
	else
		e.reg[dest] = band(src, 0xffff)
	end
end

local function push_cho(e, narg, nextv)
	local addr = (e.env < e.cho and e.env or e.cho) - 9 - narg
	if addr < e.top then error(HEAPFULL, 0) end
	e.heapdata[addr + 0] = e.env
	e.heapdata[addr + 1] = e.sim
	e.heapdata[addr + 2] = rshift(e.cont, 16)
	e.heapdata[addr + 3] = band(e.cont, 0xffff)
	e.heapdata[addr + 4] = rshift(nextv, 16)
	e.heapdata[addr + 5] = band(nextv, 0xffff)
	e.heapdata[addr + 6] = e.cho
	e.heapdata[addr + 7] = e.top
	e.heapdata[addr + 8] = e.trl
	for i = 0, narg - 1 do
		e.heapdata[addr + 9 + i] = e.reg[i]
	end
	e.cho = addr
end

local function push_aux(e, v)
	v = deref(e, v)
	if v >= 0xe000 then
		push_aux(e, e.heapdata[band(v, 0x1fff) + 1])
		push_aux(e, e.heapdata[band(v, 0x1fff) + 0])
		v = 0x8100
	elseif v >= 0xc000 then
		local count = 0
		while true do
			push_aux(e, v - 0x4000)
			count = count + 1
			v = deref(e, v - 0x3fff)
			if v == 0x3f00 then
				v = bor(0xc000, count); break
			elseif band(v, 0xe000) ~= 0xc000 then
				push_aux(e, v)
				v = bor(0xe000, count); break
			end
		end
	elseif v >= 0x8000 then
		v = 0x8000
	end
	if e.aux >= e.trl then error(AUXFULL, 0) end
	e.auxdata[e.aux] = v
	e.aux = e.aux + 1
end

local function pop_aux(e)
	e.aux = e.aux - 1
	local v = e.auxdata[e.aux]
	local addr, count
	if v == 0x8000 then
		addr = e.top; e.top = e.top + 1
		if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
		e.heapdata[addr] = 0
		v = bor(0x8000, addr)
	elseif v == 0x8100 then
		addr = e.top; e.top = e.top + 2
		if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
		e.heapdata[addr + 0] = pop_aux(e)
		e.heapdata[addr + 1] = pop_aux(e)
		v = bor(0xe000, addr)
	elseif v >= 0xc000 then
		count = band(v, 0x1fff)
		if band(v, 0x2000) ~= 0 then v = pop_aux(e) else v = 0x3f00 end
		while count > 0 do
			count = count - 1
			addr = e.top; e.top = e.top + 2
			if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
			e.heapdata[addr + 0] = pop_aux(e)
			e.heapdata[addr + 1] = v
			v = bor(0xc000, addr)
		end
	end
	return v
end

local function pop_aux_list(e)
	local list = 0x3f00
	while true do
		local v = pop_aux(e)
		if v == 0 then break end
		local addr = e.top; e.top = e.top + 2
		if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
		e.heapdata[addr + 0] = v
		e.heapdata[addr + 1] = list
		list = bor(0xc000, addr)
	end
	return list
end

local function fieldaddr(e, field, obj)
	obj = deref(e, obj)
	if obj > e.nob then
		error(EXPECTOBJ, 0)
	else
		return e.ramdata[obj] + field
	end
end

local function readfield(e, field, obj)
	obj = deref(e, obj)
	if obj > e.nob then
		return 0
	else
		return e.ramdata[e.ramdata[obj] + field]
	end
end

local function unlink(e, root_addr, nextv, key)
	if key == 0 or key >= 0x2000 then return end
	local tail = e.ramdata[fieldaddr(e, nextv, key)]
	local addr = root_addr
	while e.ramdata[addr] ~= 0 do
		if e.ramdata[addr] == key then
			e.ramdata[addr] = tail
			return
		end
		addr = fieldaddr(e, nextv, e.ramdata[addr])
	end
end

local function pop_lts(e)
	e.tmp = e.tmp - 1
	local v = e.ramdata[e.tmp]
	local addr, count
	if v == 0x8100 then
		addr = e.top; e.top = e.top + 2
		if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
		e.heapdata[addr + 0] = pop_lts(e)
		e.heapdata[addr + 1] = pop_lts(e)
		v = bor(0xe000, addr)
	elseif v >= 0xc000 then
		count = band(v, 0x1fff)
		if band(v, 0x2000) ~= 0 then v = pop_lts(e) else v = 0x3f00 end
		while count > 0 do
			count = count - 1
			addr = e.top; e.top = e.top + 2
			if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
			e.heapdata[addr + 0] = pop_lts(e)
			e.heapdata[addr + 1] = v
			v = bor(0xc000, addr)
		end
	end
	return v
end

local function push_lts(e, v)
	v = deref(e, v)
	if v >= 0xe000 then
		push_lts(e, e.heapdata[band(v, 0x1fff) + 1])
		push_lts(e, e.heapdata[band(v, 0x1fff) + 0])
		v = 0x8100
	elseif v >= 0xc000 then
		local count = 0
		while true do
			push_lts(e, v - 0x4000)
			count = count + 1
			v = deref(e, v - 0x3fff)
			if v == 0x3f00 then
				v = bor(0xc000, count); break
			elseif band(v, 0xe000) ~= 0xc000 then
				push_lts(e, v)
				v = bor(0xe000, count); break
			end
		end
	elseif v >= 0x8000 then
		error(EXPECTBOUND, 0)
	end
	if e.tmp > e.ramdata.length then error(LTSFULL, 0) end
	e.ramdata[e.tmp] = v
	e.tmp = e.tmp + 1
end

local function clear_lts(e, addr)
	local v = e.ramdata[addr]
	if band(v, 0x8000) ~= 0 then
		e.ramdata[addr] = 0
		v = band(v, 0x7fff)
		local size = e.ramdata[v]
		for i = v, e.ltt - size - 1 do
			e.ramdata[i] = e.ramdata[i + size]
		end
		e.ltt = e.ltt - size
		while v < e.ltt do
			e.ramdata[e.ramdata[v + 1]] = e.ramdata[e.ramdata[v + 1]] - size
			v = v + e.ramdata[v]
		end
	end
end

local function get_lts(e, v)
	if band(v, 0x8000) ~= 0 then
		e.tmp = band(v, 0x7fff)
		e.tmp = e.tmp + e.ramdata[e.tmp]
		return pop_lts(e)
	else
		return v
	end
end

local function put_lts(e, addr, v)
	clear_lts(e, addr)
	v = deref(e, v)
	if v < 0x8000 then
		e.ramdata[addr] = v
	else
		e.tmp = e.ltt + 2
		if e.tmp > e.ramdata.length then error(LTSFULL, 0) end
		push_lts(e, v)
		e.ramdata[addr] = bor(0x8000, e.ltt)
		e.ramdata[e.ltt + 0] = e.tmp - e.ltt
		e.ramdata[e.ltt + 1] = addr
		e.ltt = e.tmp
	end
end

local function val2str(e, v)
	v = deref(e, v)
	local str
	if v >= 0xe000 then
		str = ""
		for i = 0, 1 do
			local x = e.heapdata[band(v, 0x1fff) + i]
			if x >= 0x3f00 then
				while x >= 0xc000 do
					str = str .. val2str(e, e.heapdata[band(x, 0x1fff)])
					x = e.heapdata[band(x, 0x1fff) + 1]
				end
			else
				str = str .. val2str(e, x)
			end
		end
	elseif v >= 0xc000 then
		local needsp = false
		e.upper = false
		str = "["
		while band(v, 0xe000) == 0xc000 do
			if needsp then str = str .. " " end
			str = str .. val2str(e, v - 0x4000)
			needsp = true
			v = deref(e, v - 0x3fff)
		end
		if v == 0x3f00 then
			str = str .. "]"
		else
			str = str .. " | " .. val2str(e, v) .. "]"
		end
	elseif v >= 0x8000 then
		e.upper = false
		str = "$"
	elseif v >= 0x4000 then
		e.upper = false
		str = tostring(band(v, 0x3fff))
	elseif v >= 0x3f00 then
		e.upper = false
		str = "[]"
	elseif v >= 0x3e00 then
		str = decodechar(e, band(v, 0xff))
	elseif v >= 0x2000 then
		local entry = 2 + band(v, 0x1fff) * 3
		local len = e.dict[entry]
		local addr = bor(lshift(e.dict[entry + 1], 8), e.dict[entry + 2])
		str = ""
		for i = 0, len - 1 do
			str = str .. decodechar(e, e.dict[addr + i])
		end
	elseif v ~= 0 then
		e.upper = false
		str = "#"
		if e.tags then
			local addr = get16(e.tags, v * 2)
			while true do
				local c = e.tags[addr]; addr = addr + 1
				if c == 0 then break end
				str = str .. decodechar(e, c)
			end
		end
	else
		str = ""
	end
	return str
end

local function wordmap(e, mapnum, v)
	local map = get16(e.maps, 2 + mapnum * 2)
	local start = 0
	local endd = get16(e.maps, map)
	while start < endd do
		local mid = rshift(start + endd, 1)
		local midval = get16(e.maps, map + 2 + mid * 4)
		if midval == v then
			local ptr = get16(e.maps, map + 4 + mid * 4)
			if ptr == 0 then
				return false
			elseif band(ptr, 0xe000) ~= 0 then
				if e.aux >= e.trl then error(AUXFULL, 0) end
				e.auxdata[e.aux] = band(ptr, 0x1fff)
				e.aux = e.aux + 1
				return true
			else
				while true do
					local o = e.maps[ptr]; ptr = ptr + 1
					if o == 0 then break end
					if e.aux >= e.trl then error(AUXFULL, 0) end
					if o >= 0xe0 then
						o = bor(lshift(band(o, 0x1f), 8), e.maps[ptr]); ptr = ptr + 1
					end
					e.auxdata[e.aux] = o
					e.aux = e.aux + 1
				end
				return true
			end
		elseif midval > v then
			endd = mid
		else
			start = mid + 1
		end
	end
	return true
end

local function compat_rand(e)
	local rs = e.randomstate
	local high = math.floor(rs / 65536) % 65536
	local low = rs % 65536
	local newhigh = (0x15a * low + 0x4e35 * high) % 65536
	rs = (newhigh * 65536 + 0x4e35 * low + 1) % 4294967296
	e.randomstate = rs
	return math.floor(rs / 65536) % 32768
end

local function makepairsub(e, literal, arg, addr)
	if literal then
		e.heapdata[addr] = arg
	elseif band(arg, 0x80) ~= 0 then
		e.heapdata[addr] = destvalue(e, arg)
	else
		e.heapdata[addr] = 0
		store(e, arg, bor(0x8000, addr))
	end
end

local function makepair(e, a1val, a1, a2, a3)
	local addr
	if band(a3, 0x80) ~= 0 then
		a3 = deref(e, destvalue(e, a3))
		if band(a3, 0xe000) == 0xc000 then
			if a1val then
				if not unify(e, a1, a3 - 0x4000) then fail(e) end
			else
				store(e, a1, a3 - 0x4000)
			end
			store(e, a2, a3 - 0x3fff)
		elseif band(a3, 0xe000) == 0x8000 then
			addr = e.top; e.top = e.top + 2
			if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
			makepairsub(e, a1val, a1, addr)
			makepairsub(e, false, a2, addr + 1)
			unify(e, a3, bor(0xc000, addr))
		else
			fail(e)
		end
	else
		addr = e.top; e.top = e.top + 2
		if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
		makepairsub(e, a1val, a1, addr)
		makepairsub(e, false, a2, addr + 1)
		store(e, a3, bor(0xc000, addr))
	end
end

local function prepend_chars(e, v, list)
	local entry = 2 + band(v, 0x1fff) * 3
	local len = e.dict[entry]
	local addr = bor(lshift(e.dict[entry + 1], 8), e.dict[entry + 2])
	for i = len - 1, 0, -1 do
		local ch = e.dict[addr + i]
		if ch >= 0x30 and ch <= 0x39 then
			ch = ch + 0x4000 - 0x30
		else
			ch = bor(ch, 0x3e00)
		end
		list = create_pair(e, ch, list)
	end
	return list
end

local function words_to_charlist(e, list)
	local buf = {length = 0}
	repeat
		local v = deref(e, e.heapdata[band(list, 0x1fff) + 0])
		if v >= 0xe000 then
			local part1 = e.heapdata[band(v, 0x1fff) + 0]
			if part1 >= 0x8000 then
				buf = concat_arr(buf, words_to_charlist(e, part1))
			else
				buf = concat_arr(buf, words_to_charlist(e, v))
			end
		elseif v >= 0x4000 and v < 0x8000 then
			local s = tostring(band(v, 0x3fff))
			for i = 1, #s do arr_push(buf, string.byte(s, i)) end
		elseif v >= 0x3e00 and v < 0x3f00 then
			local ch = band(v, 0xff)
			if ch <= 0x20 then return 0 end
			if bytes_includes(e.stopchars, ch) then return 0 end
			arr_push(buf, ch)
		elseif v >= 0x2000 and v < 0x3e00 then
			local entry = 2 + band(v, 0x1fff) * 3
			local len = e.dict[entry]
			local addr = bor(lshift(e.dict[entry + 1], 8), e.dict[entry + 2])
			for i = 0, len - 1 do arr_push(buf, e.dict[addr + i]) end
		else
			return 0
		end
		list = deref(e, e.heapdata[band(list, 0x1fff) + 1])
	until band(list, 0xe000) ~= 0xc000
	if list ~= 0x3f00 then return 0 end
	return buf
end

-- Bytecode value fetchers
local function fvalue(e)
	local v = e.code[e.inst]; e.inst = e.inst + 1
	if v >= 0xc0 then
		return e.heapdata[e.env + 4 + band(v, 0x3f)]
	elseif v >= 0x80 then
		return e.reg[band(v, 0x3f)]
	else
		local lo = e.code[e.inst]; e.inst = e.inst + 1
		return bor(lshift(v, 8), lo)
	end
end

local function findex(e)
	local v = e.code[e.inst]; e.inst = e.inst + 1
	if v >= 0xc0 then
		local lo = e.code[e.inst]; e.inst = e.inst + 1
		return bor(lshift(band(v, 0x3f), 8), lo)
	else
		return v
	end
end

local function fcode(e)
	local v = e.code[e.inst]; e.inst = e.inst + 1
	if v == 0 then
		return 0
	elseif v < 0x40 then
		return e.inst + v
	elseif v < 0x80 then
		v = bor(lshift(band(v, 0x3f), 8), e.code[e.inst]); e.inst = e.inst + 1
		if band(v, 0x2000) ~= 0 then
			return e.inst + v - 0x4000
		else
			return e.inst + v
		end
	else
		v = bor(lshift(band(v, 0x7f), 16), lshift(e.code[e.inst], 8)); e.inst = e.inst + 1
		local r = bor(v, e.code[e.inst]); e.inst = e.inst + 1
		return r
	end
end

local function fstring(e)
	local v = e.code[e.inst]; e.inst = e.inst + 1
	if v >= 0xc0 then
		v = bor(lshift(band(v, 0x3f), 16), lshift(e.code[e.inst], 8)); e.inst = e.inst + 1
		v = bor(v, e.code[e.inst]); e.inst = e.inst + 1
		return lshift(v, e.strshift)
	elseif v >= 0x80 then
		v = bor(lshift(band(v, 0x3f), 8), e.code[e.inst]); e.inst = e.inst + 1
		return lshift(v, e.strshift)
	else
		return lshift(v, 1)
	end
end

local function fword(e)
	local v = e.code[e.inst]; e.inst = e.inst + 1
	local lo = e.code[e.inst]; e.inst = e.inst + 1
	return bor(lshift(v, 8), lo)
end

local function nextbyte(e)
	local b = e.code[e.inst]; e.inst = e.inst + 1
	return b
end

-- Word parsing
local function parse_word(e, chars)
	local len = chars.length
	local enddecoder = get16(e.lang, 4)
	local rev_ending = {length = 0}

	local function buildlist(list)
		local v = 0x3f00
		for i = 0, list.length - 1 do
			local ch = list[i]
			if ch >= 0x30 and ch <= 0x39 then
				v = create_pair(e, ch + 0x4000 - 0x30, v)
			else
				v = create_pair(e, bor(0x3e00, ch), v)
			end
		end
		return v
	end

	local function finddict()
		local start = 0
		local endd = get16(e.dict, 0)
		local diff = 0
		while start < endd do
			local mid = rshift(start + endd, 1)
			local dictlen = e.dict[2 + 3 * mid]
			local dictoffs = get16(e.dict, 2 + 3 * mid + 1)
			local i = 0
			diff = 0
			while i < len and i < dictlen do
				diff = chars[i] - e.dict[dictoffs + i]
				if diff ~= 0 then break end
				i = i + 1
			end
			if i == dictlen and i == len then
				if diff == 0 then return bor(0x2000, mid) end
			elseif i == dictlen then
				diff = 1
			elseif i == len then
				diff = -1
			end
			if diff < 0 then endd = mid else start = mid + 1 end
		end
		return 0
	end

	local v
	if len > 1 then
		v = finddict()
		if v ~= 0 then return v end
	end

	v = 0
	do
		local i = 0
		while i < chars.length do
			if chars[i] < 0x30 or chars[i] > 0x39 then break end
			v = v * 10 + chars[i] - 0x30
			if v >= 16384 then break end
			i = i + 1
		end
		if i == chars.length then
			return bor(0x4000, v)
		end
	end

	if len == 1 then
		return bor(0x3e00, chars[0])
	end

	local state = 0
	while true do
		local instr = e.lang[enddecoder + state]; state = state + 1
		if instr == 0 then
			while len > 0 do len = len - 1; arr_push(rev_ending, chars[len]) end
			return bor(create_pair(e, buildlist(rev_ending), 0x3f00), 0xe000)
		elseif instr == 1 then
			v = finddict()
			if v ~= 0 then
				return bor(create_pair(e, v, buildlist(rev_ending)), 0xe000)
			end
		else
			local nxt = e.lang[enddecoder + state]; state = state + 1
			if len > 2 and instr == chars[len - 1] then
				arr_push(rev_ending, instr)
				len = len - 1
				state = nxt
			end
		end
	end
end

-- Opcode handlers
local ophandlers = {}

ophandlers[0x00] = function(e, op) end

ophandlers[0x01] = function(e, op)
	fail(e)
end

ophandlers[0x02] = function(e, op)
	e.cont = fcode(e)
end

ophandlers[0x03] = function(e, op)
	if e.sim < 0x8000 then e.cho = e.sim end
	e.inst = e.cont
end

ophandlers[0x04] = function(e, op)
	e.inst = fcode(e)
end

ophandlers[0x05] = function(e, op)
	e.sim = 0xffff
	e.inst = fcode(e)
end

ophandlers[0x85] = function(e, op)
	local a1 = fcode(e)
	e.cont = e.inst
	e.sim = 0xffff
	e.inst = a1
end

ophandlers[0x06] = function(e, op)
	e.sim = e.cho
	e.inst = fcode(e)
end

ophandlers[0x86] = function(e, op)
	local a1 = fcode(e)
	e.cont = e.inst
	e.sim = e.cho
	e.inst = a1
end

ophandlers[0x07] = function(e, op)
	if e.sim >= 0x8000 then e.sim = e.cho end
	e.inst = fcode(e)
end

ophandlers[0x87] = function(e, op)
	if e.sim >= 0x8000 then e.sim = e.cho end
end

ophandlers[0x08] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or nextbyte(e)
	local addr = (e.env < e.cho and e.env or e.cho) - 4 - a1
	if addr < e.top then error(HEAPFULL, 0) end
	e.heapdata[addr + 0] = e.env
	e.heapdata[addr + 1] = e.sim
	e.heapdata[addr + 2] = rshift(e.cont, 16)
	e.heapdata[addr + 3] = band(e.cont, 0xffff)
	e.env = addr
end
ophandlers[0x88] = ophandlers[0x08]

ophandlers[0x09] = function(e, op)
	e.cont = bor(lshift(e.heapdata[e.env + 2], 16), e.heapdata[e.env + 3])
	e.sim = e.heapdata[e.env + 1]
	e.env = e.heapdata[e.env + 0]
end

ophandlers[0x89] = function(e, op)
	e.inst = bor(lshift(e.heapdata[e.env + 2], 16), e.heapdata[e.env + 3])
	if e.heapdata[e.env + 1] < 0x8000 then e.cho = e.heapdata[e.env + 1] end
	e.env = e.heapdata[e.env + 0]
end

ophandlers[0x0a] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or nextbyte(e)
	push_cho(e, a1, fcode(e))
end
ophandlers[0x8a] = ophandlers[0x0a]

ophandlers[0x0b] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or nextbyte(e)
	for i = 0, a1 - 1 do
		e.reg[i] = e.heapdata[e.cho + 9 + i]
	end
	while e.trl < e.heapdata[e.cho + 8] do
		e.heapdata[e.auxdata[e.trl]] = 0
		e.trl = e.trl + 1
	end
	e.top = e.heapdata[e.cho + 7]
	e.cont = bor(lshift(e.heapdata[e.cho + 2], 16), e.heapdata[e.cho + 3])
	e.sim = e.heapdata[e.cho + 1]
	e.env = e.heapdata[e.cho + 0]
	e.cho = e.heapdata[e.cho + 6]
end
ophandlers[0x8b] = ophandlers[0x0b]

ophandlers[0x0c] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or nextbyte(e)
	local a2 = fcode(e)
	e.heapdata[e.cho + 4] = rshift(a2, 16)
	e.heapdata[e.cho + 5] = band(a2, 0xffff)
	for i = 0, a1 - 1 do
		e.reg[i] = e.heapdata[e.cho + 9 + i]
	end
	while e.trl < e.heapdata[e.cho + 8] do
		e.heapdata[e.auxdata[e.trl]] = 0
		e.trl = e.trl + 1
	end
	e.top = e.heapdata[e.cho + 7]
	e.cont = bor(lshift(e.heapdata[e.cho + 2], 16), e.heapdata[e.cho + 3])
	e.sim = e.heapdata[e.cho + 1]
	e.env = e.heapdata[e.cho + 0]
end
ophandlers[0x8c] = ophandlers[0x0c]

ophandlers[0x0d] = function(e, op)
	e.cho = e.heapdata[e.cho + 6]
end

ophandlers[0x0e] = function(e, op)
	store(e, nextbyte(e), e.cho)
end

ophandlers[0x0f] = function(e, op)
	e.cho = fvalue(e)
end

ophandlers[0x10] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fvalue(e)
	local a2 = nextbyte(e)
	store(e, a2, a1)
end
ophandlers[0x90] = ophandlers[0x10]

ophandlers[0x11] = function(e, op)
	local addr = e.top; e.top = e.top + 1
	if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
	e.heapdata[addr] = 0
	store(e, nextbyte(e), bor(0x8000, addr))
end

ophandlers[0x12] = function(e, op)
	local a1 = nextbyte(e)
	local a2 = nextbyte(e)
	local a3 = nextbyte(e)
	makepair(e, false, a1, a2, a3)
end

ophandlers[0x13] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a2 = nextbyte(e)
	local a3 = nextbyte(e)
	makepair(e, true, a1, a2, a3)
end
ophandlers[0x93] = ophandlers[0x13]

ophandlers[0x14] = function(e, op)
	push_aux(e, fvalue(e))
end

ophandlers[0x94] = function(e, op)
	if e.aux >= e.trl then error(AUXFULL, 0) end
	e.auxdata[e.aux] = 0; e.aux = e.aux + 1
end

ophandlers[0x15] = function(e, op)
	if e.aux >= e.trl then error(AUXFULL, 0) end
	e.auxdata[e.aux] = fword(e); e.aux = e.aux + 1
end

ophandlers[0x95] = function(e, op)
	if e.aux >= e.trl then error(AUXFULL, 0) end
	e.auxdata[e.aux] = nextbyte(e); e.aux = e.aux + 1
end

ophandlers[0x16] = function(e, op)
	store(e, nextbyte(e), pop_aux(e))
end

ophandlers[0x17] = function(e, op)
	store(e, nextbyte(e), pop_aux_list(e))
end

ophandlers[0x18] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local flag = false
	while true do
		e.aux = e.aux - 1
		local v = e.auxdata[e.aux]
		if v == 0 then break end
		if v == a1 then flag = true end
	end
	if not flag then fail(e) end
end

ophandlers[0x19] = function(e, op)
	local saved = e.top
	local a1 = deref(e, fvalue(e))
	local v = pop_aux_list(e)
	while band(a1, 0xe000) == 0xc000 do
		local iter = v
		local match = false
		while band(iter, 0xe000) == 0xc000 and not match do
			if would_unify(e, iter - 0x4000, a1 - 0x4000) then
				match = true
			end
			iter = deref(e, iter - 0x3fff)
		end
		if not match then
			fail(e)
			break
		end
		a1 = deref(e, a1 - 0x3fff)
	end
	e.top = saved
end

ophandlers[0x1b] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	local v = 0x3f00
	if a1 ~= a2 and band(a1, 0xe000) == 0xc000 then
		local curr = e.top
		v = bor(0xc000, curr)
		while true do
			e.top = e.top + 2
			if e.top > e.env or e.top > e.cho then error(HEAPFULL, 0) end
			e.heapdata[curr + 0] = e.heapdata[band(a1, 0x1fff)]
			a1 = deref(e, a1 - 0x3fff)
			if a1 == a2 or band(a1, 0xe000) ~= 0xc000 then
				break
			end
			e.heapdata[curr + 1] = bor(0xc000, e.top)
			curr = e.top
		end
		e.heapdata[curr + 1] = 0x3f00
	end
	store(e, nextbyte(e), v)
end

ophandlers[0x1c] = function(e, op)
	e.cho = e.stc
	e.inst = bor(lshift(e.heapdata[e.cho + 4], 16), e.heapdata[e.cho + 5])
end

ophandlers[0x1d] = function(e, op)
	if e.aux + 2 > e.trl then error(AUXFULL, 0) end
	e.auxdata[e.aux] = e.stc; e.aux = e.aux + 1
	e.auxdata[e.aux] = e.sta; e.aux = e.aux + 1
	e.sta = e.aux
	push_cho(e, 0, fcode(e))
	e.stc = e.cho
end

ophandlers[0x1e] = function(e, op)
	e.aux = e.sta
	e.aux = e.aux - 1; e.sta = e.auxdata[e.aux]
	e.aux = e.aux - 1; e.stc = e.auxdata[e.aux]
end

ophandlers[0x1f] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local v
	if a1 >= 0x2000 and a1 < 0x3e00 then
		v = prepend_chars(e, a1, 0x3f00)
	elseif a1 >= 0x3e00 and a1 < 0x3f00 then
		v = create_pair(e, a1, 0x3f00)
	elseif a1 >= 0x4000 and a1 < 0x8000 then
		local i = band(a1, 0x3fff)
		v = 0x3f00
		repeat
			v = create_pair(e, bor(0x4000, i % 10), v)
			i = math.floor(i / 10)
		until i == 0
	elseif a1 >= 0xe000 then
		local a2 = e.heapdata[band(a1, 0x1fff) + 0]
		if a2 >= 0x8000 then
			v = a2
		else
			local a3 = e.heapdata[band(a1, 0x1fff) + 1]
			v = prepend_chars(e, a2, a3)
		end
	else
		fail(e)
		return
	end
	store(e, nextbyte(e), v)
end

ophandlers[0x9f] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if band(a1, 0xe000) ~= 0xc000 then
		fail(e)
		return
	end
	local a2 = deref(e, e.heapdata[band(a1, 0x1fff) + 0])
	if band(a2, 0xff00) == 0x3e00 then
		local a3 = deref(e, e.heapdata[band(a1, 0x1fff) + 1])
		if a3 == 0x3f00 then
			store(e, nextbyte(e), a2)
			return
		end
	end
	local buf = words_to_charlist(e, a1)
	if buf ~= 0 then
		store(e, nextbyte(e), parse_word(e, buf))
	else
		fail(e)
	end
end

ophandlers[0x20] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	store(e, nextbyte(e), readfield(e, a2, a1))
end
ophandlers[0xa0] = ophandlers[0x20]

ophandlers[0x21] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local v = readfield(e, rshift(a2, 1), a1)
	store(e, nextbyte(e), (band(a2, 1) ~= 0) and band(v, 0xff) or rshift(v, 8))
end
ophandlers[0xa1] = ophandlers[0x21]

ophandlers[0x22] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local v = get_lts(e, readfield(e, a2, a1))
	if v ~= 0 then
		store(e, nextbyte(e), v)
	else
		fail(e)
	end
end
ophandlers[0xa2] = ophandlers[0x22]

ophandlers[0x24] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	e.ramdata[fieldaddr(e, a2, a1)] = fvalue(e)
end
ophandlers[0xa4] = ophandlers[0x24]

ophandlers[0x25] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local a3 = fvalue(e)
	local addr = fieldaddr(e, rshift(a2, 1), a1)
	if band(a2, 1) ~= 0 then
		e.ramdata[addr] = bor(band(e.ramdata[addr], 0xff00), band(a3, 0xff))
	else
		e.ramdata[addr] = bor(band(e.ramdata[addr], 0x00ff), lshift(band(a3, 0xff), 8))
	end
end
ophandlers[0xa5] = ophandlers[0x25]

ophandlers[0x26] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or deref(e, fvalue(e))
	local a2 = findex(e)
	local a3 = fvalue(e)
	if a1 <= e.nob or a3 ~= 0 then
		put_lts(e, fieldaddr(e, a2, a1), a3)
	end
end
ophandlers[0xa6] = ophandlers[0x26]

ophandlers[0x28] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local addr = fieldaddr(e, rshift(a2, 4), a1)
	e.ramdata[addr] = bor(e.ramdata[addr], rshift(0x8000, band(a2, 15)))
end
ophandlers[0xa8] = ophandlers[0x28]

ophandlers[0x29] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or deref(e, fvalue(e))
	local a2 = findex(e)
	if a1 <= e.nob then
		local addr = fieldaddr(e, rshift(a2, 4), a1)
		e.ramdata[addr] = band(e.ramdata[addr], bnot(rshift(0x8000, band(a2, 15))))
	end
end
ophandlers[0xa9] = ophandlers[0x29]

ophandlers[0x2d] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local a3 = findex(e)
	unlink(e, fieldaddr(e, a2, a1), a3, deref(e, fvalue(e)))
end
ophandlers[0xad] = ophandlers[0x2d]

ophandlers[0x2e] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or deref(e, fvalue(e))
	local a2 = (band(op, 0x01) ~= 0) and nextbyte(e) or deref(e, fvalue(e))
	if a1 < e.nob or a2 ~= 0 then
		if a1 >= 0x2000 or a2 >= 0x2000 then error(EXPECTOBJ, 0) end
		local v = e.ramdata[fieldaddr(e, 0, a1)]
		if v ~= 0 then
			unlink(e, fieldaddr(e, 1, v), 2, a1)
		end
		e.ramdata[fieldaddr(e, 0, a1)] = a2
		if a2 ~= 0 then
			e.ramdata[fieldaddr(e, 2, a1)] = e.ramdata[fieldaddr(e, 1, a2)]
			e.ramdata[fieldaddr(e, 1, a2)] = a1
		end
	end
end
ophandlers[0x2f] = ophandlers[0x2e]
ophandlers[0xae] = ophandlers[0x2e]
ophandlers[0xaf] = ophandlers[0x2e]

ophandlers[0x30] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fword(e)
	local a2 = fvalue(e)
	local a3 = fcode(e)
	if a1 == a2 then e.inst = a3 end
end
ophandlers[0xb0] = ophandlers[0x30]

ophandlers[0x31] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if band(a1, 0xe000) ~= 0x8000 then e.inst = a2 end
end

ophandlers[0x32] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 == 0x3f00 then e.inst = a2 end
end

ophandlers[0x33] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 >= 0x4000 and a1 < 0x8000 then e.inst = a2 end
end

ophandlers[0x34] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if band(a1, 0xe000) == 0xc000 then e.inst = a2 end
end

ophandlers[0x35] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 < 0x2000 then e.inst = a2 end
end

ophandlers[0x36] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 >= 0xe000 or (a1 >= 0x2000 and a1 < 0x3f00) then e.inst = a2 end
end

ophandlers[0xb6] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 >= 0xe000 and band(e.heapdata[band(a1, 0x1fff)], 0xe000) == 0xc000 then e.inst = a2 end
end

ophandlers[0x37] = function(e, op)
	local a1 = fvalue(e)
	local a2 = fvalue(e)
	local a3 = fcode(e)
	if would_unify(e, a1, a2) then e.inst = a3 end
end

ophandlers[0x38] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	local a3 = fcode(e)
	if a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 and a1 > a2 then e.inst = a3 end
end

ophandlers[0x39] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a2 = fvalue(e)
	local a3 = fcode(e)
	if a1 == deref(e, a2) then e.inst = a3 end
end
ophandlers[0xb9] = ophandlers[0x39]

ophandlers[0x3a] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local a3 = (band(op, 1) ~= 0) and nextbyte(e) or fvalue(e)
	local a4 = fcode(e)
	if readfield(e, a2, a1) == a3 then e.inst = a4 end
end
ophandlers[0xba] = ophandlers[0x3a]
ophandlers[0x3d] = ophandlers[0x3a]
ophandlers[0xbd] = ophandlers[0x3a]

ophandlers[0x3b] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local a3 = fcode(e)
	if band(readfield(e, rshift(a2, 4), a1), rshift(0x8000, band(a2, 15))) ~= 0 then e.inst = a3 end
end
ophandlers[0xbb] = ophandlers[0x3b]

ophandlers[0x3c] = function(e, op)
	local a1 = fcode(e)
	if e.cwl ~= 0 then e.inst = a1 end
end

ophandlers[0x40] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fword(e)
	local a2 = fvalue(e)
	local a3 = fcode(e)
	if a1 ~= a2 then e.inst = a3 end
end
ophandlers[0xc0] = ophandlers[0x40]

ophandlers[0x41] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if band(a1, 0xe000) == 0x8000 then e.inst = a2 end
end

ophandlers[0x42] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 ~= 0x3f00 then e.inst = a2 end
end

ophandlers[0x43] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 < 0x4000 or a1 >= 0x8000 then e.inst = a2 end
end

ophandlers[0x44] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if band(a1, 0xe000) ~= 0xc000 then e.inst = a2 end
end

ophandlers[0x45] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 >= 0x2000 then e.inst = a2 end
end

ophandlers[0x46] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 < 0xe000 and (a1 < 0x2000 or a1 >= 0x3f00) then e.inst = a2 end
end

ophandlers[0xc6] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = fcode(e)
	if a1 < 0xe000 or band(e.heapdata[band(a1, 0x1fff)], 0xe000) ~= 0xc000 then e.inst = a2 end
end

ophandlers[0x47] = function(e, op)
	local a1 = fvalue(e)
	local a2 = fvalue(e)
	local a3 = fcode(e)
	if not would_unify(e, a1, a2) then e.inst = a3 end
end

ophandlers[0x48] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	local a3 = fcode(e)
	if not (a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 and a1 > a2) then e.inst = a3 end
end

ophandlers[0x49] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a2 = fvalue(e)
	local a3 = fcode(e)
	if a1 ~= deref(e, a2) then e.inst = a3 end
end
ophandlers[0xc9] = ophandlers[0x49]

ophandlers[0x4a] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local a3 = (band(op, 1) ~= 0) and nextbyte(e) or fvalue(e)
	local a4 = fcode(e)
	if readfield(e, a2, a1) ~= a3 then e.inst = a4 end
end
ophandlers[0xca] = ophandlers[0x4a]
ophandlers[0x4d] = ophandlers[0x4a]
ophandlers[0xcd] = ophandlers[0x4a]

ophandlers[0x4b] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and 0 or fvalue(e)
	local a2 = findex(e)
	local a3 = fcode(e)
	if band(readfield(e, rshift(a2, 4), a1), rshift(0x8000, band(a2, 15))) == 0 then e.inst = a3 end
end
ophandlers[0xcb] = ophandlers[0x4b]

ophandlers[0x4c] = function(e, op)
	local a1 = fcode(e)
	if e.cwl == 0 then e.inst = a1 end
end

ophandlers[0x50] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	store(e, nextbyte(e), band(a1 + a2, 0xffff))
end

ophandlers[0xd0] = function(e, op)
	local a1 = deref(e, fvalue(e))
	store(e, nextbyte(e), band(a1 + 1, 0xffff))
end

ophandlers[0x51] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	store(e, nextbyte(e), band(a1 - a2, 0xffff))
end

ophandlers[0xd1] = function(e, op)
	local a1 = deref(e, fvalue(e))
	store(e, nextbyte(e), band(a1 - 1, 0xffff))
end

ophandlers[0x52] = function(e, op)
	local a1 = nextbyte(e)
	store(e, nextbyte(e), compat_rand(e) % (a1 + 1))
end

ophandlers[0x58] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 then
		local v = band(a1, 0x3fff) + band(a2, 0x3fff)
		if v < 0x4000 then
			store(e, nextbyte(e), bor(v, 0x4000))
		else fail(e) end
	else fail(e) end
end

ophandlers[0xd8] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x7fff then
		store(e, nextbyte(e), a1 + 1)
	else fail(e) end
end

ophandlers[0x59] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 then
		local v = band(a1, 0x3fff) - band(a2, 0x3fff)
		if v >= 0 then
			store(e, nextbyte(e), bor(v, 0x4000))
		else fail(e) end
	else fail(e) end
end

ophandlers[0xd9] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if a1 > 0x4000 and a1 < 0x8000 then
		store(e, nextbyte(e), a1 - 1)
	else fail(e) end
end

ophandlers[0x5a] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 and a2 >= a1 then
		local v = a1 + (compat_rand(e) % (a2 - a1 + 1))
		store(e, nextbyte(e), v)
	else fail(e) end
end

ophandlers[0x5b] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 then
		local v = band(band(a1, 0x3fff) * band(a2, 0x3fff), 0x3fff)
		store(e, nextbyte(e), bor(v, 0x4000))
	else fail(e) end
end

ophandlers[0x5c] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x8000 and a2 > 0x4000 and a2 < 0x8000 then
		local v = math.floor(band(a1, 0x3fff) / band(a2, 0x3fff))
		store(e, nextbyte(e), bor(v, 0x4000))
	else fail(e) end
end

ophandlers[0x5d] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if a1 >= 0x4000 and a1 < 0x8000 and a2 > 0x4000 and a2 < 0x8000 then
		local v = band(a1, 0x3fff) % band(a2, 0x3fff)
		store(e, nextbyte(e), bor(v, 0x4000))
	else fail(e) end
end

ophandlers[0x60] = function(e, op)
	if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space()
	elseif e.spc == e.SP_NBSP then e.io.nbsp() end
	e.io.print(decodestr(e, fstring(e)))
	e.spc = e.SP_AUTO
end

ophandlers[0xe0] = function(e, op)
	if e.spc == e.SP_PENDING then e.io.space()
	elseif e.spc == e.SP_NBSP then e.io.nbsp() end
	e.io.print(decodestr(e, fstring(e)))
	e.spc = e.SP_AUTO
end

ophandlers[0x61] = function(e, op)
	if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space()
	elseif e.spc == e.SP_NBSP then e.io.nbsp() end
	e.io.print(decodestr(e, fstring(e)))
	e.spc = e.SP_NOSPACE
end

ophandlers[0xe1] = function(e, op)
	if e.spc == e.SP_PENDING then e.io.space()
	elseif e.spc == e.SP_NBSP then e.io.nbsp() end
	e.io.print(decodestr(e, fstring(e)))
	e.spc = e.SP_NOSPACE
end

ophandlers[0x62] = function(e, op)
	if e.cwl == 0 then
		if e.spc < e.SP_NOSPACE then e.spc = e.SP_NOSPACE end
	end
end

ophandlers[0xe2] = function(e, op)
	if e.cwl == 0 then
		if e.spc < e.SP_PENDING then e.spc = e.SP_PENDING end
	end
end

ophandlers[0x63] = function(e, op)
	if e.cwl == 0 then
		if e.spc < e.SP_LINE then
			e.io.line()
			e.spc = e.SP_LINE
		end
	end
end

ophandlers[0xe3] = function(e, op)
	if e.cwl == 0 then
		if e.spc < e.SP_PAR then
			if e.n_span ~= 0 then
				e.io.line()
				e.io.line()
			else
				e.io.par()
			end
			e.spc = e.SP_PAR
		end
	end
end

ophandlers[0x64] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if e.cwl == 0 and a1 > 0x4000 and a1 < 0x8000 then
		e.io.space_n(band(a1, 0x3fff))
		e.spc = e.SP_SPACE
	end
end

ophandlers[0x65] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if e.cwl ~= 0 then
		push_aux(e, a1)
	else
		if band(a1, 0xff00) == 0x3e00 then
			local tmp = band(a1, 0xff)
			if e.spc == e.SP_PENDING or (e.spc == e.SP_AUTO and not bytes_includes(e.nospcbefore, tmp)) then
				e.io.space()
			end
			e.io.print(decodechar(e, tmp))
			if bytes_includes(e.nospcafter, tmp) then
				e.spc = e.SP_NOSPACE
			else
				e.spc = e.SP_AUTO
			end
		else
			if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
			e.io.print(val2str(e, a1))
			e.spc = e.SP_AUTO
		end
	end
end

ophandlers[0x66] = function(e, op)
	local a1 = findex(e)
	if e.cwl == 0 then
		if e.n_span ~= 0 then error(IOSTATE, 0) end
		e.io.enter_div(a1)
		arr_push(e.divs, a1)
		e.spc = e.SP_PAR
	end
end

ophandlers[0xe6] = function(e, op)
	if e.cwl == 0 then
		e.io.leave_div(arr_pop(e.divs))
		e.spc = e.SP_LINE
	end
end

ophandlers[0x67] = function(e, op)
	if e.head[0] < 1 then
		local a1 = findex(e)
		if e.cwl == 0 then
			if e.in_status or e.n_span ~= 0 then error(IOSTATE, 0) end
			e.io.enter_status(0, a1)
			e.in_status = 1
			e.spc = e.SP_PAR
		end
	else
		local a1 = findex(e)
		if e.in_status or e.n_span ~= 0 then error(IOSTATE, 0) end
		e.io.set_body(a1)
	end
end

ophandlers[0xe7] = function(e, op)
	if e.head[0] < 1 then
		if e.cwl == 0 then
			e.io.leave_status()
			e.in_status = false
			e.spc = e.SP_PAR
		end
	else
		io.stderr:write("Opcode $E7 is not defined in version 1.x\n")
	end
end

ophandlers[0x68] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if e.cwl == 0 then
		if e.n_link == 0 then
			if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
			e.io.enter_link_res(get_res(e, band(a1, 0x1fff)))
			e.spc = e.SP_NOSPACE
		end
		e.n_link = e.n_link + 1
		e.n_span = e.n_span + 1
	end
end

ophandlers[0xe8] = function(e, op)
	if e.cwl == 0 then
		e.n_link = e.n_link - 1
		e.n_span = e.n_span - 1
		if e.n_link == 0 then e.io.leave_link_res() end
	end
end

ophandlers[0x69] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if e.cwl == 0 then
		if e.n_link == 0 then
			if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
			local saved_upper = e.upper
			e.upper = false
			local str = ""
			while band(a1, 0xe000) == 0xc000 do
				local v = deref(e, a1 - 0x4000)
				if (v >= 0x2000 and v < 0x8000) or v >= 0xe000 then
					if str ~= "" then str = str .. " " end
					str = str .. val2str(e, v)
				end
				a1 = deref(e, a1 - 0x3fff)
			end
			e.io.enter_link(str)
			e.upper = saved_upper
			e.spc = e.SP_NOSPACE
		end
		e.n_link = e.n_link + 1
		e.n_span = e.n_span + 1
	end
end

ophandlers[0xe9] = function(e, op)
	if e.cwl == 0 then
		e.n_link = e.n_link - 1
		e.n_span = e.n_span - 1
		if e.n_link == 0 then e.io.leave_link() end
	end
end

ophandlers[0x6a] = function(e, op)
	if e.cwl == 0 then
		if e.n_link == 0 then
			if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
			e.io.enter_self_link()
			e.spc = e.SP_NOSPACE
		end
		e.n_link = e.n_link + 1
		e.n_span = e.n_span + 1
	end
end

ophandlers[0xea] = function(e, op)
	if e.cwl == 0 then
		e.n_link = e.n_link - 1
		e.n_span = e.n_span - 1
		if e.n_link == 0 then e.io.leave_self_link() end
	end
end

ophandlers[0x6b] = function(e, op)
	local a1 = nextbyte(e)
	if e.cwl == 0 then
		if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
		e.io.setstyle(a1)
		e.spc = e.SP_SPACE
	end
end

ophandlers[0xeb] = function(e, op)
	local a1 = nextbyte(e)
	if e.cwl == 0 then
		e.io.resetstyle(a1)
	end
end

ophandlers[0x6c] = function(e, op)
	local a1 = deref(e, fvalue(e))
	if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
	e.io.embed_res(get_res(e, band(a1, 0x1fff)))
	e.spc = e.SP_AUTO
end

ophandlers[0xec] = function(e, op)
	local a1 = deref(e, fvalue(e))
	store(e, nextbyte(e), e.io.can_embed_res(get_res(e, band(a1, 0x1fff))) and 1 or 0)
end

ophandlers[0x6d] = function(e, op)
	local a1 = deref(e, fvalue(e))
	local a2 = deref(e, fvalue(e))
	if e.cwl == 0 then
		if a1 >= 0x4000 and a1 < 0x8000 and a2 >= 0x4000 and a2 < 0x8000 then
			e.io.progressbar(band(a1, 0x3fff), band(a2, 0x3fff))
		end
	end
end

ophandlers[0x6e] = function(e, op)
	local a1 = findex(e)
	if e.cwl == 0 then
		if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
		e.io.enter_span(a1)
		e.n_span = e.n_span + 1
		e.spc = e.SP_NOSPACE
	end
end

ophandlers[0xee] = function(e, op)
	if e.cwl == 0 then
		e.io.leave_span()
		e.n_span = e.n_span - 1
		e.spc = e.SP_AUTO
	end
end

ophandlers[0x6f] = function(e, op)
	local a1 = nextbyte(e)
	local a2 = findex(e)
	if e.cwl == 0 then
		if e.in_status or e.n_span ~= 0 then error(IOSTATE, 0) end
		e.io.enter_status(a1, a2)
		e.in_status = a1 + 1
		e.spc = e.SP_PAR
	end
end

ophandlers[0xef] = function(e, op)
	if e.head[0] > 0 then
		if e.cwl == 0 then
			e.io.leave_status()
			e.in_status = false
			e.spc = e.SP_PAR
		end
	else
		io.stderr:write("Opcode $EF is not defined in version 0.x\n")
	end
end

ophandlers[0x70] = function(e, op)
	local a1 = nextbyte(e)
	if a1 == 0x00 then -- quit
		e.io.flush()
		return status.quit
	elseif a1 == 0x01 then -- restart
		vm_clear_divs(e)
		vm_reset(e, 0, true)
		vm_restore_state(e, e.initstate)
		e.io.reset()
	elseif a1 == 0x02 then -- restore
		e.io.flush()
		e.io.restore()
		return status.restore
	elseif a1 == 0x03 then -- undo
		if #e.undodata > 0 then
			vm_clear_divs(e)
			vm_restore_state(e, vm_rldec_state(e.initstate, table.remove(e.undodata)))
		elseif not e.pruned_undo then
			fail(e)
		end
	elseif a1 == 0x04 then -- unstyle
		if e.cwl == 0 then e.io.unstyle() end
	elseif a1 == 0x05 then -- print_serial
		if e.cwl == 0 then
			if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space() end
			for i = 0, 5 do
				e.io.print(string.char(e.head[6 + i]))
			end
			e.spc = e.SP_AUTO
		end
	elseif a1 == 0x06 or a1 == 0x07 then -- clear / clear_all
		if e.in_status or e.n_span ~= 0 then error(IOSTATE, 0) end
		local tmp = e.divs
		vm_clear_divs(e)
		if a1 == 0x06 then
			e.io.clear()
		else
			e.io.clear_all()
		end
		for i = 0, tmp.length - 1 do
			e.io.enter_div(tmp[i])
		end
		e.divs = tmp
	elseif a1 == 0x08 then -- script_on
		if not e.io.script_on() then fail(e) end
	elseif a1 == 0x09 then -- script_off
		e.io.script_off()
	elseif a1 == 0x0a then -- trace_on
		e.trace = true
	elseif a1 == 0x0b then -- trace_off
		e.trace = false
	elseif a1 == 0x0c then -- inc_cwl
		e.cwl = e.cwl + 1
	elseif a1 == 0x0d then -- dec_cwl
		e.cwl = e.cwl - 1
	elseif a1 == 0x0e then -- uppercase
		if e.cwl == 0 then e.upper = true end
	elseif a1 == 0x0f then -- clear_links
		e.io.clear_links()
	elseif a1 == 0x10 then -- clear_old
		if e.n_span ~= 0 then error(IOSTATE, 0) end
		e.io.clear_old()
	elseif a1 == 0x11 then -- clear_div
		e.io.clear_div()
	elseif a1 == 0x12 then -- clear_status
		if e.in_status then error(IOSTATE, 0) end
		e.io.clear_status()
	elseif a1 == 0x13 then -- nbsp
		if e.cwl == 0 then
			if e.spc < e.SP_NBSP then e.spc = e.SP_NBSP end
		end
	else
		error("Unimplemented ext0 " .. string.format("%x", a1) .. " at " .. string.format("%x", e.inst - 2), 0)
	end
end

ophandlers[0x72] = function(e, op)
	local a1 = fcode(e)
	if e.in_status or e.n_span ~= 0 then error(IOSTATE, 0) end
	if not e.io.save(vm_wrap_savefile(e, vm_rlenc_state(e.initstate, vm_capture_state(e, a1)))) then
		fail(e)
	end
end

ophandlers[0xf2] = function(e, op)
	local a1 = fcode(e)
	if e.in_status or e.n_span ~= 0 then error(IOSTATE, 0) end
	if #e.undodata > 50 then
		table.remove(e.undodata, 1)
		e.pruned_undo = true
	end
	e.undodata[#e.undodata + 1] = vm_rlenc_state(e.initstate, vm_capture_state(e, a1))
end

ophandlers[0x73] = function(e, op)
	if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space()
	elseif e.spc == e.SP_NBSP then e.io.nbsp() end
	e.io.flush()
	return status.get_input
end

ophandlers[0xf3] = function(e, op)
	if e.spc == e.SP_AUTO or e.spc == e.SP_PENDING then e.io.space()
	elseif e.spc == e.SP_NBSP then e.io.nbsp() end
	e.io.flush()
	return status.get_key
end

ophandlers[0x74] = function(e, op)
	local a1 = nextbyte(e)
	local v = 0
	if a1 == 0x00 then
		v = 0x4000
		for i = 0, e.heapdata.length - 1 do
			if e.heapdata[i] ~= 0x3f3f then v = v + 1 end
		end
	elseif a1 == 0x01 then
		v = 0x4000
		for i = 0, e.auxdata.length - 1 do
			if e.auxdata[i] ~= 0x3f3f then v = v + 1 end
		end
	elseif a1 == 0x02 then
		v = 0x4000
		for i = e.ltb, e.ramdata.length - 1 do
			if e.ramdata[i] ~= 0x3f3f then v = v + 1 end
		end
	elseif a1 == 0x20 then
		v = bor(0x4000, e.io.measure_dims(0))
	elseif a1 == 0x21 then
		v = bor(0x4000, e.io.measure_dims(1))
	elseif a1 == 0x40 then
		v = 1
	elseif a1 == 0x41 then
		v = 1
	elseif a1 == 0x42 then
		v = e.io.have_links() and 1 or 0
	elseif a1 == 0x43 then
		v = e.havequit and 1 or 0
	elseif a1 == 0x44 then
		v = e.io.have_styles() and 1 or 0
	elseif a1 == 0x45 then
		v = e.io.have_color() and 1 or 0
	elseif a1 == 0x46 then
		v = e.io.have_align() and 1 or 0
	elseif a1 == 0x50 then
		v = e.io.script_active() and 1 or 0
	elseif a1 == 0x60 then
		v = e.havetop and 1 or 0
	elseif a1 == 0x61 then
		v = e.haveinline and 1 or 0
	else
		if a1 > 0x7f then
			error("Unimplemented vminfo " .. string.format("%x", a1) .. " at " .. string.format("%x", e.inst - 2), 0)
		end
		if a1 < 0x40 then
			v = 0x4000
		end
		io.stderr:write("Unrecognized vminfo " .. string.format("%x", a1) .. "; returning default " .. string.format("%x", v) .. "\n")
	end
	store(e, nextbyte(e), v)
end

ophandlers[0x78] = function(e, op)
	local v = deref(e, fvalue(e))
	if v >= 0xe000 then v = e.heapdata[band(v, 0x1fff)] end
	e.reg[0x3f] = v
end

ophandlers[0x79] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a2 = fcode(e)
	if e.reg[0x3f] == a1 then e.inst = a2 end
end
ophandlers[0xf9] = ophandlers[0x79]

ophandlers[0x7a] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a2 = fcode(e)
	local a3 = fcode(e)
	if e.reg[0x3f] > a1 then
		e.inst = a2
	elseif e.reg[0x3f] == a1 then
		e.inst = a3
	end
end
ophandlers[0xfa] = ophandlers[0x7a]

ophandlers[0x7b] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fvalue(e)
	local a2 = fcode(e)
	if e.reg[0x3f] > a1 then e.inst = a2 end
end
ophandlers[0xfb] = ophandlers[0x7b]

ophandlers[0x7c] = function(e, op)
	local a1 = findex(e)
	local a2 = fcode(e)
	if wordmap(e, a1, e.reg[0x3f]) then e.inst = a2 end
end

ophandlers[0x7d] = function(e, op)
	local a1 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a2 = (band(op, 0x80) ~= 0) and nextbyte(e) or fword(e)
	local a3 = fcode(e)
	if e.reg[0x3f] == a1 or e.reg[0x3f] == a2 then e.inst = a3 end
end
ophandlers[0xfd] = ophandlers[0x7d]

ophandlers[0x7f] = function(e, op)
	local a1 = fstring(e)
	local a2 = fstring(e)
	local a3 = fstring(e)
	local a4 = fword(e)
	if e.trace then
		local str = decodestr(e, a1) .. "("
		local arg = decodestr(e, a2)
		local j = 0
		for i = 1, #arg do
			local ch = arg:sub(i, i)
			if ch == "$" then
				str = str .. val2str(e, e.reg[j]); j = j + 1
			else
				str = str .. ch
			end
		end
		str = str .. ") " .. decodestr(e, a3) .. ":" .. a4
		e.io.trace(str)
	end
end

-- VM run loop
local function dispatch(e)
	while true do
		local op = e.code[e.inst]; e.inst = e.inst + 1
		local h = ophandlers[op]
		if not h then
			error("Unimplemented op " .. string.format("%x", op) .. " at " .. string.format("%x", e.inst - 1), 0)
		end
		local r = h(e, op)
		if r ~= nil then return r end
	end
end

local function vm_run(e, param)
	if param and param ~= 0 then
		store(e, nextbyte(e), param)
	end
	while true do
		local ok, res = pcall(dispatch, e)
		if ok then
			return res
		elseif type(res) == "number" and res > 0x4000 and res < 0x8000 then
			if e.spc < e.SP_LINE then e.io.line() end
			vm_clear_divs(e)
			vm_reset(e, res, false)
		else
			error(res, 0)
		end
	end
end

-- Story preparation and input
local function prepare_story(file_array, io_obj, seed, quit, toparea, inlinearea)
	local function findfiles()
		local size = get32(file_array, 4) + 8
		local pos = 12
		local list = {}
		while pos < size do
			local chname = getfour(file_array, pos)
			local chsize = get32(file_array, pos + 4)
			if chname == "FILE" then
				local fname = {}
				local i = 0
				while file_array[pos + 8 + i] ~= 0 do
					fname[#fname + 1] = string.char(file_array[pos + 8 + i])
					i = i + 1
				end
				list[table.concat(fname)] = slice_bytes(file_array, pos + 8 + i + 1, pos + 8 + chsize)
			end
			pos = pos + 8 + band(chsize + 1, bnot(1))
		end
		return list
	end

	local function findch(name, mandatory)
		local data = findchunk(file_array, name)
		if not data and mandatory then
			error("Missing " .. name .. " chunk.", 0)
		end
		return data
	end

	if getfour(file_array, 0) ~= "FORM" or getfour(file_array, 8) ~= "AAVM" then
		error("Not an aastory file", 0)
	end

	local e = {
		VER_MAJOR = 1,
		VER_MINOR = 0,

		SP_AUTO = 0,
		SP_NOSPACE = 1,
		SP_NBSP = 2,
		SP_PENDING = 3,
		SP_SPACE = 4,
		SP_LINE = 5,
		SP_PAR = 6,

		head = findch("HEAD", true),
		code = findch("CODE", true),
		dict = findch("DICT", true),
		init = findch("INIT", true),
		lang = findch("LANG", true),
		maps = findch("MAPS", true),
		tags = findch("TAGS", false),
		writ = findch("WRIT", true),
		look = findch("LOOK", true),
		meta = findch("META", false),
		urls = findch("URLS", false),
		files = findfiles(),

		randomseed = seed,
		strshift = 0,
		extchars = 0,
		esc_bits = 7,
		esc_boundary = 0,
		io = io_obj,
		stopchars = {length = 0},
		nospcbefore = {length = 0},
		nospcafter = {length = 0},

		reg = newbytes(64),
		inst = 0, cont = 0, top = 0, env = 0, cho = 0, sim = 0xffff, aux = 0,
		trl = 0, sta = 0, stc = 0, cwl = 0, spc = 0, nob = 0, ltb = 0, ltt = 0,

		upper = false,
		trace = false,
		divs = {length = 0},
		in_status = false,
		n_statusdiv = 0,
		n_span = 0,
		n_link = 0,

		undodata = {},
		pruned_undo = false,
		havequit = quit,
		havetop = toparea,
		haveinline = inlinearea,

		tmp = 0
	}

	if (e.head[0] > e.VER_MAJOR) or (e.head[0] == e.VER_MAJOR and e.head[1] > e.VER_MINOR) then
		error("Unsupported aastory file format version " .. e.head[0] .. "." .. e.head[1] .. "; this interpreter supports up to " .. e.VER_MAJOR .. "." .. e.VER_MINOR, 0)
	end
	if e.head[2] ~= 2 then
		error("Unsupported word size " .. e.head[2] .. "; this interpreter only supports 2", 0)
	end

	e.heapdata = newbytes(get16(e.head, 16))
	e.auxdata = newbytes(get16(e.head, 18))
	e.ramdata = newbytes(get16(e.head, 20))
	e.strshift = e.head[3]
	e.extchars = get16(e.lang, 2)

	if e.head[0] > 0 or e.head[1] >= 4 then
		e.esc_boundary = e.lang[e.extchars] - 32
		if e.esc_boundary < 0 then e.esc_boundary = 0 end
		local i = e.esc_boundary + get16(e.dict, 0) - 1
		e.esc_bits = 0
		while i > 0 do
			i = rshift(i, 1)
			e.esc_bits = e.esc_bits + 1
		end
	end

	local stopptr = get16(e.lang, 6)
	local stopend = stopptr
	while e.lang[stopend] ~= 0 do stopend = stopend + 1 end
	e.stopchars = slice_bytes(e.lang, stopptr, stopend)
	if e.head[0] > 0 or e.head[1] >= 4 then
		stopptr = stopend + 1
		stopend = stopptr
		while e.lang[stopend] ~= 0 do stopend = stopend + 1 end
		e.nospcbefore = slice_bytes(e.lang, stopptr, stopend)
		stopptr = stopend + 1
		stopend = stopptr
		while e.lang[stopend] ~= 0 do stopend = stopend + 1 end
		e.nospcafter = slice_bytes(e.lang, stopptr, stopend)
	end

	vm_reinit(e)
	vm_reset(e, 0, true)
	e.initstate = vm_capture_state(e, 1)
	io_obj.reset()

	return e
end

local function vm_proceed_with_input(e, str)
	local cps = utf8_codepoints(str)
	local n = cps.length
	local chars = {length = n}
	for i = 0, n - 1 do
		local uchar = cps[i]
		if uchar >= 0x41 and uchar <= 0x5a then
			chars[i] = bxor(uchar, 0x20)
		elseif uchar < 0x80 then
			chars[i] = uchar
		else
			chars[i] = 0x3f
			for j = e.lang[e.extchars] - 1, 0, -1 do
				local entry = e.extchars + 1 + j * 5
				if e.lang[entry + 2] == band(rshift(uchar, 16), 0xff)
					and e.lang[entry + 3] == band(rshift(uchar, 8), 0xff)
					and e.lang[entry + 4] == band(uchar, 0xff) then
					chars[i] = e.lang[entry]
					break
				end
			end
		end
	end

	local words = {length = 0}
	local start = 0
	local i = 0
	while i < n do
		if chars[i] == 32 then
			if i ~= start then arr_push(words, slice_bytes(chars, start, i)) end
			start = i + 1
		else
			if bytes_includes(e.stopchars, chars[i]) then
				if i ~= start then arr_push(words, slice_bytes(chars, start, i)) end
				arr_push(words, slice_bytes(chars, i, i + 1))
				start = i + 1
			end
		end
		i = i + 1
	end
	if i ~= start then arr_push(words, slice_bytes(chars, start, i)) end

	local v = 0x3f00
	local ok, err = pcall(function()
		for k = words.length - 1, 0, -1 do
			v = create_pair(e, parse_word(e, words[k]), v)
		end
	end)
	if not ok then
		if err == HEAPFULL then
			if e.spc < e.SP_LINE then e.io.line() end
			vm_clear_divs(e)
			vm_reset(e, err, false)
			v = nil
		else
			error(err, 0)
		end
	end

	e.spc = e.SP_LINE
	return vm_run(e, v)
end

local function vm_proceed_with_key(e, code)
	local v = 0
	if code >= 0x20 and code < 0x7f then
		if code >= 0x41 and code <= 0x5a then code = bxor(code, 0x20) end
		v = code
	end
	if v == 0 then
		for _, kc in pairs(keys) do
			if code == kc then v = code; break end
		end
	end
	if v == 0 then
		local i = 0
		while i < e.lang[e.extchars] do
			local entry = 1 + i * 5
			local cond = bor((code == lshift(e.lang[entry + 2], 16)) and 1 or 0, lshift(e.lang[entry + 3], 8), e.lang[entry + 4])
			if cond ~= 0 then break end
			i = i + 1
		end
		if i < e.lang[e.extchars] then
			v = bor(0x80, e.lang[1 + i * 5])
		end
	end
	if v == 0 then
		return status.get_key
	else
		e.spc = e.SP_SPACE
		if v >= 0x30 and v <= 0x39 then
			v = v + 0x4000 - 0x30
		else
			v = bor(v, 0x3e00)
		end
		return vm_run(e, v)
	end
end

-- Public API
local instance_e

local aaengine = {}

function aaengine.prepare_story(file_array, io_obj, seed, quit, toparea, inlinearea)
	instance_e = prepare_story(file_array, io_obj, seed, quit, toparea, inlinearea)
	aaengine.keys = keys
	aaengine.status = status
	aaengine.opcodes = opcodes
end

function aaengine.get_styles()
	return get_styles(instance_e)
end

function aaengine.get_metadata()
	return get_metadata(instance_e)
end

function aaengine.get_file(name)
	return instance_e.files[name]
end

function aaengine.get_resources()
	return get_resources(instance_e)
end

function aaengine.get_story_key()
	local m = get_metadata(instance_e)
	local str = (m.title:gsub("[^a-zA-Z0-9]+", "-")) .. "-"
	for i = 0, 5 do str = str .. decodechar(instance_e, instance_e.head[6 + i]) end
	str = str .. "-"
	for i = 0, 3 do
		local hex = string.format("%x", instance_e.head[12 + i])
		if #hex == 1 then hex = "0" .. hex end
		str = str .. hex
	end
	return str
end

function aaengine.vm_start()
	return vm_run(instance_e, nil)
end

function aaengine.vm_proceed_with_input(str)
	return vm_proceed_with_input(instance_e, str)
end

function aaengine.vm_proceed_with_key(charcode)
	return vm_proceed_with_key(instance_e, charcode)
end

function aaengine.vm_restore(filedata)
	local v
	if filedata then v = vm_unwrap_savefile(instance_e, filedata) end
	if filedata and v then
		vm_clear_divs(instance_e)
		vm_reset(instance_e, 0, true)
		vm_restore_state(instance_e, vm_rldec_state(instance_e.initstate, v))
	end
	instance_e.spc = instance_e.SP_LINE
	return vm_run(instance_e, nil)
end

function aaengine.async_restart()
	vm_clear_divs(instance_e)
	vm_reset(instance_e, 0, true)
	vm_restore_state(instance_e, instance_e.initstate)
	instance_e.io.reset()
	return vm_run(instance_e, nil)
end

function aaengine.async_save(st)
	local state = vm_capture_state(instance_e, instance_e.inst - ((st == status.quit) and 2 or 1))
	return vm_wrap_savefile(instance_e, vm_rlenc_state(instance_e.initstate, state))
end

function aaengine.async_restore(filedata)
	local v = vm_unwrap_savefile(instance_e, filedata)
	vm_reset(instance_e, 0, true)
	vm_restore_state(instance_e, vm_rldec_state(instance_e.initstate, v))
	instance_e.spc = instance_e.SP_LINE
end

function aaengine.async_resume()
	return vm_run(instance_e, nil)
end

function aaengine.get_undo_array()
	local out = {}
	for i = 1, #instance_e.undodata do
		out[i] = vm_wrap_savefile(instance_e, instance_e.undodata[i])
	end
	return out
end

function aaengine.set_undo_array(arr)
	local out = {}
	for i = 1, #arr do
		out[i] = vm_unwrap_savefile(instance_e, arr[i])
	end
	instance_e.undodata = out
end

function aaengine.mem_info()
	local e = instance_e
	local h, a, lts = 0, 0, 0
	for i = 0, e.heapdata.length - 1 do
		if e.heapdata[i] ~= 0x3f3f then h = h + 1 end
	end
	for i = 0, e.auxdata.length - 1 do
		if e.auxdata[i] ~= 0x3f3f then a = a + 1 end
	end
	for i = e.ltb, e.ramdata.length - 1 do
		if e.ramdata[i] ~= 0x3f3f then lts = lts + 1 end
	end
	return {
		heap = h,
		aux = a,
		lts = lts,
		heapsize = e.heapdata.length,
		auxsize = e.auxdata.length,
		ltssize = e.ramdata.length - e.ltb
	}
end

aaengine.keys = keys
aaengine.status = status
aaengine.opcodes = opcodes

return aaengine

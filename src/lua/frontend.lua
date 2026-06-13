-- Å-machine LuaJIT terminal frontend, ported from src/js/nodefrontend.js.
--
-- Wraps the Lua engine (engine.lua) and handles word-wrapped terminal I/O.
-- Demonstrates embedding the engine in a Lua project and is used to run the
-- automated test suite. Output is byte-for-byte compatible with the Node
-- frontend.

local VERSION = "1.0.3"

local lua_io = io
local stdout = io.stdout
local stdin = io.stdin
local stderr = io.stderr

-- On Windows the standard streams default to text mode, which rewrites "\n"
-- into "\r\n" and corrupts the byte-exact transcript. Switch fd 0 and 1 to
-- binary through the C runtime. macOS, Linux and other Unix platforms already
-- use binary streams, so nothing platform-specific runs there.
if jit and jit.os == "Windows" then
	pcall(function()
		local ffi = require("ffi")
		ffi.cdef[[int _setmode(int fd, int mode);]]
		ffi.C._setmode(0, 0x8000)
		ffi.C._setmode(1, 0x8000)
	end)
end

local function is_terminal()
	local ok, ffi = pcall(require, "ffi")
	if ok then
		ffi.cdef[[int isatty(int fd); int fileno(void *stream);]]
		return ffi.C.isatty(ffi.C.fileno(stdin)) == 1
	end
	return false
end

local script_dir = (arg[0] or ""):match("^(.*[/\\])") or "./"
package.path = script_dir .. "?.lua;" .. package.path
local aaengine = require("engine")

local status
local restore_status
local io_tag_lines = false
local quirks = false

local function w(s)
	stdout:write(s)
end

local function utf8_len(s)
	local _, n = s:gsub("[%z\1-\127\194-\244]", "")
	return n
end

local function bytes_from_string(s)
	local a = {length = #s}
	for i = 1, #s do a[i - 1] = string.byte(s, i) end
	return a
end

local function codepoints(line)
	local cps = {length = 0}
	local i = 1
	local len = #line
	while i <= len do
		local c = string.byte(line, i)
		local cp, adv
		if c < 0x80 then
			cp = c; adv = 1
		elseif c < 0xe0 then
			cp = (c % 0x20) * 0x40 + (string.byte(line, i + 1) or 0) % 0x40; adv = 2
		elseif c < 0xf0 then
			cp = (c % 0x10) * 0x1000 + ((string.byte(line, i + 1) or 0) % 0x40) * 0x40 + (string.byte(line, i + 2) or 0) % 0x40; adv = 3
		else
			cp = (c % 0x08) * 0x40000 + ((string.byte(line, i + 1) or 0) % 0x40) * 0x1000 + ((string.byte(line, i + 2) or 0) % 0x40) * 0x40 + (string.byte(line, i + 3) or 0) % 0x40; adv = 4
		end
		cps[cps.length] = cp
		cps.length = cps.length + 1
		i = i + adv
	end
	return cps
end

local io = {
	hidden = false,
	newlines = 0,
	pending_spaces = 0,
	pword = "",
	pwlen = 0,
	xpos = 0,
	width = 80
}

io.vspace_n = function(n)
	n = math.floor(n) + 1
	while io.newlines < n do
		w("\n")
		if io_tag_lines then w("  ") end
		io.newlines = io.newlines + 1
	end
	io.xpos = 0
	io.pending_spaces = 0
end

io.flush = function()
	if io.width > 0 and io.xpos + io.pending_spaces + io.pwlen > io.width then
		io.vspace_n(0)
	end
	while io.pending_spaces > 0 do
		if io.xpos ~= 0 then
			w(" ")
			io.xpos = io.xpos + 1
		end
		io.pending_spaces = io.pending_spaces - 1
	end
	if io.pwlen > 0 then
		w(io.pword)
		io.xpos = io.xpos + io.pwlen
		io.newlines = 0
		io.pword = ""
		io.pwlen = 0
	end
end

io.reset = function()
	io.hidden = false
	io.pword = ""
	io.pwlen = 0
	io.pending_spaces = 0
	io.xpos = 0
	io.newlines = quirks and 999 or 1
end

io.clear = function() io.par() end
io.clear_all = function() io.par() end
io.clear_links = function() end
io.clear_old = function() end
io.clear_div = function() end
io.clear_status = function() end

io.print = function(str)
	if not io.hidden then
		for ch in str:gmatch("[%z\1-\127\194-\244][\128-\191]*") do
			if ch == " " then
				io.flush()
				io.pending_spaces = io.pending_spaces + 1
			elseif ch == "-" then
				io.pword = io.pword .. ch
				io.pwlen = io.pwlen + 1
				io.flush()
			else
				io.pword = io.pword .. ch
				io.pwlen = io.pwlen + 1
			end
		end
	end
end

io.nbsp = function()
	if not io.hidden then
		io.pword = io.pword .. " "
		io.pwlen = io.pwlen + 1
	end
end

io.space = function()
	if not io.hidden then
		io.print(" ")
	end
end

io.space_n = function(n)
	if not io.hidden then
		io.flush()
		if io.width > 0 and n > io.width - io.xpos then
			n = io.width - io.xpos
		end
		for _ = 1, n do
			w(" ")
			io.xpos = io.xpos + 1
		end
		io.newlines = 0
	end
end

io.line = function()
	if not io.hidden then
		io.flush()
		io.vspace_n(0)
	end
end

io.par = function()
	if not io.hidden then
		io.flush()
		io.vspace_n(1)
	end
end

io.measure_dims = function(which)
	if which == 0 then
		return io.width > 0 and io.width or 0
	else
		return 0
	end
end

io.setstyle = function(s) end
io.resetstyle = function(s) end
io.unstyle = function() end
io.set_body = function(id) end

io.parse_em = function(id, key, defvalue)
	if id >= 0 then
		local map = io.styles[id]
		local str = map and map[key]
		if str then
			local num = str:match("^ *(%d+)em")
			if num then return tonumber(num) end
		end
	end
	return defvalue
end

io.enter_div = function(id)
	if not io.hidden then
		io.flush()
		io.vspace_n(io.parse_em(id, "margin-top", 0))
	end
end

io.leave_div = function(id)
	if not io.hidden then
		io.flush()
		io.vspace_n(io.parse_em(id, "margin-bottom", 0))
	end
end

io.enter_span = function(id) end
io.leave_span = function() end

io.enter_status = function(area, id)
	io.line()
	io.hidden = true
end

io.leave_status = function()
	io.hidden = false
end

io.have_links = function() return false end
io.enter_link = function(str) end
io.leave_link = function() end
io.enter_link_res = function(res) end
io.leave_link_res = function() end
io.enter_self_link = function() end
io.leave_self_link = function() end

io.leave_all = function()
	io.line()
	io.hidden = false
end

io.embed_res = function(res)
	io.print("[")
	io.print(res.alt)
	io.print("]")
end

io.can_embed_res = function(res) return false end

io.progressbar = function(p, total)
	if not io.hidden then
		local full
		if io.width <= 0 then
			full = 80 - 3
		else
			full = io.width - 3
		end
		local first = math.floor(full * (p / total) + 0.5)
		local second = full - first
		io.enter_div(-1)
		io.print("[")
		for _ = 1, first do io.print("=") end
		for _ = 1, second do io.print(" ") end
		io.print("]")
		io.leave_div(-1)
	end
end

io.trace = function(str)
	if not io.hidden then
		io.enter_div(-1)
		w(str)
		io.xpos = utf8_len(str)
		io.newlines = 0
		io.leave_div(-1)
	end
end

io.script_on = function() return false end
io.script_off = function() end
io.script_active = function() return false end

io.save = function(filedata)
	local f = lua_io.open("saved-game.aasave", "wb")
	if not f then return false end
	local parts = {}
	for i = 0, filedata.length - 1 do
		parts[i + 1] = string.char(filedata[i])
	end
	f:write(table.concat(parts))
	f:close()
	return true
end

io.restore = function()
	local data
	local f = lua_io.open("saved-game.aasave", "rb")
	if f then
		local content = f:read("*a")
		f:close()
		if content then data = bytes_from_string(content) end
	end
	restore_status = aaengine.vm_restore(data)
end

io.have_styles = function() return false end
io.have_color = function() return false end
io.have_align = function() return false end

local function add_tag()
	if not io_tag_lines then return end
	w("\n")
	if status == aaengine.status.get_input then
		w("> ")
	elseif status == aaengine.status.get_key then
		w(") ")
	else
		w("  ")
	end
end

local function reconcile_restore()
	if status == aaengine.status.restore and restore_status ~= nil then
		status = restore_status
		restore_status = nil
	end
end

local function usage()
	stdout:write("Usage: aamrun [OPTIONS] file.aastory\n")
	stdout:write("    -s N            Set random seed.\n")
	stdout:write("    -w N            Set screen width (default 80).\n")
	stdout:write("    -h, --help      Show this message.\n")
	stdout:write("    -v, -V          Show version and exit.\n")
	stdout:write("    -T, --tag-lines Prepend output with \"  \", input with \"> \" or \") \".\n")
	stdout:write("    -D, --dfrotz    Emulate dfrotz quirks.\n")
end

local seed, width, filename
local show_version = false
local show_help = false
local positionals = 0

local i = 1
while i <= #arg do
	local a = arg[i]
	if a == "-s" then
		i = i + 1; seed = tonumber(arg[i])
	elseif a == "-w" then
		i = i + 1; width = tonumber(arg[i])
	elseif a == "-T" or a == "--tag-lines" then
		io_tag_lines = true
	elseif a == "-D" or a == "--dfrotz" then
		quirks = true
	elseif a == "-v" or a == "-V" then
		show_version = true
	elseif a == "-h" or a == "--help" then
		show_help = true
	else
		filename = a
		positionals = positionals + 1
	end
	i = i + 1
end

if show_version then
	stdout:write("Å-machine Lua frontend version " .. VERSION .. "\n")
	os.exit(0)
end
if positionals ~= 1 then
	stderr:write("ERROR: Exactly one filename must be specified\n")
	usage()
	os.exit(1)
end
if show_help then
	usage()
	os.exit(0)
end
if seed == 0 then seed = nil end
if width and width ~= 0 then io.width = width end

local storyfile
do
	local f = lua_io.open(filename, "rb")
	if not f then
		stderr:write("ERROR: Unable to read file " .. filename .. "\n")
		os.exit(1)
	end
	local content = f:read("*a")
	f:close()
	storyfile = bytes_from_string(content or "")
end

aaengine.prepare_story(storyfile, io, seed, true, false, false)
io.styles = aaengine.get_styles()

status = aaengine.vm_start()
reconcile_restore()
if status == aaengine.status.quit then
	io.line()
	io.flush()
	os.exit(0)
end
add_tag()

local function process_line(line)
	if status == aaengine.status.get_key then
		local cps = codepoints(line)
		local ci = 0
		while ci < cps.length and status == aaengine.status.get_key do
			if io_tag_lines then w("  ") end
			status = aaengine.vm_proceed_with_key(cps[ci])
			reconcile_restore()
			add_tag()
			ci = ci + 1
		end
		if status == aaengine.status.get_key then
			if io_tag_lines then w("  ") end
			status = aaengine.vm_proceed_with_key(aaengine.keys.KEY_RETURN)
			reconcile_restore()
			add_tag()
		end
	elseif status == aaengine.status.get_input then
		io.xpos = 0
		io.pending_spaces = 0
		io.newlines = quirks and 999 or 1
		if io_tag_lines then w("  ") end
		status = aaengine.vm_proceed_with_input(line)
		reconcile_restore()
		add_tag()
	end

	if status == aaengine.status.quit then
		if quirks then io.par() else io.line() end
		io.flush()
		os.exit(0)
	end
end

if is_terminal() then
	while true do
		local line = stdin:read("*l")
		if line == nil then break end
		line = line:gsub("\r$", "")
		w(line)
		w("\r\n")
		process_line(line)
	end
else
	local all = stdin:read("*a") or ""
	local start = 1
	while true do
		local nl = all:find("\n", start, true)
		if not nl then
			if start <= #all then
				local line = all:sub(start)
				line = line:gsub("\r$", "")
				w(line)
				w("\r\n")
				process_line(line)
			end
			break
		end
		local line = all:sub(start, nl - 1)
		line = line:gsub("\r$", "")
		w(line)
		w("\r\n")
		process_line(line)
		start = nl + 1
	end
end

io.line()
io.flush()

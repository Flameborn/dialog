package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const VERSION = "1.0.3"

type TerminalIO struct {
	hidden      bool
	newlines    int
	pendingSpc  int
	pendingWord string
	xpos        int
	width       int
	styles      []map[string]string
	quirks      bool
}

func NewTerminalIO(width int, quirks bool) *TerminalIO {
	return &TerminalIO{width: width, quirks: quirks}
}

func (io *TerminalIO) Reset() {
	io.hidden = false
	io.pendingWord = ""
	io.pendingSpc = 0
	io.xpos = 0
	if io.quirks {
		io.newlines = 999
	} else {
		io.newlines = 1
	}
}

func (io *TerminalIO) vspaceN(n int) {
	n++
	for io.newlines < n {
		fmt.Print("\n")
		io.newlines++
	}
	io.xpos = 0
	io.pendingSpc = 0
}

func (io *TerminalIO) Flush() {
	if io.width > 0 && io.xpos+io.pendingSpc+utf8.RuneCountInString(io.pendingWord) > io.width {
		io.vspaceN(0)
	}
	for io.pendingSpc > 0 {
		if io.xpos > 0 {
			fmt.Print(" ")
			io.xpos++
		}
		io.pendingSpc--
	}
	if len(io.pendingWord) > 0 {
		fmt.Print(io.pendingWord)
		io.xpos += utf8.RuneCountInString(io.pendingWord)
		io.newlines = 0
		io.pendingWord = ""
	}
}

func (io *TerminalIO) Print(str string) {
	if !io.hidden {
		for _, r := range str {
			if r == ' ' {
				io.Flush()
				io.pendingSpc++
			} else if r == '-' {
				io.pendingWord += string(r)
				io.Flush()
			} else {
				io.pendingWord += string(r)
			}
		}
	}
}

func (io *TerminalIO) Nbsp() {
	if !io.hidden {
		io.pendingWord += " "
	}
}

func (io *TerminalIO) Space() {
	if !io.hidden {
		io.Print(" ")
	}
}

func (io *TerminalIO) SpaceN(n int) {
	if !io.hidden {
		io.Flush()
		if io.width > 0 && n > io.width-io.xpos {
			n = io.width - io.xpos
		}
		for i := 0; i < n; i++ {
			fmt.Print(" ")
			io.xpos++
		}
		io.newlines = 0
	}
}

func (io *TerminalIO) Line() {
	if !io.hidden {
		io.Flush()
		io.vspaceN(0)
	}
}

func (io *TerminalIO) Par() {
	if !io.hidden {
		io.Flush()
		io.vspaceN(1)
	}
}

func (io *TerminalIO) SetBody(id int) {}

func (io *TerminalIO) parseEm(id int, key string, defvalue int) int {
	if id >= 0 && id < len(io.styles) {
		if str, ok := io.styles[id][key]; ok {
			str = strings.TrimSpace(str)
			if strings.HasSuffix(str, "em") {
				str = strings.TrimSuffix(str, "em")
				str = strings.TrimSpace(str)
				if v, err := strconv.Atoi(str); err == nil {
					return v
				}
			}
		}
	}
	return defvalue
}

func (io *TerminalIO) EnterDiv(id int) {
	if !io.hidden {
		io.Flush()
		io.vspaceN(io.parseEm(id, "margin-top", 0))
	}
}

func (io *TerminalIO) LeaveDiv(id int) {
	if !io.hidden {
		io.Flush()
		io.vspaceN(io.parseEm(id, "margin-bottom", 0))
	}
}

func (io *TerminalIO) EnterSpan(id int) {}
func (io *TerminalIO) LeaveSpan()      {}

func (io *TerminalIO) EnterStatus(area, id int) {
	io.Line()
	io.hidden = true
}

func (io *TerminalIO) LeaveStatus() {
	io.hidden = false
}

func (io *TerminalIO) HaveLinks() bool       { return false }
func (io *TerminalIO) EnterLink(str string)   {}
func (io *TerminalIO) LeaveLink()             {}
func (io *TerminalIO) EnterLinkRes(res Resource) {}
func (io *TerminalIO) LeaveLinkRes()          {}
func (io *TerminalIO) EnterSelfLink()         {}
func (io *TerminalIO) LeaveSelfLink()         {}

func (io *TerminalIO) LeaveAll() {
	io.Line()
	io.hidden = false
}

func (io *TerminalIO) EmbedRes(res Resource) {
	io.Print("[")
	io.Print(res.Alt)
	io.Print("]")
}

func (io *TerminalIO) CanEmbedRes(res Resource) bool { return false }

func (io *TerminalIO) ProgressBar(p, total int) {
	if !io.hidden {
		full := 0
		if io.width <= 0 {
			full = 80 - 3
		} else {
			full = io.width - 3
		}
		first := int(float64(full) * (float64(p) / float64(total)))
		second := full - first
		io.EnterDiv(-1)
		io.Print("[")
		for i := 0; i < first; i++ {
			io.Print("=")
		}
		for i := 0; i < second; i++ {
			io.Print(" ")
		}
		io.Print("]")
		io.LeaveDiv(-1)
	}
}

func (io *TerminalIO) Trace(str string) {
	if !io.hidden {
		io.EnterDiv(-1)
		fmt.Print(str)
		io.xpos = utf8.RuneCountInString(str)
		io.newlines = 0
		io.LeaveDiv(-1)
	}
}

func (io *TerminalIO) MeasureDims(which int) int {
	if which == 0 && io.width > 0 {
		return io.width
	}
	return 0
}

func (io *TerminalIO) ScriptOn() bool      { return false }
func (io *TerminalIO) ScriptOff()          {}
func (io *TerminalIO) ScriptActive() bool  { return false }
func (io *TerminalIO) HaveStyles() bool    { return false }
func (io *TerminalIO) HaveColor() bool     { return false }
func (io *TerminalIO) HaveAlign() bool     { return false }
func (io *TerminalIO) SetStyle(s int)      {}
func (io *TerminalIO) ResetStyle(s int)    {}
func (io *TerminalIO) Unstyle()            {}
func (io *TerminalIO) Clear()              { io.Par() }
func (io *TerminalIO) ClearAll()           { io.Par() }
func (io *TerminalIO) ClearLinks()         {}
func (io *TerminalIO) ClearOld()           {}
func (io *TerminalIO) ClearDiv()           {}
func (io *TerminalIO) ClearStatus()        {}

func (io *TerminalIO) Save(filedata []byte) bool {
	err := os.WriteFile("saved-game.aasave", filedata, 0644)
	return err == nil
}

func (io *TerminalIO) Restore() {}

func addTag(tagLines bool, status int) {
	if !tagLines {
		return
	}
	fmt.Print("\n")
	switch status {
	case StatusGetInput:
		fmt.Print("> ")
	case StatusGetKey:
		fmt.Print(") ")
	default:
		fmt.Print("  ")
	}
}

func usage() {
	fmt.Println("Usage: aamrun [OPTIONS] file.aastory")
	fmt.Println("    -s N            Set random seed.")
	fmt.Println("    -w N            Set screen width (default 80).")
	fmt.Println("    -h, --help      Show this message.")
	fmt.Println("    -v, -V          Show version and exit.")
	fmt.Println("    -T, --tag-lines Prepend output with \"  \", input with \"> \" or \") \".")
	fmt.Println("    -D, --dfrotz    Emulate dfrotz quirks.")
}

func main() {
	args := os.Args[1:]
	seed := 0
	width := 80
	quirks := false
	tagLines := false
	filename := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-s":
			if i+1 < len(args) {
				i++
				seed, _ = strconv.Atoi(args[i])
			}
		case "-w":
			if i+1 < len(args) {
				i++
				width, _ = strconv.Atoi(args[i])
			}
		case "-h", "--help":
			usage()
			os.Exit(0)
		case "-v", "-V":
			fmt.Printf("Å-machine Go frontend version %s\n", VERSION)
			os.Exit(0)
		case "-T", "--tag-lines":
			tagLines = true
		case "-D", "--dfrotz":
			quirks = true
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
				usage()
				os.Exit(1)
			}
			filename = args[i]
		}
	}

	if filename == "" {
		fmt.Fprintln(os.Stderr, "ERROR: Exactly one filename must be specified")
		usage()
		os.Exit(1)
	}

	storyfile, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to read file %s: %v\n", filename, err)
		os.Exit(1)
	}

	io := NewTerminalIO(width, quirks)
	engine := NewEngine(storyfile, io, uint32(seed), true, false, false)
	io.styles = GetStyles(engine)

	status := engine.VMStart()
	if status == StatusQuit {
		io.Line()
		io.Flush()
		os.Exit(0)
	}
	addTag(tagLines, status)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if status == StatusGetKey {
			fmt.Println(line)
			for i := 0; i < len(line) && status == StatusGetKey; i++ {
				if tagLines {
					fmt.Print("  ")
				}
				status = engine.VMProceedWithKey(int(line[i]))
				addTag(tagLines, status)
			}
			if status == StatusGetKey {
				if tagLines {
					fmt.Print("  ")
				}
				status = engine.VMProceedWithKey(Keys["KEY_RETURN"])
				addTag(tagLines, status)
			}
		} else if status == StatusGetInput {
			fmt.Println(line)
			io.xpos = 0
			io.pendingSpc = 0
			if io.quirks {
				io.newlines = 999
			} else {
				io.newlines = 1
			}
			if tagLines {
				fmt.Print("  ")
			}
			status = engine.VMProceedWithInput(line)
			addTag(tagLines, status)
		}
		if status == StatusQuit {
			if io.quirks {
				io.Par()
			} else {
				io.Line()
			}
			io.Flush()
			os.Exit(0)
		}
	}

	io.Line()
	io.Flush()
}

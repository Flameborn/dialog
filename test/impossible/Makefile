# Call `make DIFF=meld` to get a fancy diff
DIFF = diff

DIR6502 = ../../src/6502
AAMBOX = $(DIR6502)/aambox6502
FRONTEND = $(DIR6502)/aambox_frontend.bin

$(AAMBOX):
	$(MAKE) -C $(DIR6502) aambox6502

$(FRONTEND):
	$(MAKE) -C $(DIR6502) aambox_frontend.bin

all: test

test: test.js test.6502 test.lua test.go

test.js: impossible.js.out
	$(DIFF) impossible.js.out impossible.js.gold

impossible.js.out: impossible.aastory
	node ../../src/js/nodefrontend.js -s 1234 impossible.aastory <impossible.in >impossible.js.out

test.6502: impossible.6502.out
	$(DIFF) impossible.6502.out impossible.6502.gold

impossible.6502.out: impossible.aastory $(AAMBOX) $(FRONTEND)
	$(AAMBOX) -s 1234 $(FRONTEND) impossible.aastory <impossible.in >impossible.6502.out

test.lua: impossible.lua.out
	$(DIFF) impossible.lua.out impossible.lua.gold

impossible.lua.out: impossible.aastory
	luajit ../../src/lua/frontend.lua -s 1234 impossible.aastory <impossible.in >impossible.lua.out

test.go: impossible.go.out
	$(DIFF) impossible.go.out impossible.go.gold

impossible.go.out: impossible.aastory
	../../src/go/aamrun -s 1234 impossible.aastory <impossible.in >impossible.go.out

clean:
	rm -f *.out

.PHONY: all test test.js test.6502 test.lua test.go clean

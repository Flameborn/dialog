# VERY IMPORTANT: This should be a version of gosling compiled with Dialog 1a/01 or earlier, ensuring that it uses version 0.x of the Å-machine
# The purpose is to ensure the interpreters can still handle old story files

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

test.js: gosling.js.out
	$(DIFF) gosling.js.out gosling.js.gold

gosling.js.out: gosling.aastory
	node ../../src/js/nodefrontend.js -s 1234 gosling.aastory <gosling.in >gosling.js.out

test.6502: gosling.6502.out
	$(DIFF) gosling.6502.out gosling.6502.gold

gosling.6502.out: gosling.aastory $(AAMBOX) $(FRONTEND)
	$(AAMBOX) -s 1234 $(FRONTEND) gosling.aastory <gosling.in >gosling.6502.out

test.lua: gosling.lua.out
	$(DIFF) gosling.lua.out gosling.lua.gold

gosling.lua.out: gosling.aastory
	luajit ../../src/lua/frontend.lua -s 1234 gosling.aastory <gosling.in >gosling.lua.out

test.go: gosling.go.out
	$(DIFF) gosling.go.out gosling.go.gold

gosling.go.out: gosling.aastory
	../../src/go/aamrun -s 1234 gosling.aastory <gosling.in >gosling.go.out

clean:
	rm -f *.out

.PHONY: all test test.js test.6502 test.lua test.go clean

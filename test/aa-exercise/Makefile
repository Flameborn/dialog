# Call `make DIFF=meld` to get a fancy diff
DIFF = diff

DIR6502 = ../../src/6502
AAMBOX = $(DIR6502)/aambox6502
FRONTEND = $(DIR6502)/aambox_frontend.bin
ASSEMBLE = ../../../aavm/aavm.scm

$(AAMBOX):
	$(MAKE) -C $(DIR6502) aambox6502

$(FRONTEND):
	$(MAKE) -C $(DIR6502) aambox_frontend.bin

assemble:
	$(ASSEMBLE) -S aa-exercise.scm aa-exercise.aastory

all: test

test: test.js test.6502 test.lua test.go

test.js: aa-exercise.js.out
	$(DIFF) aa-exercise.js.out aa-exercise.js.gold

aa-exercise.js.out: aa-exercise.aastory
	node ../../src/js/nodefrontend.js -s 1234 aa-exercise.aastory <aa-exercise.in >aa-exercise.js.out

test.6502: aa-exercise.6502.out
	$(DIFF) aa-exercise.6502.out aa-exercise.6502.gold

aa-exercise.6502.out: aa-exercise.aastory $(AAMBOX) $(FRONTEND)
	$(AAMBOX) -s 1234 $(FRONTEND) aa-exercise.aastory <aa-exercise.in >aa-exercise.6502.out

test.lua: aa-exercise.lua.out
	$(DIFF) aa-exercise.lua.out aa-exercise.lua.gold

aa-exercise.lua.out: aa-exercise.aastory
	luajit ../../src/lua/frontend.lua -s 1234 aa-exercise.aastory <aa-exercise.in >aa-exercise.lua.out

test.go: aa-exercise.go.out
	$(DIFF) aa-exercise.go.out aa-exercise.go.gold

aa-exercise.go.out: aa-exercise.aastory
	../../src/go/aamrun -s 1234 aa-exercise.aastory <aa-exercise.in >aa-exercise.go.out

clean:
	rm -f *.out

.PHONY: all test test.js test.6502 test.lua test.go clean

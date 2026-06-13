package main

import (
	"os"
	"testing"
)

func loadStory(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to load %s: %v", path, err)
	}
	return data
}

func TestIFFParsing(t *testing.T) {
	data := loadStory(t, "../../test/body_not_status/body_not_status.aastory")

	// Check FORM header
	if get4(data, 0) != "FORM" {
		t.Fatal("Missing FORM header")
	}
	if get4(data, 8) != "AAVM" {
		t.Fatal("Missing AAVM type")
	}

	// Find required chunks
	chunks := []string{"HEAD", "CODE", "DICT", "INIT", "LANG", "MAPS", "WRIT", "LOOK"}
	for _, name := range chunks {
		ch := findChunk(data, name)
		if ch == nil {
			t.Errorf("Missing required chunk: %s", name)
		} else {
			t.Logf("Found chunk %s: %d bytes", name, len(ch))
		}
	}

	// Verify HEAD format
	head := findChunk(data, "HEAD")
	if head == nil {
		t.Fatal("HEAD chunk missing")
	}
	verMajor := int(head[0])
	verMinor := int(head[1])
	wordSize := int(head[2])
	t.Logf("Story format: %d.%d, word size: %d", verMajor, verMinor, wordSize)
	if wordSize != 2 {
		t.Errorf("Expected word size 2, got %d", wordSize)
	}

	// Check heap/aux/ram sizes
	heapSize := get16(head, 16)
	auxSize := get16(head, 18)
	ramSize := get16(head, 20)
	t.Logf("Heap: %d, Aux: %d, RAM: %d", heapSize, auxSize, ramSize)
	if heapSize == 0 || auxSize == 0 || ramSize == 0 {
		t.Error("Invalid memory sizes")
	}
}

func TestIFFParsingGosling(t *testing.T) {
	data := loadStory(t, "../../test/gosling/gosling.aastory")

	if get4(data, 0) != "FORM" {
		t.Fatal("Missing FORM header")
	}
	if get4(data, 8) != "AAVM" {
		t.Fatal("Missing AAVM type")
	}

	chunks := []string{"HEAD", "CODE", "DICT", "INIT", "LANG", "MAPS", "WRIT", "LOOK"}
	for _, name := range chunks {
		ch := findChunk(data, name)
		if ch == nil {
			t.Errorf("Missing required chunk: %s", name)
		} else {
			t.Logf("Found chunk %s: %d bytes", name, len(ch))
		}
	}

	head := findChunk(data, "HEAD")
	verMajor := int(head[0])
	verMinor := int(head[1])
	t.Logf("Gosling format: %d.%d", verMajor, verMinor)
	// Gosling is a 0.x file
	if verMajor != 0 {
		t.Errorf("Expected gosling format 0.x, got %d.%d", verMajor, verMinor)
	}
}

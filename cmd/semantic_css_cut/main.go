// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

var (
	selFile = flag.String("sel", "", "selection file")
	outFile = flag.String("o", "", "output file")
)

func main() {
	flag.Parse()
	var allChunks []chunk
	for _, fn := range flag.Args() {
		chunks, err := parseCSSFile(fn)
		if err != nil {
			log.Fatal(err)
		}
		allChunks = append(allChunks, chunks...)
	}
	var w io.Writer
	if *outFile != "" {
		f, err := os.Create(*outFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		w = f
	} else {
		w = os.Stdout
	}
	if *selFile == "" {
		outputTOC(w, allChunks)
		return
	}
	f, err := os.Open(*selFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	chunks, err := parseSelection(f, *selFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = outputCSS(w, allChunks, chunks, *selFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type chunk struct {
	level   int
	name    string
	text    string
	linenum int
	enabled bool
}

func parseCSSFile(filename string) ([]chunk, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	chunks, err := parseCSS(f)
	if err != nil {
		return nil, err
	}
	chunks[0].name = filename
	return chunks, err
}

func parseCSS(r io.Reader) ([]chunk, error) {
	chunks := []chunk{}
	var b bytes.Buffer
	nextName := false
	name := ""
	level := 0
	linenum := 0
	chunkline := 1
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		linenum++
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "/*!"):
			chunks = append(chunks, chunk{level: level, name: name, text: b.String(), linenum: chunkline})
			chunkline = linenum
			b.Reset()
			level = 0
			name = strings.TrimSpace(strings.TrimPrefix(line, "/*!"))
			nextName = (name == "")
		case strings.HasPrefix(line, "/***"):
			chunks = append(chunks, chunk{level: level, name: name, text: b.String(), linenum: chunkline})
			chunkline = linenum
			b.Reset()
			level = 1
			nextName = true
		case strings.HasPrefix(line, "/*----"):
			chunks = append(chunks, chunk{level: level, name: name, text: b.String(), linenum: chunkline})
			chunkline = linenum
			b.Reset()
			level = 2
			nextName = true
		case strings.HasPrefix(line, "/*--- ") && strings.HasSuffix(line, " ---*/"):
			chunks = append(chunks, chunk{level: level, name: name, text: b.String(), linenum: chunkline})
			chunkline = linenum
			b.Reset()
			level = 3
			nextName = false
			name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "/*--- "), " ---*/"))
		case strings.HasPrefix(line, "/* ") && strings.HasSuffix(line, " */"):
			chunks = append(chunks, chunk{level: level, name: name, text: b.String(), linenum: chunkline})
			chunkline = linenum
			b.Reset()
			level = 4
			nextName = false
			name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "/* "), " */"))
		default:
			if nextName {
				name = strings.TrimSpace(line)
				nextName = false
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	chunks = append(chunks, chunk{level: level, name: name, text: b.String(), linenum: chunkline})
	return chunks, nil
}

func outputTOC(w io.Writer, chunks []chunk) error {
	for _, c := range chunks {
		if _, err := fmt.Fprintf(w, "%s%s\n", prefixes[c.level], c.name); err != nil {
			return err
		}
	}
	return nil
}

var prefixes = []string{
	"@ ", "# ", "## ", "### ", "#### ",
}

func parseSelection(r io.Reader, filename string) ([]chunk, error) {
	chunks := []chunk{}
	sc := bufio.NewScanner(r)
	var name string
	linenum := 0
	for sc.Scan() {
		linenum++
		line := sc.Text()
		if line == "" {
			continue
		}
		enabled := true
		switch line[0] {
		case '+':
			line = line[1:]
		case '-':
			enabled = false
			line = line[1:]
		}
		level := -1
		for i, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				level = i
				name = strings.TrimPrefix(line, prefix)
				break
			}
		}
		if level == -1 {
			return nil, fmt.Errorf("%s:%d: line does not start with known prefix", filename, linenum)
		}
		chunks = append(chunks, chunk{level: level, name: name, linenum: linenum, enabled: enabled})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func outputCSS(w io.Writer, input, selection []chunk, filename string) error {
	var fn string
	// check input and selection match
	for i := range input {
		if input[i].level == 0 {
			fn = input[i].name
		}
		if input[i].level != selection[i].level || input[i].name != selection[i].name {
			fmt.Errorf("%s:%d: section level or name does not match\n%s:%d: source here",
				filename, selection[i].linenum, fn, input[i].linenum)
		}
	}
	enabled := true
	level := 0
	for i := range input {
		if !enabled && input[i].level <= level {
			enabled = selection[i].enabled
			level = selection[i].level
		}
		if enabled && !selection[i].enabled {
			enabled = false
			level = selection[i].level
			continue
		}
		if enabled {
			if _, err := io.WriteString(w, input[i].text); err != nil {
				return err
			}
		}
	}
	return nil
}

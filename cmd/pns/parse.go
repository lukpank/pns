// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrThreeStars        = errors.New(`separator (first line of imported file) should start with "***"`)
	ErrEmptyTagList      = errors.New("empty tag list")
	ErrNoTopic           = errors.New("no topic in a tag list")
	ErrEmptyLineExpected = errors.New("empty line expected after the header")
)

func parseFile(filename string) ([]*Note, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parse(f)
}

func parse(r io.Reader) ([]*Note, error) {
	var sep string
	var err error
	var notes []*Note
	sc := bufio.NewScanner(r)
	if sc.Scan() {
		sep = sc.Text()
		if !strings.HasPrefix(sep, "***") {
			return nil, ErrThreeStars
		}
	} else {
		if err = sc.Err(); err != nil {
			return nil, err
		}
		return nil, io.ErrUnexpectedEOF
	}
	n := &Note{}
outer:
	for {
		if sc.Scan() {
			err = n.parseTags(sc.Text())
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
		if sc.Scan() {
			n.Created, err = time.Parse("2006-01-02 15:04:05 -0700", sc.Text())
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
		if sc.Scan() {
			n.Modified, err = time.Parse("2006-01-02 15:04:05 -0700", sc.Text())
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
		if sc.Scan() {
			n.ID, err = strconv.ParseInt(sc.Text(), 10, 64)
			if err != nil {
				return nil, err
			}
		}
		if sc.Scan() {
			if sc.Text() != "" {
				return nil, ErrEmptyLineExpected
			}
		} else {
			break
		}
		var lines []string
		for sc.Scan() {
			line := sc.Text()
			if line == sep {
				n.Text = strings.Join(lines, "\n")
				notes = append(notes, n)
				n = &Note{}
				continue outer
			}
			lines = append(lines, line)
		}
		if err = sc.Err(); err != nil {
			return nil, err
		}
		n.Text = strings.Join(lines, "\n")
		notes = append(notes, n)
		return notes, nil
	}
	if err = sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF

}

func (n *Note) parseTags(line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ErrEmptyTagList
	}
	for _, s := range fields {
		if s[0] == '/' {
			n.Topics = append(n.Topics, s)
		} else {
			n.Tags = append(n.Tags, s)
		}
	}
	if len(n.Topics) == 0 {
		return ErrNoTopic
	}
	return nil
}

// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"os"
	"time"
)

type Progress struct {
	cnt  int
	done int
	t    time.Time
	b    bytes.Buffer
	len  int
}

func NewProgress(cnt int) *Progress {
	return &Progress{cnt: cnt, t: time.Now()}
}

func (p *Progress) Done() {
	p.done += 1
	d := time.Since(p.t)
	left := (time.Duration(p.cnt)*d)/time.Duration(p.done) - d
	dh, dm, ds := hms(d)
	lh, lm, ls := hms(left)
	p.b.Reset()
	n, _ := fmt.Fprintf(&p.b, "\r%d/%d duration %02d:%02d:%02d, left %02d:%02d:%02d", p.done, p.cnt, dh, dm, ds, lh, lm, ls)
	if p.len <= n {
		p.b.WriteByte(' ')
	}
	for i := p.len; i > n; i-- {
		p.b.WriteByte(' ')
	}
	if p.done == p.cnt {
		p.b.WriteByte('\n')
	}
	p.len = n
	os.Stderr.Write(p.b.Bytes())
}

func hms(d time.Duration) (int, int, int) {
	s := int(d.Seconds())
	h := s / 3600
	s -= 3600 * h
	m := s / 60
	s -= 60 * m
	return h, m, s
}

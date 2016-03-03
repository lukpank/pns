// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"unicode"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

var NoDifference = errors.New("no difference")

// htmlDiff writes diff of two given texts as HTML into the given
// io.Writer. htmlDiff returns error only if there are no differences
// between the texts (pseudo error NoDifference) or if there are
// errors while writing to the given io.Writer.
func htmlDiff(w io.Writer, oldText, newText string) (err error) {
	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToRunes(oldText, newText)
	diff := dmp.DiffCharsToLines(dmp.DiffMainRunes(a, b, false), lines)
	if len(diff) == 1 && diff[0].Type == diffmatchpatch.DiffEqual {
		return NoDifference
	}
	skip := -1
	for i, d := range diff {
		if i == skip {
			continue
		}
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			if i+1 < len(diff) && diff[i+1].Type == diffmatchpatch.DiffInsert {
				err = htmlTokenDiff(w, dmp, d.Text, diff[i+1].Text)
				skip = i + 1
			} else {
				_, err = fmt.Fprintf(w, `<div class="del">%s</div>`, template.HTMLEscapeString(d.Text))
			}
		case diffmatchpatch.DiffEqual:
			_, err = fmt.Fprintf(w, `<div class="context">%s</div>`, template.HTMLEscapeString(d.Text))
		case diffmatchpatch.DiffInsert:
			_, err = fmt.Fprintf(w, `<div class="ins">%s</div>`, template.HTMLEscapeString(d.Text))
		}
		if err != nil {
			return
		}
	}
	return nil
}

func htmlTokenDiff(w io.Writer, dmp *diffmatchpatch.DiffMatchPatch, oldText, newText string) error {
	a, b, tokens := tokensToRunes(oldText, newText)
	diff := dmp.DiffCharsToLines(dmp.DiffMainRunes(a, b, false), tokens)

	_, err := w.Write([]byte(`<div class="del">`))
	if err != nil {
		return err
	}

	for _, d := range diff {
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			_, err = fmt.Fprintf(w, `<del>%s</del>`, template.HTMLEscapeString(d.Text))
		case diffmatchpatch.DiffEqual:
			_, err = w.Write([]byte(template.HTMLEscapeString(d.Text)))
		}
		if err != nil {
			return err
		}
	}

	_, err = w.Write([]byte(`</div><div class="ins">`))
	if err != nil {
		return err
	}

	for _, d := range diff {
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			_, err = w.Write([]byte(template.HTMLEscapeString(d.Text)))
		case diffmatchpatch.DiffInsert:
			_, err = fmt.Fprintf(w, `<ins>%s</ins>`, template.HTMLEscapeString(d.Text))
		}
		if err != nil {
			return err
		}
	}
	_, err = w.Write([]byte(`</div>`))
	if err != nil {
		return err
	}

	return nil
}

func tokensToRunes(oldText, newText string) ([]rune, []rune, []string) {
	oldTokens := splitTokens(oldText)
	newTokens := splitTokens(newText)
	oldRunes := make([]rune, len(oldTokens))
	newRunes := make([]rune, len(newTokens))
	m := make(map[string]rune)
	a := tokensToRunesCollect(nil, m, oldTokens, oldRunes)
	a = tokensToRunesCollect(a, m, newTokens, newRunes)
	return oldRunes, newRunes, a
}

func tokensToRunesCollect(a []string, m map[string]rune, tokens []string, runes []rune) []string {
	for i, s := range tokens {
		if j, present := m[s]; present {
			runes[i] = j
		} else {
			j = rune(len(a))
			m[s] = j
			a = append(a, s)
			runes[i] = j
		}
	}
	return a
}

func splitTokens(s string) []string {
	var (
		start      = 0
		width      int
		tokens     []string
		isAlphaNum = false
	)
	for i := 0; i < len(s); i += width {
		var r rune
		r, width = utf8.DecodeRuneInString(s[i:])
		if isAlphaNum {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				tokens = append(tokens, s[start:i])
				isAlphaNum = false
				tokens = append(tokens, s[i:i+width])
			}
		} else {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				isAlphaNum = true
				start = i
			} else {
				tokens = append(tokens, s[i:i+width])
			}

		}
	}
	if isAlphaNum {
		tokens = append(tokens, s[start:])
	}
	return tokens
}

// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/mxk/go-sqlite/sqlite3"
)

type DB struct {
	db *sql.DB
}

var ErrSingleThread = errors.New("single threaded sqlite3 is not supported")

func OpenDB(filename string) (*DB, error) {
	if sqlite3.SingleThread() {
		return nil, ErrSingleThread
	}
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

func (db *DB) Init() (err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)
	_, err = tx.Exec("CREATE TABLE notes(note TEXT, created INTEGER, modified INTEGER)")
	if err == nil {
		_, err = tx.Exec("CREATE TABLE tags(noteid INTEGER, tagid INTEGER)")
	}
	if err == nil {
		_, err = tx.Exec("CREATE TABLE tagnames(name TEXT UNIQUE)")
	}
	if err != nil {
		return err
	}
	done = true
	return nil
}

func commitOrRollback(tx *sql.Tx, done *bool, err *error) {
	var e error
	if *done {
		e = tx.Commit()
	} else {
		e = tx.Rollback()
	}
	if *err != nil {
		if e != nil {
			*err = MultiError{*err, e}
		}
	} else {
		*err = e
	}
}

func (db *DB) Import(notes []*Note) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	m := make(map[string]int64)
	for _, n := range notes {
		for _, s := range n.Topics {
			m[s] = -1
		}
		for _, s := range n.Tags {
			m[s] = -1
		}
	}

	for k := range m {
		result, err := tx.Exec("INSERT INTO tagnames VALUES(?)", k)
		if err != nil {
			return err
		}
		m[k], err = result.LastInsertId()
		if err != nil {
			return err
		}
	}

	for _, n := range notes {
		result, err := tx.Exec("INSERT INTO notes (note, created, modified) VALUES(?, ?, ?)",
			n.Text, n.Created, n.Modified)
		if err != nil {
			return err
		}
		noteid, err := result.LastInsertId()
		if err != nil {
			return err
		}

		for _, s := range n.Topics {
			_, err := tx.Exec("INSERT INTO tags (noteid, tagid) VALUES(?, ?)", noteid, m[s])
			if err != nil {
				return err
			}
		}
		for _, s := range n.Tags {
			_, err := tx.Exec("INSERT INTO tags (noteid, tagid) VALUES(?, ?)", noteid, m[s])
			if err != nil {
				return err
			}
		}
	}
	done = true
	return nil
}

type MultiError []error

func (me MultiError) Error() string {
	var s []string
	for _, err := range me {
		s = append(s, err.Error())
	}
	return strings.Join(s, "; ")
}

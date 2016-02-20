// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type sessions struct {
	mu   sync.Mutex
	m    map[string]*session
	next time.Time
	del  []string
}

type session struct {
	expires time.Time
	client  time.Time // the time session was send to the client
}

func NewSessions() *sessions {
	return &sessions{m: make(map[string]*session)}
}

// NewSession returns new random session ID. It also stores the
// session ID later authentication. It also stores session expiration
// time and time of sending the session cookie to the client. The
// session cookie send to the client should have max age equal to
// twice the duration given as argument to NewSession so the session
// is properly extended with following calls to CheckSession.
func (s *sessions) NewSession(d time.Duration) (string, error) {
	var a [16]byte
	_, err := rand.Read(a[:])
	if err != nil {
		return "", err
	}
	v := hex.EncodeToString(a[:])

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	t := now.Add(d)
	if len(s.m) == 0 || t.Before(s.next) {
		s.next = t
	}
	s.m[v] = &session{t, now} // now: we treat the new session cookie as already send
	s.expire()
	return v, nil
}

// CheckSession returns error (ErrAuth) on invalid or expired sessions
// and nil on a proper session.  Additionally the first return value
// indicates whether a new session cookie should be send to the
// client.  The session cookie send to the client should have max age
// equal to twice the duration given as argument to NewSession so the
// session is properly extended with following calls to CheckSession.
func (s *sessions) CheckSession(v string, d time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	entry, present := s.m[v]
	if !present {
		return false, ErrAuth
	}
	now := time.Now()
	entry.expires = now.Add(d)
	if now.Sub(entry.client) > d/2 {
		entry.client = now // we treat the new session cookie as already sent
		return true, nil
	}
	return false, nil
}

func (s *sessions) Remove(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, v)
}

// expire removes expired sessions. The map with with sessions is only
// iterated if some session is already expired. Caller should lock the
// mutex before calling expire.
func (s *sessions) expire() {
	if len(s.m) == 0 {
		return
	}
	now := time.Now()
	if s.next.Before(now) {
		s.next = now.Add(24 * time.Hour)
		for k, v := range s.m {
			if v.expires.Before(now) {
				s.del = append(s.del, k)
			} else if v.expires.Before(s.next) {
				s.next = v.expires
			}
		}
		for _, k := range s.del {
			delete(s.m, k)
		}
		s.del = s.del[:0]
	}
}

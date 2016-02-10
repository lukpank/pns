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
	m    map[string]time.Time
	next time.Time
	del  []string
}

func NewSessions() *sessions {
	return &sessions{m: make(map[string]time.Time)}
}

func (s *sessions) NewSession(d time.Duration) (string, error) {
	var a [16]byte
	_, err := rand.Read(a[:])
	if err != nil {
		return "", err
	}
	v := hex.EncodeToString(a[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	t := time.Now().Add(d)
	if len(s.m) == 0 || t.Before(s.next) {
		s.next = t
	}
	s.m[v] = t
	s.expire()
	return v, nil
}

func (s *sessions) ValidSession(v string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	_, present := s.m[v]
	return present
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
			if v.Before(now) {
				s.del = append(s.del, k)
			} else if v.Before(s.next) {
				s.next = v
			}
		}
		for _, k := range s.del {
			delete(s.m, k)
		}
		s.del = s.del[:0]
	}
}

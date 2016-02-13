// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

type logger struct {
	handler http.Handler
}

func (l *logger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	rw := &responseWriter{w, 0, false}
	defer func() {
		log.Println(remoteAddr(r), r.Host, r.Method, path, "-", rw.status, http.StatusText(rw.status), time.Since(t))
	}()
	l.handler.ServeHTTP(rw, r)
}

func remoteAddr(r *http.Request) string {
	forward := r.Header.Get("X-Forwarded-For")
	if forward != "" {
		return fmt.Sprintf("%s (%s)", r.RemoteAddr, forward)
	}
	return r.RemoteAddr
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

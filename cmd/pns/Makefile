GOFILES := $(wildcard *.go)
REV := $(shell git log -1 --format=%h)

pns: $(GOFILES)
	go build -ldflags '-X main.Version=pns-0.1-$(REV)'

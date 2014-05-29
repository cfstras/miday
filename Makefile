GOPATH=$(CURDIR)
export GOPATH

all: compile

run: compile start

imports:
	goimports -l -w *.go

compile: imports
	go build

start:
	./miday

deps:
	sudo apt-get install libportmidi-dev portaudio19-dev
	go get \
		gopkg.in/qml.v0 \
		code.google.com/p/go.tools/cmd/goimports \
		code.google.com/p/portaudio-go/portaudio

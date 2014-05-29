GOPATH=$(CURDIR)
export GOPATH

all: compile

run: compile start

clean:
	rm -rf bin/ pkg/
	rm -rf src/code.google.com src/gopkg.in

fix:
	goimports -l -w *.go

compile:
	go build

start:
	./miday

INSTALL = $(shell which brew || which apt-get || which yum)

deps:
	sudo ${INSTALL} install libportmidi-dev portaudio19-dev
	go get \
		gopkg.in/qml.v0 \
		code.google.com/p/go.tools/cmd/goimports \
		code.google.com/p/portaudio-go/portaudio

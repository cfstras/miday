GOPATH=$(CURDIR)
export GOPATH

run: compile start

imports:
	goimports -l -w .

compile: imports
	go build

start:
	./miday

deps:
	sudo apt-get install libportmidi-dev
	go get gopkg.in/qml.v0 code.google.com/p/go.tools/cmd/goimports github.com/rakyll/portmidi

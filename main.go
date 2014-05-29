package main

import (
	"fmt"

	midi "github.com/cfstras/miday/portmidi"
)

func main() {
	fmt.Println("hello, miday")
	midi.Initialize()
}

package main

import (
	"sync"

	midi "github.com/cfstras/miday/portmidi"
)

type MidiRouter struct {
	Inputs  []<-chan midi.Event
	Outputs []chan<- midi.Event
}

func (r *MidiRouter) Route() {
	var group sync.WaitGroup
	for _, ch := range r.Inputs {
		group.Add(1)
		go func(input <-chan midi.Event) {
			for ev := range input {
				for _, out := range r.Outputs {
					out <- ev
				}
			}
			group.Done()
		}(ch)
	}
	group.Wait()
}

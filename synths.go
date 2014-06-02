package main

import (
	"fmt"
	"math"

	midi "github.com/cfstras/miday/portmidi"
)

type Note struct {
	note      uint8
	vel       uint8
	lastPhase bool
	lastVal   float64
}

type Synth struct {
	input       chan midi.Event
	notes       [16]Note
	phases      [256]float64
	volume      float64
	totalVolume float64
	scaleVolume float64
}

type SineSynth struct {
	Synth
}

func (s *Synth) Route() {
	for ev := range s.input {
		st := uint8(ev.Status) / 0xf
		vel := uint8(ev.Data2)
		note := uint8(ev.Data1)
		var pos *Note
		for i, v := range s.notes {
			if v.note == note {
				pos = &s.notes[i]
				break
			}
		}
		if pos == nil {
			for i, v := range s.notes {
				if v.vel == 0 {
					pos = &s.notes[i]
				}
			}
		}
		if pos == nil {
			pos = &s.notes[0]
		}
		switch st {
		case 0x8: // note off
			fmt.Println("note", note, "off")
			if pos.note == note {
				pos.lastPhase = true
			}

		case 0x9: // note on
			fmt.Println("note", note, vel)
			pos.lastPhase = false
			pos.vel = vel
			pos.note = note
		default:
			fmt.Printf("unknown st %x, note %d, vel %d\n", st, note, vel)
			// ignore other commands
		}
		s.totalVolume = 0
		for _, n := range s.notes {
			s.totalVolume += float64(n.vel) / 127
		}
		if s.totalVolume > 1 {
			s.scaleVolume = 0.75 / s.totalVolume
		} else {
			s.scaleVolume = 0.75
		}
	}
}

func (s *SineSynth) Process(InBufs [][]float32, OutBufs [][]float32) {
	for i := range OutBufs[0] {
		OutBufs[0][i] = 0
		OutBufs[1][i] = 0
		for i2 := range s.notes {
			note := &s.notes[i2]
			if note.vel == 0 {
				continue
			}
			val := s.volume * math.Sin(2*math.Pi*s.phases[note.note]) * (float64(note.vel) / 127)
			if note.lastPhase && math.Copysign(val, note.lastVal) != val {
				note.vel = 0
				s.phases[note.note] = 0
				note.lastPhase = false
			}
			note.lastVal = val

			OutBufs[0][i] += float32(val * s.scaleVolume)
			OutBufs[1][i] += float32(val * s.scaleVolume)

			step := math.Exp2((float64(note.note)-69)/12) * 440 / sampleRate
			_, s.phases[note.note] = math.Modf(s.phases[note.note] + step)
		}
	}
}

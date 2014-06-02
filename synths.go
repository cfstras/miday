package main

import (
	"fmt"
	"math"

	midi "github.com/cfstras/miday/portmidi"
)

type Note struct {
	note uint8
	vel  uint8
}

type Synth struct {
	input       chan midi.Event
	notes       [16]Note
	phases      [256]float64
	volume      float64
	notesOn     int
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
			if pos.vel > 0 && pos.note == note {
				s.notesOn--
				s.totalVolume -= float64(pos.vel) / 127
			}
			pos.vel = 0
			s.phases[note] = 0

		case 0x9: // note on
			fmt.Println("note", note, vel)
			if pos.vel == 0 {
				s.notesOn++
				s.totalVolume += float64(vel) / 127
			} else {
				s.totalVolume += float64(vel)/127 - float64(pos.vel)/127
			}
			pos.vel = vel
			pos.note = note
		default:
			fmt.Printf("unknown st %x, note %d, vel %d\n", st, note, vel)
			// ignore other commands
		}
		if s.totalVolume > 1 {
			s.scaleVolume = 1 / s.totalVolume
		} else {
			s.scaleVolume = 1
		}
	}
}

func (s *SineSynth) Process(InBufs [][]float32, OutBufs [][]float32) {
	for i := range OutBufs[0] {
		OutBufs[0][i] = 0
		OutBufs[1][i] = 0
		for _, note := range s.notes {
			if note.vel == 0 {
				continue
			}
			val := s.volume * math.Sin(2*math.Pi*s.phases[note.note]) * (float64(note.vel) / 127)

			OutBufs[0][i] += float32(val * s.scaleVolume)
			OutBufs[1][i] += float32(val * s.scaleVolume)

			step := math.Exp2((float64(note.note)-69)/12) * 440 / sampleRate
			//step := 440.0 / sampleRate

			_, s.phases[note.note] = math.Modf(s.phases[note.note] + step)
		}
		if OutBufs[0][i] > 1 {
			OutBufs[0][i] = 1
		}
	}
}

package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	audio "code.google.com/p/portaudio-go/portaudio"
	midi "github.com/cfstras/miday/portmidi"
)

const sampleRate = 44100

type Streams struct {
	Audios   []*AudioStream
	MidiIns  []*MidiStream
	MidiOuts []*MidiStream
}

type Stream struct {
	Name      string
	Interface string
}

type MidiStream struct {
	Stream
	midi   *midi.Stream
	events <-chan midi.Event
}

type AudioStream struct {
	Stream
	audio   *audio.Stream
	InBufs  [][]float32
	OutBufs [][]float32
	Ops     []AudioEffect
}

type AudioEffect interface {
	Process(InBufs [][]float32, OutBufs [][]float32)
}

type Note struct {
	note uint8
	vel  uint8
}

type SineSynth struct {
	input   chan midi.Event
	notes   [16]Note
	phases  [256]float64
	volume  float64
	notesOn int
}

func (s *SineSynth) Route() {
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
			}
			pos.vel = 0

		case 0x9: // note on
			fmt.Println("note", note, vel)
			if pos.vel == 0 {
				s.notesOn++
			}
			pos.vel = vel
			pos.note = note
		default:
			fmt.Printf("%x\n", st)
			// ignore other commands
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

			OutBufs[0][i] += float32(val) / float32(s.notesOn)
			OutBufs[1][i] += float32(val) / float32(s.notesOn)

			step := math.Exp2((float64(note.note)-69)/12) * 440 / sampleRate
			//step := 440.0 / sampleRate

			_, s.phases[note.note] = math.Modf(s.phases[note.note] + step)
		}
		if OutBufs[0][i] > 1 {
			OutBufs[0][i] = 1
		}
	}
}

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

var streams Streams
var errors chan error
var printer chan midi.Event

func main() {
	fmt.Println("hello, miday")
	streams = Streams{}

	errors = make(chan error, 8)
	go func(input <-chan error) {
		for err := range input {
			fmt.Fprintln(os.Stderr, "error:", err)
			panic(err)
		}
	}(errors)

	printer = make(chan midi.Event, 8)
	go func(input <-chan midi.Event) {
		for ev := range input {
			fmt.Println("midi:", ev)
		}
	}(printer)

	midi.Initialize()
	defer midi.Terminate()
	audio.Initialize()
	defer audio.Terminate()
	streams.initMidi()
	streams.initAudio()

	router := MidiRouter{}
	for _, str := range streams.MidiIns {
		router.Inputs = append(router.Inputs, str.events)
	}
	router.Outputs = []chan<- midi.Event{}

	sineSynth := &SineSynth{input: make(chan midi.Event, 8), volume: 1}
	router.Outputs = append(router.Outputs, sineSynth.input)
	streams.Audios[0].Ops = append(streams.Audios[0].Ops, sineSynth)

	go sineSynth.Route()
	go router.Route()
	go streams.startAudio()
	defer streams.stopAudio()

	bio := bufio.NewReader(os.Stdin)
	bio.ReadLine()
}

func (streams *Streams) initMidi() {
	streams.MidiIns = append(streams.MidiIns, openMidis(true, "LPK")...)
	streams.MidiOuts = append(streams.MidiOuts, openMidis(false, "LPK")...)
}

func openMidis(doInput bool, filter string) []*MidiStream {
	var strs []*MidiStream
	num := midi.DeviceId(midi.CountDevices())
	for i := midi.DeviceId(0); i < num; i++ {
		info := midi.GetDeviceInfo(i)
		if info.IsOpened {
			continue
		}
		if !strings.Contains(info.Name, filter) {
			continue
		}
		if !(info.IsInputAvailable && doInput) &&
			!(info.IsOutputAvailable && !doInput) {
			continue
		}
		fmt.Println("opening", info)

		var stream *midi.Stream
		var err error
		if doInput {
			stream, err = midi.NewInputStream(i, 64)
		} else {
			stream, err = midi.NewOutputStream(i, 64, 0)
		}
		if err != nil {
			errors <- err
			continue
		}
		var ch <-chan midi.Event
		if doInput {
			ch = stream.Listen()
		}
		str := &MidiStream{Stream{info.Name, info.Interface},
			stream, ch}
		strs = append(strs, str)
	}
	return strs
}

func (streams *Streams) initAudio() {
	stream := &AudioStream{Stream{"portaudio", "stream"}, nil, nil, nil, nil}
	fmt.Println("opening audio")
	var err error
	stream.audio, err = audio.OpenDefaultStream(0, 2, sampleRate, 1024*4, stream.Process)
	if err != nil {
		errors <- err
		return
	}
	streams.Audios = []*AudioStream{stream}
}

func (streams *Streams) startAudio() {
	for _, s := range streams.Audios {
		err := s.audio.Start()
		if err != nil {
			errors <- err
		}
	}
}

func (streams *Streams) stopAudio() {
	for _, s := range streams.Audios {
		s.audio.Stop()
	}
}

func (stream *AudioStream) Process(out [][]float32) {
	//fmt.Println("processing", len(out[0]), "samples")
	//stream.InBufs = in
	stream.OutBufs = out
	for _, op := range stream.Ops {
		op.Process(stream.InBufs, stream.OutBufs)
	}
}

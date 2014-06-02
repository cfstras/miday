package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	audio "code.google.com/p/portaudio-go/portaudio"
	midi "github.com/cfstras/miday/portmidi"
)

const (
	sampleRate = 44100
)

type Streams struct {
	Audio    *AudioStream
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
	audio      *audio.Stream
	InBufs     [][]float32
	OutBufs    [][]float32
	BufferSize int
	Ops        []AudioEffect
}

type AudioEffect interface {
	Process(InBufs [][]float32, OutBufs [][]float32)
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

	sineSynth := &SineSynth{}
	sineSynth.input = make(chan midi.Event, 8)
	sineSynth.volume = 1
	router.Outputs = append(router.Outputs, sineSynth.input)
	streams.Audio.Ops = append(streams.Audio.Ops, sineSynth)

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
	stream := &AudioStream{Stream{"portaudio", "stream"}, nil, nil, nil, 0, nil}
	fmt.Println("opening audio")
	var err error
	h, err := audio.DefaultHostApi()
	params := audio.LowLatencyParameters(nil, h.DefaultOutputDevice)
	params.Input.Channels = 0
	params.Output.Channels = 2
	params.FramesPerBuffer = 2048
	stream.BufferSize = params.FramesPerBuffer
	stream.audio, err = audio.OpenStream(params, stream.Process)
	if err != nil {
		errors <- err
		return
	}
	streams.Audio = stream
}

func (streams *Streams) startAudio() {
	err := streams.Audio.audio.Start()
	if err != nil {
		errors <- err
	}
}

func (streams *Streams) stopAudio() {
	streams.Audio.audio.Stop()
}

func (stream *AudioStream) Process(out [][]float32) {
	//fmt.Println("processing", len(out[0]), "samples")
	//stream.InBufs = in
	stream.OutBufs = out
	for _, op := range stream.Ops {
		op.Process(stream.InBufs, stream.OutBufs)
	}
}

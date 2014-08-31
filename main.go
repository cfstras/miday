package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
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
	midi      *midi.Stream
	eventsIn  <-chan midi.Event
	eventsOut chan<- midi.Event
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
var devicePrefix string

func main() {
	flag.StringVar(&devicePrefix, "prefix", "", "MIDI device prefix to filter devices")
	flag.Parse()

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
	defer exit()
	audio.Initialize()

	sigChan := make(chan os.Signal)
	go func(ch <-chan os.Signal) {
		for _ = range ch {
			exit()
		}
	}(sigChan)
	signal.Notify(sigChan)

	streams.initMidi()
	streams.initAudio()

	router := MidiRouter{}
	for _, str := range streams.MidiIns {
		router.Inputs = append(router.Inputs, str.eventsIn)
	}
	router.Outputs = []chan<- midi.Event{}
	for _, str := range streams.MidiOuts {
		router.Outputs = append(router.Outputs, str.eventsOut)
	}

	sineSynth := &SineSynth{}
	sineSynth.input = make(chan midi.Event, 8)
	sineSynth.volume = 1
	router.Outputs = append(router.Outputs, sineSynth.input)
	streams.Audio.Ops = append(streams.Audio.Ops, sineSynth)

	go func() {
		go sineSynth.Route()
		go router.Route()
		go streams.startAudio()
		defer streams.stopAudio()
	}()

	//RunGui()

	bio := bufio.NewReader(os.Stdin)
	bio.ReadLine()
}

func exit() {
	streams.Audio.audio.Close()
	for _, s := range streams.MidiIns {
		s.midi.Close()
	}
	streams.MidiIns = streams.MidiIns[:0]
	for _, s := range streams.MidiOuts {
		close(s.eventsOut)
		s.midi.Close()
	}
	streams.MidiIns = streams.MidiOuts[:0]
	audio.Terminate()
	midi.Terminate()
	os.Exit(0)
}

func (streams *Streams) initMidi() {
	streams.MidiIns = append(streams.MidiIns, openMidis(true, devicePrefix)...)
	streams.MidiOuts = append(streams.MidiOuts, openMidis(false, devicePrefix)...)
}

func getMidiDevices() (res []*midi.DeviceInfo) {
	num := midi.DeviceId(midi.CountDevices())
	for i := midi.DeviceId(0); i < num; i++ {
		info := midi.GetDeviceInfo(i)
		res = append(res, info)
	}
	return
}

func openMidis(doInput bool, filter string) []*MidiStream {
	var strs []*MidiStream
	for ii, info := range getMidiDevices() {
		i := midi.DeviceId(ii)
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
		var chIn <-chan midi.Event
		var chOut chan midi.Event
		if doInput {
			chIn = stream.Listen()
		} else {
			chOut = make(chan midi.Event)
			go func(ch <-chan midi.Event) {
				for e := range ch {
					stream.Write([]midi.Event{e})
				}
			}(chOut)
		}
		str := &MidiStream{Stream{info.Name, info.Interface},
			stream, chIn, chOut}
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

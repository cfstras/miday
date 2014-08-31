package main

import (
	"fmt"
	qml "gopkg.in/qml.v1"
)

func RunGui() {
	if err := qml.Run(run); err != nil {
		fmt.Println("error:", err)
		panic(err)
	}
}

func run() error {
	engine := qml.NewEngine()

	component, err := engine.LoadFile("gui.qml")
	if err != nil {
		return err
	}
	win := component.CreateWindow(nil)
	win.Set("x", 560)
	win.Set("y", 320)
	win.Show()
	win.Wait()

	return nil
}

package main

import (
	"client/internal"
	"fmt"

	"github.com/caarlos0/env/v11"
)

func main() {
	var cfg internal.Config
	err := env.Parse(&cfg)
	if err != nil {
		panic(err.Error())
	}
	game, err := internal.NewGame(&cfg)
	if err != nil {
		panic(err.Error())
	}
	err = game.Start()
	if err != nil {
		panic(err.Error())
	}
	for {
		value := <-game.Events
		fmt.Printf("Notification value: %v\n", value)
	}
}

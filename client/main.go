package main

import (
	"client/internal"
	"context"
	"errors"

	"github.com/caarlos0/env/v11"
	"github.com/hajimehoshi/ebiten/v2"
	_ "github.com/hajimehoshi/ebiten/v2"
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
	err = ebiten.RunGame(game)
	if err != nil && !errors.Is(err, internal.GameFinished) {
		panic(err.Error())
	}
	err = game.Finish()
	if err != nil && !errors.Is(err, context.Canceled) {
		panic(err.Error())
	}
}

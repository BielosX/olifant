package internal

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gonum.org/v1/gonum/spatial/r2"
)

type Player struct {
	Position r2.Vec
	Velocity r2.Vec
	Score    int32
}

func NewPlayer() *Player {
	return &Player{
		Position: r2.Vec{
			X: 0.5,
			Y: 0.5,
		},
		Velocity: r2.Vec{
			X: 0.0,
			Y: 0.0,
		},
		Score: 0,
	}
}

type Game struct {
	id              uuid.UUID
	pool            *pgxpool.Pool
	listenerErrors  chan error
	Events          chan bool
	listenerContext context.Context
	listenerCancel  context.CancelFunc
	listenerWait    chan struct{}
	player          *Player
}

func getPostgresUrl(cfg *Config, appName string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?application_name=%s",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		appName)
}

func NewGame(cfg *Config) (*Game, error) {
	ctx := context.Background()
	id := uuid.New()
	config, err := pgxpool.ParseConfig(getPostgresUrl(cfg, id.String()))
	if err != nil {
		return nil, err
	}
	config.MaxConns = 4
	config.MinConns = 2
	config.MinIdleConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	listenerErrors := make(chan error, 16)
	events := make(chan bool, 1024)
	listenerWait := make(chan struct{})
	listenerContext, listenerCancel := context.WithCancel(context.Background())
	player := NewPlayer()
	return &Game{
		id,
		pool,
		listenerErrors,
		events,
		listenerContext,
		listenerCancel,
		listenerWait,
		player,
	}, nil
}

func (g *Game) listen() {
	c, err := g.pool.Acquire(g.listenerContext)
	if err != nil {
		g.listenerErrors <- err
		g.listenerWait <- struct{}{}
		return
	}
	defer c.Release()
	_, err = c.Exec(g.listenerContext, fmt.Sprintf("LISTEN \"%s\"", g.id))
	if err != nil {
		g.listenerErrors <- err
		g.listenerWait <- struct{}{}
		return
	}
	for {
		select {
		case <-g.listenerContext.Done():
			g.listenerWait <- struct{}{}
			return
		default:
			var n *pgconn.Notification
			n, err = c.Conn().WaitForNotification(g.listenerContext)
			if err != nil {
				g.listenerErrors <- err
				continue
			}
			b, err := strconv.ParseBool(n.Payload)
			if err != nil {
				g.listenerErrors <- err
				continue
			}
			g.Events <- b
		}
	}
}

func (g *Game) Start() error {
	go g.listen()
	ctx := context.Background()
	c, err := g.pool.Acquire(ctx)
	if err != nil {
		g.listenerCancel()
		return err
	}
	defer c.Release()
	_, err = c.Exec(ctx, "SELECT game.start_game($1, $2)", g.id.String(), "test")
	if err != nil {
		g.listenerCancel()
		return err
	}
	return nil
}

type GameInput string

const (
	UpPressed     GameInput = "UP_PRESSED"
	UpReleased    GameInput = "UP_RELEASED"
	DownPressed   GameInput = "DOWN_PRESSED"
	DownReleased  GameInput = "DOWN_RELEASED"
	LeftPressed   GameInput = "LEFT_PRESSED"
	LeftReleased  GameInput = "LEFT_RELEASED"
	RightPressed  GameInput = "RIGHT_PRESSED"
	RightReleased GameInput = "RIGHT_RELEASED"
)

func (g *Game) SendInputEvent(input GameInput) error {
	ctx := context.Background()
	c, err := g.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	_, err = c.Exec(ctx, "INSERT INTO game.input_events (game_id, event) VALUES ($1, $2)", g.id.String(), input)
	return err
}

func (g *Game) GetPlayer() (*Player, error) {
	ctx := context.Background()
	c, err := g.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Release()
	r, err := c.Query(ctx, "SELECT position, velocity, score FROM game.players WHERE game_id=$1", g.id.String())
	if err != nil {
		return nil, err
	}
	if !r.Next() {
		return nil, errors.New(fmt.Sprintf("player with game_id=%s not found", g.id.String()))
	}
	var position []float64
	var velocity []float64
	var score int32
	err = r.Scan(&position, &velocity, &score)
	if err != nil {
		return nil, err
	}
	if !(len(position) == 2 && len(velocity) == 2) {
		return nil, errors.New("position and Velocity should have len == 2")
	}
	return &Player{
		Position: r2.Vec{
			X: position[0],
			Y: position[1],
		},
		Velocity: r2.Vec{
			X: velocity[0],
			Y: velocity[1],
		},
		Score: score,
	}, nil
}

func (g *Game) Finish() {
	g.listenerCancel()
	<-g.listenerWait
}

func (g *Game) Update() error {
	select {
	case isFinished := <-g.Events:
		if isFinished {
			os.Exit(0)
		}
		player, err := g.GetPlayer()
		if err != nil {
			return err
		}
		g.player = player
	default:
		position := r2.Add(g.player.Position, r2.Scale(1.0/60.0, g.player.Velocity))
		g.player.Position = position
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyW) {
		err := g.SendInputEvent(UpPressed)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyW) {
		err := g.SendInputEvent(UpReleased)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		err := g.SendInputEvent(DownPressed)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyS) {
		err := g.SendInputEvent(DownReleased)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyA) {
		err := g.SendInputEvent(LeftPressed)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyA) {
		err := g.SendInputEvent(LeftReleased)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyD) {
		err := g.SendInputEvent(RightPressed)
		if err != nil {
			return err
		}
	}
	if inpututil.IsKeyJustReleased(ebiten.KeyD) {
		err := g.SendInputEvent(RightReleased)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	position := g.player.Position
	dy := float64(screen.Bounds().Dy())
	x := float32(position.X * float64(screen.Bounds().Dx()))
	y := float32(dy - position.Y*dy)
	screen.Fill(color.Black)
	vector.FillRect(screen, x, y, 20.0, 20.0, color.White, false)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return outsideWidth, outsideHeight
}

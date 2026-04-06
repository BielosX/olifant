package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"math"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
	"gonum.org/v1/gonum/spatial/r2"
)

const BoundingCircleRadiusKey = "bounding_circle_radius"

var ZeroVec = r2.Vec{
	X: 0.0,
	Y: 0.0,
}

type GameConsts struct {
	PlayerBoundingCircleRadius float64
}

type Player struct {
	Position  r2.Vec
	Velocity  r2.Vec
	Direction r2.Vec
	Score     int32
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
		Direction: r2.Vec{
			X: 0.0,
			Y: 1.0,
		},
		Score: 0,
	}
}

type Game struct {
	id      uuid.UUID
	pool    *pgxpool.Pool
	context context.Context
	cancel  context.CancelFunc
	events  chan bool
	inputs  chan []GameInputEvent
	group   *errgroup.Group
	player  *Player
	consts  GameConsts
	sample  *ebiten.Image
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
	events := make(chan bool, 1024)
	inputs := make(chan []GameInputEvent, 1024)
	player := NewPlayer()
	ctx, cancel := context.WithCancel(context.Background())
	group, _ := errgroup.WithContext(ctx)
	consts := GameConsts{}
	sample := ebiten.NewImage(1, 1)
	sample.Fill(color.White)
	return &Game{
		id,
		pool,
		ctx,
		cancel,
		events,
		inputs,
		group,
		player,
		consts,
		sample,
	}, nil
}

func (g *Game) sendEvents() error {
	for {
		inputs := <-g.inputs
		err := g.SendInputEvents(inputs)
		if err != nil {
			return err
		}
	}
}

func (g *Game) listen() error {
	c, err := g.pool.Acquire(g.context)
	if err != nil {
		return err
	}
	defer c.Release()
	_, err = c.Exec(g.context, fmt.Sprintf("LISTEN \"%s\"", g.id))
	if err != nil {
		return err
	}
	for {
		select {
		case <-g.context.Done():
			return nil
		default:
			var n *pgconn.Notification
			n, err = c.Conn().WaitForNotification(g.context)
			if err != nil {
				return err
			}
			b, err := strconv.ParseBool(n.Payload)
			if err != nil {
				return err
			}
			g.events <- b
		}
	}
}

func (g *Game) Start() error {
	g.group.Go(g.listen)
	g.group.Go(g.sendEvents)
	c, err := g.pool.Acquire(g.context)
	if err != nil {
		g.cancel()
		return err
	}
	defer c.Release()
	v, err := g.GetConst(BoundingCircleRadiusKey)
	g.consts.PlayerBoundingCircleRadius = v.(map[string]any)["player"].(float64)
	if err != nil {
		g.cancel()
		return err
	}
	_, err = c.Exec(g.context, "SELECT game.start_game($1, $2)", g.id.String(), "test")
	if err != nil {
		g.cancel()
		return err
	}
	return nil
}

type GameInputEvent string

const (
	UpPressed     GameInputEvent = "UP_PRESSED"
	UpReleased    GameInputEvent = "UP_RELEASED"
	DownPressed   GameInputEvent = "DOWN_PRESSED"
	DownReleased  GameInputEvent = "DOWN_RELEASED"
	LeftPressed   GameInputEvent = "LEFT_PRESSED"
	LeftReleased  GameInputEvent = "LEFT_RELEASED"
	RightPressed  GameInputEvent = "RIGHT_PRESSED"
	RightReleased GameInputEvent = "RIGHT_RELEASED"
)

type GameInput int32

const (
	UpInput GameInput = iota
	DownInput
	LeftInput
	RightInput
)

type gameInputProps struct {
	Key           ebiten.Key
	PressedEvent  GameInputEvent
	ReleasedEvent GameInputEvent
}

var gameInputMapping map[GameInput]gameInputProps

func init() {
	gameInputMapping = make(map[GameInput]gameInputProps)
	gameInputMapping[UpInput] = gameInputProps{
		Key:           ebiten.KeyW,
		PressedEvent:  UpPressed,
		ReleasedEvent: UpReleased,
	}
	gameInputMapping[DownInput] = gameInputProps{
		Key:           ebiten.KeyS,
		PressedEvent:  DownPressed,
		ReleasedEvent: DownReleased,
	}
	gameInputMapping[LeftInput] = gameInputProps{
		Key:           ebiten.KeyA,
		PressedEvent:  LeftPressed,
		ReleasedEvent: LeftReleased,
	}
	gameInputMapping[RightInput] = gameInputProps{
		Key:           ebiten.KeyD,
		PressedEvent:  RightPressed,
		ReleasedEvent: RightReleased,
	}
}

func (g *Game) SendInputEvents(events []GameInputEvent) error {
	c, err := g.pool.Acquire(g.context)
	if err != nil {
		return err
	}
	defer c.Release()
	batch := &pgx.Batch{}
	for _, event := range events {
		batch.Queue("INSERT INTO game.input_events (game_id, event) VALUES ($1, $2)", g.id.String(), event)
	}
	br := c.SendBatch(g.context, batch)
	return br.Close()
}

func (g *Game) GetConst(key string) (any, error) {
	c, err := g.pool.Acquire(g.context)
	if err != nil {
		return nil, err
	}
	defer c.Release()
	r := c.QueryRow(g.context, "SELECT value FROM game.consts WHERE key=$1", key)
	var raw []byte
	err = r.Scan(&raw)
	if err != nil {
		return nil, err
	}
	var result any
	err = json.Unmarshal(raw, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Game) GetPlayer() (*Player, error) {
	c, err := g.pool.Acquire(g.context)
	if err != nil {
		return nil, err
	}
	defer c.Release()
	r := c.QueryRow(g.context, "SELECT position, velocity, direction, score FROM game.players WHERE game_id=$1", g.id.String())
	var position []float64
	var velocity []float64
	var direction []float64
	var score int32
	err = r.Scan(&position, &velocity, &direction, &score)
	if err != nil {
		return nil, err
	}
	if !(len(position) == 2 && len(velocity) == 2 && len(direction) == 2) {
		return nil, errors.New("position, velocity and direction should have len == 2")
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
		Direction: r2.Vec{
			X: direction[0],
			Y: direction[1],
		},
		Score: score,
	}, nil
}

func (g *Game) Finish() error {
	g.cancel()
	return g.group.Wait()
}

func (g *Game) Update() error {
	select {
	case <-g.context.Done():
		return nil
	case isFinished := <-g.events:
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
	var events []GameInputEvent
	for _, v := range gameInputMapping {
		if inpututil.IsKeyJustPressed(v.Key) {
			events = append(events, v.PressedEvent)
		}
		if inpututil.IsKeyJustReleased(v.Key) {
			events = append(events, v.ReleasedEvent)

		}
	}
	g.inputs <- events
	return nil
}

func (g *Game) drawPlayer(screen *ebiten.Image) {
	dir := r2.Scale(g.consts.PlayerBoundingCircleRadius, g.player.Direction)
	pos := g.player.Position
	angle := 2 * math.Pi / 3
	back := r2.Add(r2.Scale(-1.0, dir), pos)
	left := r2.Add(r2.Rotate(dir, -angle, ZeroVec), pos)
	right := r2.Add(r2.Rotate(dir, angle, ZeroVec), pos)
	front := r2.Add(dir, pos)
	op := &ebiten.DrawTrianglesOptions{}
	screenX := float32(screen.Bounds().Dx())
	screenY := float32(screen.Bounds().Dy())
	vertices := []ebiten.Vertex{
		{
			DstX:   float32(front.X) * screenX,
			DstY:   float32(1.0-front.Y) * screenY,
			ColorA: 1.0,
			ColorR: 1.0,
			ColorG: 1.0,
			ColorB: 1.0,
		},
		{
			DstX:   float32(right.X) * screenX,
			DstY:   float32(1.0-right.Y) * screenY,
			ColorA: 1.0,
			ColorR: 1.0,
			ColorG: 1.0,
			ColorB: 1.0,
		},
		{
			DstX:   float32(left.X) * screenX,
			DstY:   float32(1.0-left.Y) * screenY,
			ColorA: 1.0,
			ColorR: 1.0,
			ColorG: 1.0,
			ColorB: 1.0,
		},
		{
			DstX:   float32(back.X) * screenX,
			DstY:   float32(1.0-back.Y) * screenY,
			ColorA: 1.0,
			ColorR: 1.0,
			ColorG: 1.0,
			ColorB: 1.0,
		},
	}
	screen.DrawTriangles(vertices, []uint16{0, 1, 2, 2, 1, 3}, g.sample, op)
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.drawPlayer(screen)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return outsideWidth, outsideHeight
}

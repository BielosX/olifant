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
	"time"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/sync/errgroup"
	"gonum.org/v1/gonum/spatial/r2"
)

const BoundingCircleRadiusKey = "bounding_circle_radius"

var GameFinished = errors.New("GameFinished")

var ZeroVec = r2.Vec{
	X: 0.0,
	Y: 0.0,
}

var BlueColor = color.RGBA{
	A: 255,
	R: 0,
	G: 0,
	B: 255,
}

var RedColor = color.RGBA{
	A: 255,
	R: 255,
	G: 0,
	B: 0,
}

type GameConsts struct {
	PlayerBoundingCircleRadius float64
	EnemyBoundingCircleRadius  float64
}

type Enemy struct {
	Position r2.Vec
	Velocity r2.Vec
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
	id       uuid.UUID
	pool     *pgxpool.Pool
	context  context.Context
	cancel   context.CancelFunc
	events   chan bool
	inputs   chan []GameInputEvent
	group    *errgroup.Group
	player   *Player
	enemies  []Enemy
	consts   GameConsts
	sample   *ebiten.Image
	lastFire time.Time
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
	lastFire := time.Time{}
	return &Game{
		id,
		pool,
		ctx,
		cancel,
		events,
		inputs,
		group,
		player,
		[]Enemy{},
		consts,
		sample,
		lastFire,
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
	g.consts.EnemyBoundingCircleRadius = v.(map[string]any)["enemy"].(float64)
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
	Fire          GameInputEvent = "FIRE"
)

type GameInput int32

const (
	UpInput GameInput = iota
	DownInput
	LeftInput
	RightInput
	FireInput
)

type gameInputProps struct {
	Key           ebiten.Key
	PressedEvent  GameInputEvent
	ReleasedEvent *GameInputEvent
}

var gameInputMapping map[GameInput]gameInputProps

func init() {
	gameInputMapping = make(map[GameInput]gameInputProps)
	gameInputMapping[UpInput] = gameInputProps{
		Key:           ebiten.KeyW,
		PressedEvent:  UpPressed,
		ReleasedEvent: new(UpReleased),
	}
	gameInputMapping[DownInput] = gameInputProps{
		Key:           ebiten.KeyS,
		PressedEvent:  DownPressed,
		ReleasedEvent: new(DownReleased),
	}
	gameInputMapping[LeftInput] = gameInputProps{
		Key:           ebiten.KeyA,
		PressedEvent:  LeftPressed,
		ReleasedEvent: new(LeftReleased),
	}
	gameInputMapping[RightInput] = gameInputProps{
		Key:           ebiten.KeyD,
		PressedEvent:  RightPressed,
		ReleasedEvent: new(RightReleased),
	}
	gameInputMapping[FireInput] = gameInputProps{
		Key:           ebiten.KeySpace,
		PressedEvent:  Fire,
		ReleasedEvent: nil,
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

func fromServerVec(vec []float64) r2.Vec {
	if len(vec) != 2 {
		panic("server vec len different than 2")
	}
	return r2.Vec{
		X: vec[0],
		Y: vec[1],
	}
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
	return &Player{
		Position:  fromServerVec(position),
		Velocity:  fromServerVec(velocity),
		Direction: fromServerVec(direction),
		Score:     score,
	}, nil
}

func (g *Game) GetEnemies() ([]Enemy, error) {
	c, err := g.pool.Acquire(g.context)
	if err != nil {
		return nil, err
	}
	defer c.Release()
	r, err := c.Query(g.context, "SELECT position, velocity FROM game.enemies WHERE game_id=$1", g.id.String())
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var position []float64
	var velocity []float64
	var enemies []Enemy
	for r.Next() {
		err = r.Scan(&position, &velocity)
		if err != nil {
			return nil, err
		}
		enemies = append(enemies, Enemy{
			Position: fromServerVec(position),
			Velocity: fromServerVec(velocity),
		})
	}
	if r.Err() != nil {
		return nil, r.Err()
	}
	return enemies, nil
}

func (g *Game) Finish() error {
	g.cancel()
	close(g.inputs)
	close(g.events)
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
		enemies, err := g.GetEnemies()
		if err != nil {
			return err
		}
		g.enemies = enemies
	default:
		position := r2.Add(g.player.Position, r2.Scale(1.0/60.0, g.player.Velocity))
		g.player.Position = position
		for i := range g.enemies {
			position = r2.Add(g.enemies[i].Position, r2.Scale(1.0/60.0, g.enemies[i].Velocity))
			g.enemies[i].Position = position
		}
	}
	var events []GameInputEvent
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.cancel()
		return GameFinished
	}
	for k, v := range gameInputMapping {
		if inpututil.IsKeyJustPressed(v.Key) {
			if k == FireInput {
				if g.lastFire.Add(time.Millisecond * 500).Before(time.Now()) {
					events = append(events, v.PressedEvent)
					g.lastFire = time.Now()
				}
			} else {
				events = append(events, v.PressedEvent)
			}
		}
		if inpututil.IsKeyJustReleased(v.Key) && v.ReleasedEvent != nil {
			events = append(events, *v.ReleasedEvent)

		}
	}
	g.inputs <- events
	return nil
}

func toVertex(vec r2.Vec, color color.Color) ebiten.Vertex {
	r, g, b, a := color.RGBA()
	maxValue := float32(0xffff)
	return ebiten.Vertex{
		DstX:   float32(vec.X),
		DstY:   float32(vec.Y),
		ColorR: float32(r) / maxValue,
		ColorG: float32(g) / maxValue,
		ColorB: float32(b) / maxValue,
		ColorA: float32(a) / maxValue,
	}
}

func (g *Game) drawEntity(screen *ebiten.Image, direction r2.Vec, pos r2.Vec, radius float64, color color.RGBA) {
	dir := r2.Scale(radius, direction)
	angle := 2 * math.Pi / 3
	back := r2.Add(r2.Scale(-1.0, dir), pos)
	rightDir := r2.Rotate(dir, angle, ZeroVec)
	right := r2.Add(rightDir, pos)
	leftDir := r2.Sub(r2.Scale(2.0*r2.Dot(rightDir, direction), direction), rightDir)
	left := r2.Add(leftDir, pos)
	front := r2.Add(dir, pos)
	op := &ebiten.DrawTrianglesOptions{}
	screenX := float64(screen.Bounds().Dx())
	screenY := float64(screen.Bounds().Dy())
	var vertices [4]ebiten.Vertex
	for i, v := range []r2.Vec{front, right, left, back} {
		vertices[i] = toVertex(r2.Vec{
			X: v.X * screenX,
			Y: (1.0 - v.Y) * screenY,
		}, color)
	}
	screen.DrawTriangles(vertices[:], []uint16{0, 1, 2, 2, 1, 3}, g.sample, op)
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.drawEntity(screen, g.player.Direction, g.player.Position, g.consts.PlayerBoundingCircleRadius, BlueColor)
	for _, enemy := range g.enemies {
		g.drawEntity(screen, r2.Unit(enemy.Velocity), enemy.Position, g.consts.EnemyBoundingCircleRadius, RedColor)
	}
	text.Draw(screen, fmt.Sprintf("Score: %v", g.player.Score), basicfont.Face7x13, 20, 20, color.White)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return outsideWidth, outsideHeight
}

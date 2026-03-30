package internal

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Game struct {
	id              uuid.UUID
	pool            *pgxpool.Pool
	listenerErrors  chan error
	Events          chan bool
	listenerContext context.Context
	listenerCancel  context.CancelFunc
	listenerWait    chan struct{}
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
	return &Game{
		id,
		pool,
		listenerErrors,
		events,
		listenerContext,
		listenerCancel,
		listenerWait,
	}, nil
}

func (g *Game) listen() {
	c, err := g.pool.Acquire(g.listenerContext)
	if err != nil {
		g.listenerErrors <- err
		return
	}
	_, err = c.Exec(g.listenerContext, fmt.Sprintf("LISTEN \"%s\"", g.id))
	if err != nil {
		g.listenerErrors <- err
		return
	}
	for {
		select {
		case <-g.listenerContext.Done():
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
	_, err = c.Exec(ctx, "SELECT game.start_game($1, $2)", g.id.String(), "test")
	if err != nil {
		g.listenerCancel()
		return err
	}
	return nil
}

package main

import (
	"context"
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Username string `env:"PG_USERNAME" envDefault:"test"`
	Password string `env:"PG_PASSWORD" envDefault:"test"`
	Host     string `env:"PG_HOST" envDefault:"localhost"`
	Port     int    `env:"PG_PORT" envDefault:"5432"`
	Database string `env:"PG_DB" envDefault:"postgres"`
}

func main() {
	ctx := context.Background()
	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		panic(err.Error())
	}
	url := fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Connected to DB")
	c, err := pool.Acquire(ctx)
	if err != nil {
		panic(err.Error())
	}
	_, err = c.Exec(ctx, "LISTEN game")
	if err != nil {
		panic(err.Error())
	}
	for {
		var n *pgconn.Notification
		n, err = c.Conn().WaitForNotification(ctx)
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("Received notification, payload: %s\n", n.Payload)
	}
}

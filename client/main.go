package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://test:test@localhost:5432/postgres")
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

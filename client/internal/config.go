package internal

type Config struct {
	Username string `env:"PG_USERNAME" envDefault:"test"`
	Password string `env:"PG_PASSWORD" envDefault:"test"`
	Host     string `env:"PG_HOST" envDefault:"localhost"`
	Port     int    `env:"PG_PORT" envDefault:"5432"`
	Database string `env:"PG_DB" envDefault:"postgres"`
}
